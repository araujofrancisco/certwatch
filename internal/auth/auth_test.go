package auth

import (
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("mysecretpassword")
	if err != nil {
		t.Fatal(err)
	}

	if err := CheckPassword(hash, "mysecretpassword"); err != nil {
		t.Error("expected password to match")
	}

	if err := CheckPassword(hash, "wrongpassword"); err == nil {
		t.Error("expected password mismatch")
	}
}

func TestGenerateAndValidateToken(t *testing.T) {
	a := New("test-secret", time.Hour)

	token, err := a.GenerateToken(42, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := a.ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}

	if claims.UserID != 42 {
		t.Errorf("expected 42, got %d", claims.UserID)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("expected user@example.com, got %s", claims.Email)
	}
}

func TestValidateInvalidToken(t *testing.T) {
	a := New("test-secret", time.Hour)
	_, err := a.ValidateToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestValidateExpiredToken(t *testing.T) {
	a := New("test-secret", -time.Hour)

	token, err := a.GenerateToken(1, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.ValidateToken(token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidateWrongSecret(t *testing.T) {
	a1 := New("secret1", time.Hour)
	a2 := New("secret2", time.Hour)

	token, err := a1.GenerateToken(1, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}

	_, err = a2.ValidateToken(token)
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}
