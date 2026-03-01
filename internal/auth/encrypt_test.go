package auth

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// 32 bytes = 64 hex chars
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}

	original := "sk-ant-api03-very-secret-key-here"
	ciphertext, err := enc.Encrypt(original)
	if err != nil {
		t.Fatal(err)
	}

	if string(ciphertext) == original {
		t.Error("ciphertext should differ from plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if decrypted != original {
		t.Errorf("expected %q, got %q", original, decrypted)
	}
}

func TestEncryptDecrypt_DevMode(t *testing.T) {
	enc, err := NewEncryptor("")
	if err != nil {
		t.Fatal(err)
	}

	original := "sk-test-key"
	ciphertext, err := enc.Encrypt(original)
	if err != nil {
		t.Fatal(err)
	}

	// Dev mode stores plaintext
	if string(ciphertext) != original {
		t.Error("dev mode should store plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if decrypted != original {
		t.Errorf("expected %q, got %q", original, decrypted)
	}
}

func TestEncryptor_BadKey(t *testing.T) {
	_, err := NewEncryptor("tooshort")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestKeyHint(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"sk-ant-api03-abcdefgh", "...efgh"},
		{"abc", "...abc"},
		{"", "..."},
	}
	for _, tt := range tests {
		got := KeyHint(tt.key)
		if got != tt.want {
			t.Errorf("KeyHint(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
