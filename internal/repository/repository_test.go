package repository

import (
	"os"
	"testing"
	"time"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

func setupDB(t *testing.T) *database.DB {
	t.Helper()
	dir, err := os.MkdirTemp("", "certwatch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	db, err := database.Open("sqlite", dir+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUserRepository(t *testing.T) {
	db := setupDB(t)
	repo := NewUserRepository(db)

	u := &models.User{Email: "test@example.com", Password: "hash", Name: "Test"}
	if err := repo.Create(u); err != nil {
		t.Fatal(err)
	}

	found, err := repo.FindByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Email != u.Email {
		t.Errorf("expected %s, got %s", u.Email, found.Email)
	}

	found2, err := repo.FindByEmail("test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if found2.ID != u.ID {
		t.Errorf("expected %d, got %d", u.ID, found2.ID)
	}

	u.Name = "Updated"
	if err := repo.Update(u); err != nil {
		t.Fatal(err)
	}
	found3, _ := repo.FindByID(u.ID)
	if found3.Name != "Updated" {
		t.Errorf("expected Updated, got %s", found3.Name)
	}
}

func TestDomainRepository(t *testing.T) {
	db := setupDB(t)
	repo := NewDomainRepository(db)

	d := &models.Domain{Domain: "example.com", Description: "Test", Enabled: true}
	if err := repo.Create(d); err != nil {
		t.Fatal(err)
	}

	found, err := repo.FindByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Domain != "example.com" {
		t.Errorf("expected example.com, got %s", found.Domain)
	}

	found2, err := repo.FindByDomain("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if found2.ID != d.ID {
		t.Errorf("expected %d, got %d", d.ID, found2.ID)
	}

	domains, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 {
		t.Errorf("expected 1 domain, got %d", len(domains))
	}

	enabled, err := repo.ListEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 1 {
		t.Errorf("expected 1 enabled domain, got %d", len(enabled))
	}

	d.Description = "Updated"
	if err := repo.Update(d); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	_, err = repo.FindByID(d.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestCertificateRepository(t *testing.T) {
	db := setupDB(t)
	domainRepo := NewDomainRepository(db)
	certRepo := NewCertificateRepository(db)

	d := &models.Domain{Domain: "example.com", Enabled: true}
	if err := domainRepo.Create(d); err != nil {
		t.Fatal(err)
	}

	c := &models.Certificate{
		DomainID:    d.ID,
		Issuer:      "Test CA",
		Subject:     "CN=example.com",
		Serial:      "01",
		Fingerprint: "abcdef",
		Protocol:    "https",
		Status:      "valid",
	}
	if err := certRepo.Create(c); err != nil {
		t.Fatal(err)
	}

	found, err := certRepo.FindByID(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Subject != c.Subject {
		t.Errorf("expected %s, got %s", c.Subject, found.Subject)
	}

	byDomain, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byDomain) != 1 {
		t.Errorf("expected 1 cert, got %d", len(byDomain))
	}

	latest, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 || latest[0].ID != c.ID {
		t.Errorf("expected cert %d for domain, got %v", c.ID, latest)
	}

	all, err := certRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 cert, got %d", len(all))
	}

	c.Status = "expired"
	if err := certRepo.Update(c); err != nil {
		t.Fatal(err)
	}

	if err := certRepo.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	_, err = certRepo.FindByID(c.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDeleteExpiredCertificates(t *testing.T) {
	db := setupDB(t)
	domainRepo := NewDomainRepository(db)
	certRepo := NewCertificateRepository(db)

	d := &models.Domain{Domain: "example.com", Enabled: true}
	if err := domainRepo.Create(d); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	// Expired cert (in the past)
	expired := &models.Certificate{
		DomainID:    d.ID,
		Issuer:      "Old CA",
		Subject:     "CN=example.com",
		Serial:      "001",
		NotBefore:   now.Add(-365 * 24 * time.Hour),
		NotAfter:    now.Add(-1 * time.Hour),
		Fingerprint: "old-fingerprint",
		Protocol:    "https",
		Status:      "expired",
		LastChecked: now.Add(-2 * time.Hour),
	}
	if err := certRepo.Create(expired); err != nil {
		t.Fatal(err)
	}

	// Valid cert (in the future)
	valid := &models.Certificate{
		DomainID:    d.ID,
		Issuer:      "Let's Encrypt",
		Subject:     "CN=example.com",
		Serial:      "002",
		NotBefore:   now.Add(-30 * 24 * time.Hour),
		NotAfter:    now.Add(30 * 24 * time.Hour),
		Fingerprint: "valid-fingerprint",
		Protocol:    "https",
		Status:      "valid",
		LastChecked: now,
	}
	if err := certRepo.Create(valid); err != nil {
		t.Fatal(err)
	}

	n, err := certRepo.DeleteExpired()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired cert deleted, got %d", n)
	}

	all, err := certRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 cert remaining, got %d", len(all))
	}
	if all[0].Fingerprint != "valid-fingerprint" {
		t.Errorf("expected valid cert to remain, got %s", all[0].Fingerprint)
	}
}

func TestDeleteExpiredByDomain(t *testing.T) {
	db := setupDB(t)
	domainRepo := NewDomainRepository(db)
	certRepo := NewCertificateRepository(db)

	d1 := &models.Domain{Domain: "example.com", Enabled: true}
	d2 := &models.Domain{Domain: "example.org", Enabled: true}
	if err := domainRepo.Create(d1); err != nil {
		t.Fatal(err)
	}
	if err := domainRepo.Create(d2); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	// Expired cert for domain 1
	e1 := &models.Certificate{
		DomainID:    d1.ID,
		Issuer:      "Old CA",
		Serial:      "001",
		NotAfter:    now.Add(-1 * time.Hour),
		Fingerprint: "old-d1",
		Status:      "expired",
		LastChecked: now.Add(-2 * time.Hour),
	}
	if err := certRepo.Create(e1); err != nil {
		t.Fatal(err)
	}

	// Valid cert for domain 1
	v1 := &models.Certificate{
		DomainID:    d1.ID,
		Issuer:      "Let's Encrypt",
		Serial:      "002",
		NotAfter:    now.Add(30 * 24 * time.Hour),
		Fingerprint: "valid-d1",
		Status:      "valid",
		LastChecked: now,
	}
	if err := certRepo.Create(v1); err != nil {
		t.Fatal(err)
	}

	// Expired cert for domain 2 (should NOT be deleted)
	e2 := &models.Certificate{
		DomainID:    d2.ID,
		Issuer:      "Old CA",
		Serial:      "003",
		NotAfter:    now.Add(-1 * time.Hour),
		Fingerprint: "old-d2",
		Status:      "expired",
		LastChecked: now.Add(-2 * time.Hour),
	}
	if err := certRepo.Create(e2); err != nil {
		t.Fatal(err)
	}

	// Delete expired only for domain 1
	n, err := certRepo.DeleteExpiredByDomain(d1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired cert deleted for domain 1, got %d", n)
	}

	// Domain 1 should have 1 cert (the valid one)
	d1certs, err := certRepo.ListByDomainID(d1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d1certs) != 1 {
		t.Errorf("expected 1 cert for domain 1, got %d", len(d1certs))
	}
	if d1certs[0].Fingerprint != "valid-d1" {
		t.Errorf("expected valid cert for domain 1, got %s", d1certs[0].Fingerprint)
	}

	// Domain 2 should still have its expired cert (untouched)
	d2certs, err := certRepo.ListByDomainID(d2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d2certs) != 1 {
		t.Errorf("expected 1 cert for domain 2, got %d", len(d2certs))
	}
	if d2certs[0].Fingerprint != "old-d2" {
		t.Errorf("expected old cert for domain 2 to remain, got %s", d2certs[0].Fingerprint)
	}
}
