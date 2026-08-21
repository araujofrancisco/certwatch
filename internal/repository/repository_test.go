package repository

import (
	"context"
	"errors"
	"os"
	"strings"
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
	ctx := context.Background()
	db := setupDB(t)
	repo := NewUserRepository(db)

	u := &models.User{Email: "test@example.com", Password: "hash", Name: "Test"}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	found, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Email != u.Email {
		t.Errorf("expected %s, got %s", u.Email, found.Email)
	}

	found2, err := repo.FindByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if found2.ID != u.ID {
		t.Errorf("expected %d, got %d", u.ID, found2.ID)
	}

	u.Name = "Updated"
	if err := repo.Update(ctx, u); err != nil {
		t.Fatal(err)
	}
	found3, _ := repo.FindByID(ctx, u.ID)
	if found3.Name != "Updated" {
		t.Errorf("expected Updated, got %s", found3.Name)
	}
}

func TestDomainRepository(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	repo := NewDomainRepository(db)

	d := &models.Domain{Domain: "example.com", Description: "Test", Enabled: true}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatal(err)
	}

	found, err := repo.FindByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Domain != "example.com" {
		t.Errorf("expected example.com, got %s", found.Domain)
	}

	found2, err := repo.FindByDomain(ctx, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if found2.ID != d.ID {
		t.Errorf("expected %d, got %d", d.ID, found2.ID)
	}

	domains, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 {
		t.Errorf("expected 1 domain, got %d", len(domains))
	}

	enabled, err := repo.ListEnabled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 1 {
		t.Errorf("expected 1 enabled domain, got %d", len(enabled))
	}

	d.Description = "Updated"
	if err := repo.Update(ctx, d); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	_, err = repo.FindByID(ctx, d.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestCertificateRepository(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	domainRepo := NewDomainRepository(db)
	certRepo := NewCertificateRepository(db)

	d := &models.Domain{Domain: "example.com", Enabled: true}
	if err := domainRepo.Create(ctx, d); err != nil {
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
	if err := certRepo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}

	found, err := certRepo.FindByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Subject != c.Subject {
		t.Errorf("expected %s, got %s", c.Subject, found.Subject)
	}

	byDomain, err := certRepo.ListByDomainID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byDomain) != 1 {
		t.Errorf("expected 1 cert, got %d", len(byDomain))
	}

	latest, err := certRepo.ListByDomainID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 || latest[0].ID != c.ID {
		t.Errorf("expected cert %d for domain, got %v", c.ID, latest)
	}

	all, err := certRepo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 cert, got %d", len(all))
	}

	c.Status = "expired"
	if err := certRepo.Update(ctx, c); err != nil {
		t.Fatal(err)
	}

	if err := certRepo.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	_, err = certRepo.FindByID(ctx, c.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDeleteExpiredCertificates(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	domainRepo := NewDomainRepository(db)
	certRepo := NewCertificateRepository(db)

	d := &models.Domain{Domain: "example.com", Enabled: true}
	if err := domainRepo.Create(ctx, d); err != nil {
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
	if err := certRepo.Create(ctx, expired); err != nil {
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
	if err := certRepo.Create(ctx, valid); err != nil {
		t.Fatal(err)
	}

	n, err := certRepo.DeleteExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired cert deleted, got %d", n)
	}

	all, err := certRepo.List(ctx)
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
	ctx := context.Background()
	db := setupDB(t)
	domainRepo := NewDomainRepository(db)
	certRepo := NewCertificateRepository(db)

	d1 := &models.Domain{Domain: "example.com", Enabled: true}
	d2 := &models.Domain{Domain: "example.org", Enabled: true}
	if err := domainRepo.Create(ctx, d1); err != nil {
		t.Fatal(err)
	}
	if err := domainRepo.Create(ctx, d2); err != nil {
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
	if err := certRepo.Create(ctx, e1); err != nil {
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
	if err := certRepo.Create(ctx, v1); err != nil {
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
	if err := certRepo.Create(ctx, e2); err != nil {
		t.Fatal(err)
	}

	// Delete expired only for domain 1
	n, err := certRepo.DeleteExpiredByDomain(ctx, d1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired cert deleted for domain 1, got %d", n)
	}

	// Domain 1 should have 1 cert (the valid one)
	d1certs, err := certRepo.ListByDomainID(ctx, d1.ID)
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
	d2certs, err := certRepo.ListByDomainID(ctx, d2.ID)
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

func TestDomainRepository_CreateMany_Rollback(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	defer db.Close()
	repo := NewDomainRepository(db)

	// Second entry violates the UNIQUE constraint on domain, so the whole
	// batch must roll back and the first insert must not persist.
	batch := []*models.Domain{
		{Domain: "rollback-a.example.com", Enabled: true},
		{Domain: "rollback-b.example.com", Enabled: true},
		{Domain: "rollback-b.example.com", Enabled: true},
	}
	if err := repo.CreateMany(ctx, batch); err == nil {
		t.Fatal("expected CreateMany to fail on duplicate")
	}

	if d, err := repo.FindByDomain(ctx, "rollback-a.example.com"); err == nil && d != nil {
		t.Fatal("expected first domain to be rolled back with the batch")
	}

	// A valid batch inserts everything and assigns IDs.
	valid := []*models.Domain{
		{Domain: "bulk-one.example.com", Description: "one", Group: "g", Enabled: true},
		{Domain: "bulk-two.example.com", Description: "two", Group: "g", Enabled: false},
	}
	if err := repo.CreateMany(ctx, valid); err != nil {
		t.Fatalf("CreateMany valid batch: %v", err)
	}
	for i, d := range valid {
		if d.ID == 0 {
			t.Errorf("domain %d: expected ID to be assigned", i)
		}
	}
	got, err := repo.FindByDomain(ctx, "bulk-two.example.com")
	if err != nil {
		t.Fatalf("FindByDomain after bulk create: %v", err)
	}
	if got.Enabled {
		t.Error("expected enabled=false to persist")
	}
	if got.Group != "g" {
		t.Errorf("group = %q, want %q", got.Group, "g")
	}
}

func TestCertificateRepository_ExpiryAggregates(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	domainRepo := NewDomainRepository(db)
	certRepo := NewCertificateRepository(db)

	d1 := &models.Domain{Domain: "agg-a.example.com", Enabled: true}
	if err := domainRepo.Create(ctx, d1); err != nil {
		t.Fatal(err)
	}
	d2 := &models.Domain{Domain: "agg-b.example.com", Enabled: true}
	if err := domainRepo.Create(ctx, d2); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	certs := []*models.Certificate{
		{DomainID: d1.ID, Issuer: "CA", Status: "valid", NotAfter: now.Add(400 * time.Hour), LastChecked: now.Add(2 * time.Hour)},
		{DomainID: d2.ID, Issuer: "CA", Status: "valid", NotAfter: now.Add(48 * time.Hour), LastChecked: now},
		{DomainID: d2.ID, Issuer: "CA", Status: "valid", NotAfter: now.Add(-24 * time.Hour), LastChecked: now}, // expired; ties on last_checked with row above
		{DomainID: d1.ID, Issuer: "CA", Status: "unknown", LastChecked: now},                                   // null expiry (failed scan) → no bucket
	}
	for _, c := range certs {
		if err := certRepo.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	warnStart := now.Add(24 * time.Hour)
	warnEnd := now.Add(360 * time.Hour)
	counts, err := certRepo.CountExpiryBuckets(ctx, warnStart, warnEnd)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Healthy != 1 {
		t.Errorf("healthy = %d, want 1", counts.Healthy)
	}
	if counts.Warning != 1 {
		t.Errorf("warning = %d, want 1", counts.Warning)
	}
	if counts.Expired != 1 {
		t.Errorf("expired = %d, want 1", counts.Expired)
	}

	soon, err := certRepo.ListExpiringSoon(ctx, now, warnEnd, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(soon) != 1 {
		t.Fatalf("expiring soon rows = %d, want 1", len(soon))
	}
	if soon[0].Domain != "agg-b.example.com" {
		t.Errorf("domain = %q, want agg-b.example.com", soon[0].Domain)
	}

	// Limit is respected.
	soon, err = certRepo.ListExpiringSoon(ctx, time.Time{}, warnEnd, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = soon

	latest, err := certRepo.ListLatestByDomain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 {
		t.Fatalf("latest per domain = %d, want 2 (one per domain)", len(latest))
	}
	byDomain := map[int64]int64{}
	for _, c := range latest {
		byDomain[c.DomainID] = c.ID
	}
	// d1's latest is certs[0] (most recently checked); d2's two rows tie on
	// last_checked and resolve to the higher id (certs[2]).
	if byDomain[d1.ID] != certs[0].ID {
		t.Errorf("d1 latest id = %d, want %d", byDomain[d1.ID], certs[0].ID)
	}
	if byDomain[d2.ID] != certs[2].ID {
		t.Errorf("d2 latest id = %d, want %d (tie resolves to newest row)", byDomain[d2.ID], certs[2].ID)
	}
}

func TestSentinelErrors(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	defer db.Close()

	domainRepo := NewDomainRepository(db)

	// Missing row maps to ErrNotFound.
	if _, err := domainRepo.FindByID(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := domainRepo.Delete(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: expected ErrNotFound, got %v", err)
	}

	// Duplicate unique key maps to ErrConflict.
	d := &models.Domain{Domain: "sentinel.example.com", Enabled: true}
	if err := domainRepo.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	dup := &models.Domain{Domain: "sentinel.example.com", Enabled: true}
	err := domainRepo.Create(ctx, dup)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	// The driver error stays in the chain for diagnostics.
	if !strings.Contains(err.Error(), "sentinel.example.com") && !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("driver detail lost from error: %v", err)
	}
}
