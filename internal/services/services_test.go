package services

import (
	"context"
	"os"
	"testing"

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

	return NewDomainService(domainRepo, certRepo, scannerReg, tagRepo),
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
