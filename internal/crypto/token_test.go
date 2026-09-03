package crypto

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestEncryptDecryptPayload(t *testing.T) {
	h := sha256.Sum256([]byte("my-super-secret-key-32-bytes-long!"))
	key := h[:]

	original := &FileTokenPayload{
		ChatID:    -1001234567890,
		MessageID: 98765,
		FileHash:  "a1b2c3",
		FileSize:  1073741824, // 1 GB
		FileName:  "Episode 01 [1080p].mkv",
		CreatedAt: time.Now().Unix(),
	}

	token, err := EncryptPayload(original, key)
	if err != nil {
		t.Fatalf("EncryptPayload failed: %v", err)
	}

	if token == "" {
		t.Fatal("token is empty")
	}

	decrypted, err := DecryptToken(token, key)
	if err != nil {
		t.Fatalf("DecryptToken failed: %v", err)
	}

	if decrypted.ChatID != original.ChatID {
		t.Errorf("ChatID mismatch: expected %d, got %d", original.ChatID, decrypted.ChatID)
	}
	if decrypted.MessageID != original.MessageID {
		t.Errorf("MessageID mismatch: expected %d, got %d", original.MessageID, decrypted.MessageID)
	}
	if decrypted.FileHash != original.FileHash {
		t.Errorf("FileHash mismatch: expected %s, got %s", original.FileHash, decrypted.FileHash)
	}
	if decrypted.FileSize != original.FileSize {
		t.Errorf("FileSize mismatch: expected %d, got %d", original.FileSize, decrypted.FileSize)
	}
	if decrypted.FileName != original.FileName {
		t.Errorf("FileName mismatch: expected %s, got %s", original.FileName, decrypted.FileName)
	}
}

func TestResolveLegacyPath(t *testing.T) {
	h := sha256.Sum256([]byte("my-super-secret-key-32-bytes-long!"))
	key := h[:]

	legacyPath := "abcdef12345"
	payload, err := ResolveTokenOrLegacy(legacyPath, -100111, key)
	if err != nil {
		t.Fatalf("ResolveTokenOrLegacy failed: %v", err)
	}
	if payload.MessageID != 12345 {
		t.Errorf("expected msgID 12345, got %d", payload.MessageID)
	}
	if payload.FileHash != "abcdef" {
		t.Errorf("expected hash abcdef, got %s", payload.FileHash)
	}
}
