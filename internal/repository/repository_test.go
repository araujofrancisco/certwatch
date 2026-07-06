package repository

import (
	"os"
	"testing"

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

	users, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}

	u.Name = "Updated"
	if err := repo.Update(u); err != nil {
		t.Fatal(err)
	}
	found3, _ := repo.FindByID(u.ID)
	if found3.Name != "Updated" {
		t.Errorf("expected Updated, got %s", found3.Name)
	}

	if err := repo.Delete(u.ID); err != nil {
		t.Fatal(err)
	}
	_, err = repo.FindByID(u.ID)
	if err == nil {
		t.Error("expected error after delete")
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

	latest, err := certRepo.LatestForDomain(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != c.ID {
		t.Errorf("expected %d, got %d", c.ID, latest.ID)
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

func TestNotificationProfileRepository(t *testing.T) {
	db := setupDB(t)
	repo := NewNotificationProfileRepository(db)

	p := &models.NotificationProfile{
		Name:       "Operations",
		Type:       "immediate",
		Enabled:    true,
		Recipients: "ops@example.com",
		Config:     `{"thresholds":[30,14,7,3,1]}`,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}

	found, err := repo.FindByID(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Name != "Operations" {
		t.Errorf("expected Operations, got %s", found.Name)
	}

	profiles, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(profiles))
	}

	p.Enabled = false
	if err := repo.Update(p); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
}
