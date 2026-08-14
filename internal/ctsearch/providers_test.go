package ctsearch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCRTShParsing(t *testing.T) {
	body := `[
	  {
	    "issuer_name": "R3",
	    "common_name": "example.com",
	    "name_value": "example.com\nwww.example.com",
	    "serial_number": "0FAB12",
	    "not_before": "2024-06-01T00:00:00",
	    "not_after": "2024-09-01T00:00:00"
	  }
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("output") != "json" {
			t.Errorf("expected output=json, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	p := NewCRTShProvider(srv.URL, srv.Client())
	res, err := p.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(res))
	}
	e := res[0]
	if e.SubjectName() != "example.com" {
		t.Errorf("expected subject example.com, got %s", e.SubjectName())
	}
	if len(e.SANs) != 2 {
		t.Errorf("expected 2 SANs, got %d", len(e.SANs))
	}
	if e.Issuer != "R3" {
		t.Errorf("expected issuer R3, got %s", e.Issuer)
	}
	if !e.NotAfter.Equal(time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected not_after: %v", e.NotAfter)
	}
	if e.Source != "crt.sh" {
		t.Errorf("expected source crt.sh, got %s", e.Source)
	}
	if e.Status != "expired" {
		t.Errorf("expected status expired for 2024 cert, got %s", e.Status)
	}
}

func TestCRTShHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	p := NewCRTShProvider(srv.URL, srv.Client())
	if _, err := p.SearchByDomain(context.Background(), "example.com", false); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestCertSpotterParsingEnvelope(t *testing.T) {
	body := `{
	  "issuances": [{
	    "id": 123,
	    "serial_number": "04:AA:BB",
	    "common_name": "example.com",
	    "name_value": "example.com\n*.example.com",
	    "not_before": "2024-06-01T00:00:00Z",
	    "not_after": "2025-06-01T00:00:00Z",
	    "issuer": {"id": 1, "name": "Let's Encrypt"}
	  }]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("include_subdomains") != "true" {
			t.Errorf("expected include_subdomains=true, got %q", r.URL.Query().Get("include_subdomains"))
		}
		if r.URL.Query().Get("expand") != "issuer" {
			t.Errorf("expected expand=issuer, got %q", r.URL.Query().Get("expand"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	p := NewCertSpotterProvider(srv.URL, "tok", srv.Client())
	res, err := p.SearchByDomain(context.Background(), "example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(res))
	}
	e := res[0]
	if e.Source != "CertSpotter" {
		t.Errorf("expected source CertSpotter, got %s", e.Source)
	}
	if e.Issuer != "Let's Encrypt" {
		t.Errorf("expected issuer Let's Encrypt, got %s", e.Issuer)
	}
	if len(e.SANs) != 2 {
		t.Errorf("expected 2 SANs, got %d", len(e.SANs))
	}
}

func TestCertSpotterRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := NewCertSpotterProvider(srv.URL, "", srv.Client())
	if _, err := p.SearchByDomain(context.Background(), "example.com", false); err == nil {
		t.Fatal("expected 429 error")
	}
}
