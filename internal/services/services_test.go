package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/models"
	"github.com/araujofrancisco/certwatch/internal/repository"
)

func setupServices(t *testing.T) (*DomainService, *CertificateService, *AuthService) {
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

	userRepo := repository.NewUserRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	scannerReg := discovery.NewRegistry()
	scannerReg.Register(discovery.NewHTTPSScanner(0))

	return NewDomainService(domainRepo, certRepo, scannerReg, tagRepo, context.Background()),
		NewCertificateService(certRepo, domainRepo),
		NewAuthService(userRepo, nil)
}

func TestAddDomain(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	d, err := domainSvc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Domain != "example.com" {
		t.Errorf("expected example.com, got %s", d.Domain)
	}
}

func TestAddDomainDuplicate(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	_, err := domainSvc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = domainSvc.AddDomain("example.com", "test", "")
	if err == nil {
		t.Error("expected error for duplicate domain")
	}
}

func TestAddDomainEmpty(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	_, err := domainSvc.AddDomain("", "test", "")
	if err == nil {
		t.Error("expected error for empty domain")
	}
}

func TestListDomains(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	_, _ = domainSvc.AddDomain("example.com", "test", "")
	_, _ = domainSvc.AddDomain("example.org", "test2", "")

	domains, err := domainSvc.ListDomains()
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(domains))
	}
}

func TestDeleteDomain(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	d, _ := domainSvc.AddDomain("example.com", "test", "")
	if err := domainSvc.DeleteDomain(d.ID); err != nil {
		t.Fatal(err)
	}
	_, err := domainSvc.GetDomain(d.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestUpdateDomain(t *testing.T) {
	svc, _, _ := setupServices(t)
	d, _ := svc.AddDomain("example.com", "original", "group-a")
	updated, err := svc.UpdateDomain(d.ID, "example.org", "updated", "group-b", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Domain != "example.org" {
		t.Errorf("expected example.org, got %s", updated.Domain)
	}
	if updated.Description != "updated" {
		t.Errorf("expected updated, got %s", updated.Description)
	}
	if updated.Group != "group-b" {
		t.Errorf("expected group-b, got %s", updated.Group)
	}
	if updated.Enabled {
		t.Error("expected disabled")
	}
}

func TestUpdateDomainWithTags(t *testing.T) {
	svc, _, _ := setupServices(t)
	d, _ := svc.AddDomain("example.com", "", "")
	_, err := svc.UpdateDomain(d.ID, "example.com", "", "", true, []string{"production"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetDomain(d.ID)
	if len(got.Tags) != 1 || got.Tags[0].Name != "production" {
		t.Errorf("expected 1 tag 'production', got %v", got.Tags)
	}
}

func TestSetDomainTags(t *testing.T) {
	svc, _, _ := setupServices(t)
	d, _ := svc.AddDomain("example.com", "", "")

	if err := svc.SetDomainTags(d.ID, []string{"prod", "us-east"}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetDomain(d.ID)
	if len(got.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got.Tags))
	}
	if err := svc.SetDomainTags(d.ID, []string{"prod"}); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetDomain(d.ID)
	if len(got.Tags) != 1 {
		t.Errorf("expected 1 tag after replace, got %d", len(got.Tags))
	}
}

func TestBulkAddDomains(t *testing.T) {
	svc, _, _ := setupServices(t)
	entries := []BulkDomainEntry{
		{Domain: "example.com"},
		{Domain: "example.org", Tags: []string{"internal"}},
		{Domain: "", Description: "empty"},
		{Domain: "not-valid-", Description: "bad"},
	}
	resp := svc.BulkAddDomains(entries)
	if resp.Summary.Total != 4 {
		t.Errorf("expected 4 total, got %d", resp.Summary.Total)
	}
	if resp.Summary.Created != 2 {
		t.Errorf("expected 2 created, got %d", resp.Summary.Created)
	}
	if resp.Summary.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", resp.Summary.Skipped)
	}
	if resp.Summary.Errors != 1 {
		t.Errorf("expected 1 errors, got %d", resp.Summary.Errors)
	}
}

func TestScanAllDomainsEmpty(t *testing.T) {
	svc, _, _ := setupServices(t)
	certs, err := svc.ScanAllDomains(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 0 {
		t.Errorf("expected 0 certs, got %d", len(certs))
	}
}

func TestListDomainsFiltered(t *testing.T) {
	svc, _, _ := setupServices(t)
	_, _ = svc.AddDomain("example.com", "desc one", "")
	_, _ = svc.AddDomain("example.org", "desc two", "")

	filtered, err := svc.ListDomainsFiltered(models.DomainFilter{Query: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered result, got %d", len(filtered))
	}
	enabled := true
	allEnabled, err := svc.ListDomainsFiltered(models.DomainFilter{Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if len(allEnabled) != 2 {
		t.Errorf("expected 2 enabled domains, got %d", len(allEnabled))
	}
}

func TestPurgeExpired(t *testing.T) {
	domainSvc, certSvc, _ := setupServices(t)

	d, _ := domainSvc.AddDomain("example.com", "", "")
	now := time.Now().UTC()

	// Expired cert
	expired := &models.Certificate{
		DomainID:    d.ID,
		Issuer:      "Old CA",
		Serial:      "001",
		NotAfter:    now.Add(-1 * time.Hour),
		Fingerprint: "old-fingerprint",
		Protocol:    "https",
		Status:      "expired",
		LastChecked: now.Add(-2 * time.Hour),
	}
	if err := domainSvc.certs.Create(expired); err != nil {
		t.Fatal(err)
	}

	// Valid cert
	valid := &models.Certificate{
		DomainID:    d.ID,
		Issuer:      "Let's Encrypt",
		Serial:      "002",
		NotAfter:    now.Add(30 * 24 * time.Hour),
		Fingerprint: "valid-fingerprint",
		Protocol:    "https",
		Status:      "valid",
		LastChecked: now,
	}
	if err := domainSvc.certs.Create(valid); err != nil {
		t.Fatal(err)
	}

	n, err := certSvc.PurgeExpired()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired cert purged, got %d", n)
	}

	all, err := certSvc.ListCertificates()
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

func TestPurgeExpiredByDomain(t *testing.T) {
	domainSvc, certSvc, _ := setupServices(t)

	d1, _ := domainSvc.AddDomain("example.com", "", "")
	d2, _ := domainSvc.AddDomain("example.org", "", "")
	now := time.Now().UTC()

	// Expired cert for domain 1
	e1 := &models.Certificate{
		DomainID:    d1.ID,
		Serial:      "001",
		NotAfter:    now.Add(-1 * time.Hour),
		Fingerprint: "old-d1",
		Status:      "expired",
		LastChecked: now.Add(-2 * time.Hour),
	}
	if err := domainSvc.certs.Create(e1); err != nil {
		t.Fatal(err)
	}

	// Expired cert for domain 2 (should survive)
	e2 := &models.Certificate{
		DomainID:    d2.ID,
		Serial:      "003",
		NotAfter:    now.Add(-1 * time.Hour),
		Fingerprint: "old-d2",
		Status:      "expired",
		LastChecked: now.Add(-2 * time.Hour),
	}
	if err := domainSvc.certs.Create(e2); err != nil {
		t.Fatal(err)
	}

	n, err := certSvc.PurgeExpiredByDomain(d1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired cert purged for domain 1, got %d", n)
	}

	d1Certs, err := certSvc.ListByDomain(d1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d1Certs) != 0 {
		t.Errorf("expected 0 certs for domain 1, got %d", len(d1Certs))
	}

	d2Certs, err := certSvc.ListByDomain(d2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d2Certs) != 1 {
		t.Errorf("expected 1 cert for domain 2 (untouched), got %d", len(d2Certs))
	}
}

func TestSaveCertificatePurgesExpiredOnRenewal(t *testing.T) {
	domainSvc, certSvc, _ := setupServices(t)
	d, _ := domainSvc.AddDomain("example.com", "", "")
	now := time.Now().UTC()

	// Pre-existing expired cert (simulating an old cert that has since been renewed)
	oldCert := &models.Certificate{
		DomainID:    d.ID,
		Issuer:      "Old CA",
		Serial:      "old-serial",
		NotBefore:   now.Add(-365 * 24 * time.Hour),
		NotAfter:    now.Add(-1 * time.Hour),
		Fingerprint: "old-fingerprint",
		Protocol:    "https",
		Status:      "expired",
		LastChecked: now.Add(-2 * time.Hour),
	}
	if err := domainSvc.certs.Create(oldCert); err != nil {
		t.Fatal(err)
	}

	// Simulate a renewal scan — new cert with different fingerprint/serial
	renewalResult := &discovery.Result{
		Subject:     "CN=example.com",
		Issuer:      "Let's Encrypt",
		Serial:      "new-serial",
		NotBefore:   now.Add(-30 * 24 * time.Hour),
		NotAfter:    now.Add(90 * 24 * time.Hour),
		Fingerprint: "new-fingerprint",
		Protocol:    "https",
		Status:      "valid",
	}

	newCert := domainSvc.saveCertificate(d.ID, renewalResult)

	if newCert.ID == 0 {
		t.Fatal("expected new cert to have an ID")
	}
	if newCert.Fingerprint != "new-fingerprint" {
		t.Errorf("expected new cert fingerprint, got %s", newCert.Fingerprint)
	}

	// The old expired cert should have been purged
	certs, err := certSvc.ListByDomain(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert (the renewed one), got %d", len(certs))
	}
	if certs[0].Fingerprint != "new-fingerprint" {
		t.Errorf("expected only the new cert to remain, got %s", certs[0].Fingerprint)
	}
}
