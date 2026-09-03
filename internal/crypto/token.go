package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid or corrupted download token")
	ErrTokenExpired = errors.New("token has expired")

	// Direct path regex for legacy/compat: 6-char hash followed by numeric message ID
	legacyPathRegex = regexp.MustCompile(`^([a-zA-Z0-9_-]{6})(\d+)$`)
	
	// Fixed deterministic IV for compact 12-byte CTR tokens
	compactIV = []byte("F2L_GO_CTR_IV_16")
)

// FileTokenPayload contains everything needed to locate and stream a file from Telegram statelessly.
type FileTokenPayload struct {
	ChatID    int64  `json:"c"`
	MessageID int64  `json:"m"`
	FileHash  string `json:"h,omitempty"`
	FileSize  int64  `json:"s,omitempty"`
	FileName  string `json:"n,omitempty"`
	CreatedAt int64  `json:"t,omitempty"`
}

// EncryptCompactMessageID generates a short, 24-character hex stateless token (e.g. 6a99b22902aa195013d8b09e).
// It packs the MessageID and a salted CRC32 checksum encrypted via AES-CTR.
func EncryptCompactMessageID(messageID int64, secretKey []byte) string {
	if len(secretKey) != 32 {
		h := sha256.Sum256(secretKey)
		secretKey = h[:]
	}

	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return fmt.Sprintf("%d", messageID)
	}

	// 12-byte payload:
	// [0..5]: MessageID (6 bytes, handles IDs up to 2^48)
	// [6..7]: Timestamp in minutes (2 bytes)
	// [8..11]: Salted CRC32 checksum (4 bytes)
	buf := make([]byte, 12)
	buf[0] = byte(messageID >> 40)
	buf[1] = byte(messageID >> 32)
	buf[2] = byte(messageID >> 24)
	buf[3] = byte(messageID >> 16)
	buf[4] = byte(messageID >> 8)
	buf[5] = byte(messageID)

	nowMin := uint16((time.Now().Unix() / 60) & 0xFFFF)
	buf[6] = byte(nowMin >> 8)
	buf[7] = byte(nowMin)

	crc := crc32.ChecksumIEEE(append(buf[0:8], secretKey[:8]...))
	binary.BigEndian.PutUint32(buf[8:12], crc)

	stream := cipher.NewCTR(block, compactIV)
	encrypted := make([]byte, 12)
	stream.XORKeyStream(encrypted, buf)

	return hex.EncodeToString(encrypted)
}

// DecryptCompactMessageID verifies and extracts MessageID from a 24-character hex token.
func DecryptCompactMessageID(tokenStr string, secretKey []byte) (int64, bool) {
	if len(tokenStr) != 24 {
		return 0, false
	}

	encrypted, err := hex.DecodeString(tokenStr)
	if err != nil || len(encrypted) != 12 {
		return 0, false
	}

	if len(secretKey) != 32 {
		h := sha256.Sum256(secretKey)
		secretKey = h[:]
	}

	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return 0, false
	}

	stream := cipher.NewCTR(block, compactIV)
	decrypted := make([]byte, 12)
	stream.XORKeyStream(decrypted, encrypted)

	expectedCRC := crc32.ChecksumIEEE(append(decrypted[0:8], secretKey[:8]...))
	actualCRC := binary.BigEndian.Uint32(decrypted[8:12])
	if expectedCRC != actualCRC {
		return 0, false
	}

	msgID := (int64(decrypted[0]) << 40) |
		(int64(decrypted[1]) << 32) |
		(int64(decrypted[2]) << 24) |
		(int64(decrypted[3]) << 16) |
		(int64(decrypted[4]) << 8) |
		int64(decrypted[5])

	if msgID <= 0 {
		return 0, false
	}

	return msgID, true
}

// EncryptPayload serializes, gzips, and encrypts the file payload using AES-256-GCM.
func EncryptPayload(payload *FileTokenPayload, secretKey []byte) (string, error) {
	if len(secretKey) != 32 {
		h := sha256.Sum256(secretKey)
		secretKey = h[:]
	}

	if payload.CreatedAt == 0 {
		payload.CreatedAt = time.Now().Unix()
	}

	plainData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token payload: %w", err)
	}

	// Compress with gzip
	var gzBuf bytes.Buffer
	gzWriter, _ := gzip.NewWriterLevel(&gzBuf, gzip.BestCompression)
	_, _ = gzWriter.Write(plainData)
	_ = gzWriter.Close()
	dataToEncrypt := gzBuf.Bytes()

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

	ciphertext := gcm.Seal(nonce, nonce, dataToEncrypt, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// DecryptToken decodes and decrypts an AES-256-GCM token back into FileTokenPayload (supports both gzip and plain).
func DecryptToken(tokenStr string, secretKey []byte) (*FileTokenPayload, error) {
	if len(secretKey) != 32 {
		h := sha256.Sum256(secretKey)
		secretKey = h[:]
	}

	rawBytes, err := base64.RawURLEncoding.DecodeString(tokenStr)
	if err != nil {
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

	// Check if gzipped
	if len(plainData) > 2 && plainData[0] == 0x1f && plainData[1] == 0x8b {
		gzReader, gzErr := gzip.NewReader(bytes.NewReader(plainData))
		if gzErr == nil {
			decompressed, readErr := io.ReadAll(gzReader)
			_ = gzReader.Close()
			if readErr == nil {
				plainData = decompressed
			}
		}
	}

	var payload FileTokenPayload
	if err := json.Unmarshal(plainData, &payload); err != nil {
		return nil, ErrInvalidToken
	}

	return &payload, nil
}

// ResolveTokenOrLegacy resolves a compact 24-char token, a gzip AES-GCM token, or legacy {hash:6}{message_id}.
func ResolveTokenOrLegacy(tokenOrPath string, defaultChatID int64, secretKey []byte) (*FileTokenPayload, error) {
	tokenOrPath = strings.TrimSpace(tokenOrPath)

	// 1. Try compact 24-char hex token first
	if msgID, ok := DecryptCompactMessageID(tokenOrPath, secretKey); ok {
		return &FileTokenPayload{
			ChatID:    defaultChatID,
			MessageID: msgID,
			CreatedAt: time.Now().Unix(),
		}, nil
	}

	// 2. Try AES-GCM token (gzipped or raw)
	payload, err := DecryptToken(tokenOrPath, secretKey)
	if err == nil && payload.MessageID > 0 {
		if payload.ChatID == 0 {
			payload.ChatID = defaultChatID
		}
		return payload, nil
	}

	// 3. Try legacy direct pattern: 6-char hash + message_id
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
