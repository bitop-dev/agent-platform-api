package auth

import (
	"testing"
)

func TestGenerateAndValidateToken(t *testing.T) {
	a := New("test-secret-key-32chars-minimum!", 60)

	token, err := a.GenerateToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := a.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("expected user-123, got %s", claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected test@example.com, got %s", claims.Email)
	}
}

func TestValidateToken_InvalidSecret(t *testing.T) {
	a1 := New("secret-one-32chars-minimum!!!!!!", 60)
	a2 := New("secret-two-32chars-minimum!!!!!!", 60)

	token, _ := a1.GenerateToken("user-1", "a@b.com")

	_, err := a2.ValidateToken(token)
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestValidateToken_Garbage(t *testing.T) {
	a := New("test-secret", 60)
	_, err := a.ValidateToken("not-a-jwt")
	if err == nil {
		t.Error("expected error for garbage token")
	}
}

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("mysecretpassword")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if !CheckPassword("mysecretpassword", hash) {
		t.Error("correct password should match")
	}
	if CheckPassword("wrongpassword", hash) {
		t.Error("wrong password should not match")
	}
}
