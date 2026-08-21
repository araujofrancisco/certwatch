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

	return NewDomainService(domainRepo, certRepo, scannerReg, tagRepo, context.Background(), 3, 100, 30*time.Second),
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
	if err := svc.ScanAllDomains(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type fakeScanner struct {
	result *discovery.Result
	err    error
}

func (f *fakeScanner) Protocol() string { return "https" }

func (f *fakeScanner) Scan(ctx context.Context, domain string) (*discovery.Result, error) {
	return f.result, f.err
}

type fakeMultiScanner struct {
	results []*discovery.Result
	err     error
}

func (f *fakeMultiScanner) Protocol() string { return "ct" }

func (f *fakeMultiScanner) Scan(ctx context.Context, domain string) (*discovery.Result, error) {
	if len(f.results) == 0 {
		return nil, f.err
	}
	return f.results[0], nil
}

func (f *fakeMultiScanner) ScanAll(ctx context.Context, domain string) ([]*discovery.Result, error) {
	return f.results, f.err
}

func TestScanDomainSavesSANs(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	reg := discovery.NewRegistry()
	reg.Register(&fakeScanner{result: &discovery.Result{
		Subject:     "example.com",
		Issuer:      "CA",
		Serial:      "01",
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "abc123",
		Protocol:    "https",
		Status:      "valid",
		SANs:        []string{"example.com", "www.example.com"},
	}})

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	cert, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(cert.SANs) != 2 {
		t.Fatalf("expected 2 SANs, got %v", cert.SANs)
	}

	got, err := svc.GetDomain(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	certs, err := svc.certs.ListByDomainID(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	if len(certs[0].SANs) != 2 || certs[0].SANs[0] != "example.com" {
		t.Errorf("SANs did not round-trip through repository: %v", certs[0].SANs)
	}
}

func TestEnqueueScanRunsDomain(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	reg := discovery.NewRegistry()
	reg.Register(&fakeScanner{result: &discovery.Result{
		Subject:     "example.com",
		Issuer:      "CA",
		Serial:      "01",
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "def456",
		Protocol:    "https",
		Status:      "valid",
		SANs:        []string{"example.com"},
	}})

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 1, 10, time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	svc.EnqueueScan(context.Background(), d.ID, true)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		certs, err := certRepo.ListByDomainID(d.ID)
		if err == nil && len(certs) > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("enqueued scan did not produce a certificate")
}

func TestEnqueueScanBackgroundRunsDomain(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	reg := discovery.NewRegistry()
	reg.Register(&fakeScanner{result: &discovery.Result{
		Subject:     "example.com",
		Issuer:      "CA",
		Serial:      "01",
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "def789",
		Protocol:    "https",
		Status:      "valid",
		SANs:        []string{"example.com"},
	}})

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 1, 10, time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	svc.EnqueueScanBackground(d.ID, true)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		certs, err := certRepo.ListByDomainID(d.ID)
		if err == nil && len(certs) > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("background enqueued scan did not produce a certificate")
}

func TestScanDomainSavesAllCTCertificates(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	now := time.Now()
	ct := &fakeMultiScanner{results: []*discovery.Result{
		{Subject: "a.example.com", Issuer: "CA1", Serial: "01", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), Fingerprint: "fp-a", Protocol: "ct", Status: "valid", SANs: []string{"a.example.com"}},
		{Subject: "b.example.com", Issuer: "CA2", Serial: "02", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), Fingerprint: "fp-b", Protocol: "ct", Status: "valid", SANs: []string{"b.example.com"}},
		{Subject: "c.example.com", Issuer: "CA3", Serial: "03", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), Fingerprint: "fp-c", Protocol: "ct", Status: "valid", SANs: []string{"c.example.com"}},
	}}
	reg := discovery.NewRegistry()
	reg.Register(ct)

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	// First scan: all three certs should be saved.
	if _, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	certs, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 3 {
		t.Fatalf("expected 3 certificates, got %d", len(certs))
	}

	// Second scan: same certs, must not create duplicates.
	if _, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	certs, err = certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 3 {
		t.Fatalf("expected 3 certificates after re-scan, got %d", len(certs))
	}

	fps := map[string]bool{}
	for _, c := range certs {
		fps[c.Fingerprint] = true
	}
	for _, want := range []string{"fp-a", "fp-b", "fp-c"} {
		if !fps[want] {
			t.Errorf("missing certificate fingerprint %s in %v", want, fps)
		}
	}
}

func TestScanDomainDedupesIssuerDNFormatVariants(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	now := time.Now()
	// The same issuance reported twice with the same serial but different
	// issuer DN attribute orders, exactly as ctlogs.dev (CN-first) and
	// CertSpotter (C-first) render the same CA.
	ct := &fakeMultiScanner{results: []*discovery.Result{
		{Subject: "example.com", Issuer: "CN=RapidSSL TLS RSA CA G1,OU=www.digicert.com,O=DigiCert Inc,C=US", Serial: "030150c1d6a9ce829bac11d55e0f097d", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), Fingerprint: "fp-1", Protocol: "ct", Status: "valid", SANs: []string{"example.com"}},
		{Subject: "example.com", Issuer: "C=US, O=DigiCert Inc, OU=www.digicert.com, CN=RapidSSL TLS RSA CA G1", Serial: "030150c1d6a9ce829bac11d55e0f097d", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), Fingerprint: "fp-1", Protocol: "ct", Status: "valid", SANs: []string{"example.com"}},
	}}
	reg := discovery.NewRegistry()
	reg.Register(ct)

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	certs, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 certificate (issuer DN variants deduped), got %d", len(certs))
	}
	if certs[0].Serial != "030150c1d6a9ce829bac11d55e0f097d" {
		t.Errorf("unexpected serial stored: %s", certs[0].Serial)
	}
}

func TestScanDomainDedupesEmptySerialVariant(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	now := time.Now()
	// The same issuance: one provider (ctlogs.dev) reports the serial, the
	// other (CertSpotter) omits it. The fingerprint differs because the dedup
	// key embeds the serial, so the empty-serial issuer+subject fallback must
	// prevent a duplicate row.
	ct := &fakeMultiScanner{results: []*discovery.Result{
		{Subject: "example.com", Issuer: "CN=RapidSSL TLS RSA CA G1,OU=www.digicert.com,O=DigiCert Inc,C=US", Serial: "030150c1d6a9ce829bac11d55e0f097d", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), Fingerprint: "fp-1", Protocol: "ct", Status: "valid", SANs: []string{"example.com"}},
		{Subject: "example.com", Issuer: "C=US, O=DigiCert Inc, OU=www.digicert.com, CN=RapidSSL TLS RSA CA G1", Serial: "", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), Fingerprint: "fp-2", Protocol: "ct", Status: "valid", SANs: []string{"example.com"}},
	}}
	reg := discovery.NewRegistry()
	reg.Register(ct)

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	certs, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 certificate (empty-serial variant deduped), got %d", len(certs))
	}
}

func TestScanDomainDoesNotCollapseDistinctRenewals(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	now := time.Now()
	issuer := "C=GB, O=Sectigo Limited, CN=Sectigo Public Server Authentication CA DV R36"
	// The current valid cert + several distinct renewals: same issuer and
	// subject (*.example.com) but DIFFERENT serials and different expiry days.
	ct := &fakeMultiScanner{results: []*discovery.Result{
		{Subject: "*.example.com", Issuer: issuer, Serial: "A1",
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour),
			Fingerprint: "fp-a1", Protocol: "ct", Status: "valid", SANs: []string{"*.example.com"}},
		{Subject: "*.example.com", Issuer: issuer, Serial: "B2",
			NotBefore: now.Add(-400 * 24 * time.Hour), NotAfter: now.Add(-10 * 24 * time.Hour),
			Fingerprint: "fp-b2", Protocol: "ct", Status: "expired", SANs: []string{"*.example.com"}},
		{Subject: "*.example.com", Issuer: issuer, Serial: "C3",
			NotBefore: now.Add(-800 * 24 * time.Hour), NotAfter: now.Add(-400 * 24 * time.Hour),
			Fingerprint: "fp-c3", Protocol: "ct", Status: "expired", SANs: []string{"*.example.com"}},
	}}
	reg := discovery.NewRegistry()
	reg.Register(ct)

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	certs, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Expired historical renewals must not be ingested at all; only the
	// current valid cert is stored, and it must not be collapsed or
	// overwritten by the expired ones.
	if len(certs) != 1 {
		t.Fatalf("expected 1 certificate (expired ones skipped), got %d", len(certs))
	}
	if certs[0].Fingerprint != "fp-a1" {
		t.Errorf("expected the valid cert fp-a1 to be stored, got %s", certs[0].Fingerprint)
	}
}

func TestScanDomainSkipsExpiredCertificates(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	now := time.Now()
	ct := &fakeMultiScanner{results: []*discovery.Result{
		{Subject: "*.example.com", Issuer: "CA1", Serial: "A1",
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour),
			Fingerprint: "fp-live", Protocol: "ct", Status: "valid", SANs: []string{"*.example.com"}},
		{Subject: "*.example.com", Issuer: "CA2", Serial: "B2",
			NotBefore: now.Add(-400 * 24 * time.Hour), NotAfter: now.Add(-10 * 24 * time.Hour),
			Fingerprint: "fp-old", Protocol: "ct", Status: "expired", SANs: []string{"*.example.com"}},
	}}
	reg := discovery.NewRegistry()
	reg.Register(ct)

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if _, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	certs, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Only the valid cert is stored; repeated scans must not re-add the
	// expired historical one.
	if len(certs) != 1 {
		t.Fatalf("expected 1 certificate after repeated scans, got %d", len(certs))
	}
	if certs[0].Fingerprint != "fp-live" {
		t.Errorf("expected fp-live, got %s", certs[0].Fingerprint)
	}
}

func TestScanDomainAllExpiredSavesNothing(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	now := time.Now()
	ct := &fakeMultiScanner{results: []*discovery.Result{
		{Subject: "*.example.com", Issuer: "CA1", Serial: "A1",
			NotBefore: now.Add(-800 * 24 * time.Hour), NotAfter: now.Add(-400 * 24 * time.Hour),
			Fingerprint: "fp-old1", Protocol: "ct", Status: "expired", SANs: []string{"*.example.com"}},
		{Subject: "*.example.com", Issuer: "CA2", Serial: "B2",
			NotBefore: now.Add(-400 * 24 * time.Hour), NotAfter: now.Add(-10 * 24 * time.Hour),
			Fingerprint: "fp-old2", Protocol: "ct", Status: "expired", SANs: []string{"*.example.com"}},
	}}
	reg := discovery.NewRegistry()
	reg.Register(ct)

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	cert, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second)
	if err != nil {
		t.Fatalf("expected no error when all results are expired, got %v", err)
	}
	if cert != nil {
		t.Errorf("expected nil certificate when all results are expired, got %+v", cert)
	}
	certs, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 0 {
		t.Fatalf("expected 0 stored certificates (no expired ingestion, no error placeholder), got %d", len(certs))
	}
}

func TestScanDomainCombinesHTTPSAndCT(t *testing.T) {
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

	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	now := time.Now()
	reg := discovery.NewRegistry()
	reg.Register(&fakeScanner{result: &discovery.Result{
		Subject: "live.example.com", Issuer: "LiveCA", Serial: "A1",
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(90 * 24 * time.Hour),
		Fingerprint: "fp-live", Protocol: "https", Status: "valid",
	}})
	reg.Register(&fakeMultiScanner{results: []*discovery.Result{
		{Subject: "ct.example.com", Issuer: "CTCA", Serial: "B1",
			NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour),
			Fingerprint: "fp-ct", Protocol: "ct", Status: "valid"},
	}})

	svc := NewDomainService(domainRepo, certRepo, reg, tagRepo, context.Background(), 3, 100, 30*time.Second)
	t.Cleanup(svc.StopScanQueue)

	d, err := svc.AddDomain("example.com", "test", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ScanDomain(context.Background(), d.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	certs, err := certRepo.ListByDomainID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 2 {
		t.Fatalf("expected 2 certificates (https + ct), got %d", len(certs))
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

	d1Certs, err := certSvc.certs.ListByDomainID(d1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d1Certs) != 0 {
		t.Errorf("expected 0 certs for domain 1, got %d", len(d1Certs))
	}

	d2Certs, err := certSvc.certs.ListByDomainID(d2.ID)
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
	certs, err := certSvc.certs.ListByDomainID(d.ID)
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
