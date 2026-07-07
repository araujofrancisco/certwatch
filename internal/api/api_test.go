package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/araujofrancisco/certwatch/internal/auth"
	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/middleware"
	"github.com/araujofrancisco/certwatch/internal/repository"
	"github.com/araujofrancisco/certwatch/internal/services"
)

func setupAPI(t *testing.T) (*Handler, *auth.Authenticator, string) {
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

	authenticator := auth.New("test-secret", 24*time.Hour)

	userRepo := repository.NewUserRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	scannerReg := discovery.NewRegistry()
	scannerReg.Register(discovery.NewHTTPSScanner(5 * time.Second))

	authSvc := services.NewAuthService(userRepo, authenticator)
	domainSvc := services.NewDomainService(domainRepo, certRepo, scannerReg, tagRepo)
	certSvc := services.NewCertificateService(certRepo, domainRepo)

	rl := middleware.NewRateLimiter(100, time.Minute)
	handler := NewHandler(domainSvc, certSvc, authSvc, authenticator, db.DB, rl)

	token, _ := authenticator.GenerateToken(1, "admin@test.com")
	return handler, authenticator, token
}

func TestHealthEndpoint(t *testing.T) {
	h, _, _ := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected ok, got %s", body["status"])
	}
}

func TestRegisterAndLogin(t *testing.T) {
	h, _, _ := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"email":"test@example.com","password":"secret123","name":"Test"}`
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}

	body2 := `{"email":"test@example.com","password":"secret123"}`
	req2 := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec2.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&resp)
	if resp["token"] == "" {
		t.Error("expected token in response")
	}
}

func TestCreateDomain(t *testing.T) {
	h, _, token := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"domain":"example.com","description":"test"}`
	req := httptest.NewRequest("POST", "/api/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestListDomains(t *testing.T) {
	h, _, token := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"domain":"example.com","description":"test"}`
	req := httptest.NewRequest("POST", "/api/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	req2 := httptest.NewRequest("GET", "/api/domains", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec2.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&resp)
	domains := resp["domains"].([]any)
	if len(domains) != 1 {
		t.Errorf("expected 1 domain, got %d", len(domains))
	}
}

func TestListCertificates(t *testing.T) {
	h, _, token := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/certificates", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuthRequiredForDomains(t *testing.T) {
	h, _, _ := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/domains", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestVersionEndpoint(t *testing.T) {
	h, _, _ := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/version", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["version"] != "0.1.0" {
		t.Errorf("expected version 0.1.0, got %s", body["version"])
	}
}

func TestDeleteDomainEndpoint(t *testing.T) {
	h, _, token := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"domain":"example.com","description":"test"}`
	req := httptest.NewRequest("POST", "/api/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	req2 := httptest.NewRequest("DELETE", "/api/domains/1", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec2.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec2.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected deleted, got %s", resp["status"])
	}
}

func TestImportDomainsEndpoint(t *testing.T) {
	h, _, token := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"domains":["example.com","example.org"]}`
	req := httptest.NewRequest("POST", "/api/domains/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	summary := resp["summary"].(map[string]any)
	if summary["created"].(float64) != 2 {
		t.Errorf("expected 2 created, got %v", summary["created"])
	}
}

func TestInventoryReportEndpoint(t *testing.T) {
	h, _, token := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"domain":"example.com","description":"test"}`
	req := httptest.NewRequest("POST", "/api/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	req2 := httptest.NewRequest("GET", "/api/reports/inventory", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec2.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&resp)
	summary := resp["summary"].(map[string]any)
	if summary["total_domains"].(float64) != 1 {
		t.Errorf("expected 1 domain, got %v", summary["total_domains"])
	}
	inventory := resp["inventory"].([]any)
	if len(inventory) != 1 {
		t.Errorf("expected 1 entry, got %d", len(inventory))
	}
}

func TestPurgeCertificateErrorsEndpoint(t *testing.T) {
	h, _, token := setupAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", "/api/certificates/errors", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
