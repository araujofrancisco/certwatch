package models

import (
	"testing"
	"time"
)

func TestUserCreation(t *testing.T) {
	u := User{
		Email:    "test@example.com",
		Password: "hashedpassword",
		Name:     "Test User",
	}
	if u.Email != "test@example.com" {
		t.Errorf("expected test@example.com, got %s", u.Email)
	}
}

func TestDomainCreation(t *testing.T) {
	d := Domain{
		Domain:      "example.com",
		Description: "Test domain",
		Enabled:     true,
	}
	if !d.Enabled {
		t.Error("expected domain to be enabled")
	}
}

func TestCertificateStatus(t *testing.T) {
	c := Certificate{
		Status: "valid",
	}
	if c.Status != "valid" {
		t.Errorf("expected valid, got %s", c.Status)
	}
}

func TestCertificateExpiry(t *testing.T) {
	now := time.Now()
	c := Certificate{
		NotBefore: now.Add(-24 * time.Hour),
		NotAfter:  now.Add(24 * time.Hour),
	}
	if c.NotAfter.Before(now) {
		t.Error("expected certificate to be valid")
	}
}
