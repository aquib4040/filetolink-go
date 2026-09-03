package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid or corrupted download token")
	ErrTokenExpired = errors.New("token has expired")
	
	// Direct path regex for legacy/compat: 6-char hash followed by numeric message ID
	legacyPathRegex = regexp.MustCompile(`^([a-zA-Z0-9_-]{6})(\d+)$`)
)

// FileTokenPayload contains everything needed to locate and stream a file from Telegram statelessly.
type FileTokenPayload struct {
	ChatID    int64  `json:"c"`
	MessageID int64  `json:"m"`
	FileHash  string `json:"h"`
	FileSize  int64  `json:"s"`
	FileName  string `json:"n"`
	CreatedAt int64  `json:"t"`
}

// EncryptPayload serializes and encrypts the file payload using AES-256-GCM.
// Returns a compact, URL-safe Base64 token with no padding.
func EncryptPayload(payload *FileTokenPayload, secretKey []byte) (string, error) {
	if len(secretKey) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(secretKey))
	}

	if payload.CreatedAt == 0 {
		payload.CreatedAt = time.Now().Unix()
	}

	plainData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token payload: %w", err)
	}

	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plainData, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// DecryptToken decodes and decrypts an AES-256-GCM token back into FileTokenPayload.
func DecryptToken(tokenStr string, secretKey []byte) (*FileTokenPayload, error) {
	if len(secretKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(secretKey))
	}

	rawBytes, err := base64.RawURLEncoding.DecodeString(tokenStr)
	if err != nil {
		// Try standard URLEncoding with padding
		rawBytes, err = base64.URLEncoding.DecodeString(tokenStr)
		if err != nil {
			return nil, ErrInvalidToken
		}
	}

	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(rawBytes) < nonceSize {
		return nil, ErrInvalidToken
	}

	nonce, ciphertext := rawBytes[:nonceSize], rawBytes[nonceSize:]
	plainData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidToken
	}

	var payload FileTokenPayload
	if err := json.Unmarshal(plainData, &payload); err != nil {
		return nil, ErrInvalidToken
	}

	return &payload, nil
}

// ResolveTokenOrLegacy attempts to decrypt the token. If decryption fails,
// it checks if the string matches legacy {hash:6}{message_id} pattern.
func ResolveTokenOrLegacy(tokenOrPath string, defaultChatID int64, secretKey []byte) (*FileTokenPayload, error) {
	// 1. Try stateless decrypt
	payload, err := DecryptToken(tokenOrPath, secretKey)
	if err == nil && payload.MessageID > 0 {
		if payload.ChatID == 0 {
			payload.ChatID = defaultChatID
		}
		return payload, nil
	}

	// 2. Try legacy direct pattern: 6-char hash + message_id
	matches := legacyPathRegex.FindStringSubmatch(tokenOrPath)
	if len(matches) == 3 {
		msgID, pErr := strconv.ParseInt(matches[2], 10, 64)
		if pErr == nil && msgID > 0 {
			return &FileTokenPayload{
				ChatID:    defaultChatID,
				MessageID: msgID,
				FileHash:  matches[1],
				FileName:  "download",
				CreatedAt: time.Now().Unix(),
			}, nil
		}
	}

	return nil, ErrInvalidToken
}
