package services

import (
	"os"
	"testing"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/discovery"
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

	scannerReg := discovery.NewRegistry()
	scannerReg.Register(discovery.NewHTTPSScanner(0))

	return NewDomainService(domainRepo, certRepo, scannerReg),
		NewCertificateService(certRepo, domainRepo),
		NewAuthService(userRepo, nil)
}

func TestAddDomain(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	d, err := domainSvc.AddDomain("example.com", "test")
	if err != nil {
		t.Fatal(err)
	}
	if d.Domain != "example.com" {
		t.Errorf("expected example.com, got %s", d.Domain)
	}
}

func TestAddDomainDuplicate(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	_, err := domainSvc.AddDomain("example.com", "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = domainSvc.AddDomain("example.com", "test")
	if err == nil {
		t.Error("expected error for duplicate domain")
	}
}

func TestAddDomainEmpty(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	_, err := domainSvc.AddDomain("", "test")
	if err == nil {
		t.Error("expected error for empty domain")
	}
}

func TestListDomains(t *testing.T) {
	domainSvc, _, _ := setupServices(t)
	domainSvc.AddDomain("example.com", "test")
	domainSvc.AddDomain("example.org", "test2")

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
	d, _ := domainSvc.AddDomain("example.com", "test")
	if err := domainSvc.DeleteDomain(d.ID); err != nil {
		t.Fatal(err)
	}
	_, err := domainSvc.GetDomain(d.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}
