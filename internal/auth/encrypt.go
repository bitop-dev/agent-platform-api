package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

// Encryptor handles AES-256-GCM encryption for API keys at rest.
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor creates an encryptor from a 32-byte hex-encoded key.
// If key is empty, returns a no-op encryptor that stores keys in plaintext (dev mode).
func NewEncryptor(hexKey string) (*Encryptor, error) {
	if hexKey == "" {
		return &Encryptor{}, nil // dev mode — no encryption
	}

	keyBytes, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (64 hex chars), got %d", len(keyBytes))
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &Encryptor{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns ciphertext bytes.
func (e *Encryptor) Encrypt(plaintext string) ([]byte, error) {
	if e.gcm == nil {
		return []byte(plaintext), nil // dev mode
	}

	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return e.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt decrypts ciphertext bytes and returns the plaintext string.
func (e *Encryptor) Decrypt(ciphertext []byte) (string, error) {
	if e.gcm == nil {
		return string(ciphertext), nil // dev mode
	}

	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, data := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// KeyHint returns the last 4 characters of a key, prefixed with "...".
func KeyHint(key string) string {
	if len(key) <= 4 {
		return "..." + key
	}
	return "..." + key[len(key)-4:]
}
