package ctsearch

import (
	"testing"
	"time"
)

func TestEntryCoversExactAndWildcard(t *testing.T) {
	e := Entry{SANs: []string{"example.com", "www.example.com"}}
	if !e.Covers("example.com") {
		t.Error("expected exact match")
	}
	if !e.Covers("WWW.EXAMPLE.COM") {
		t.Error("expected case-insensitive match")
	}
	if e.Covers("evil-example.com") {
		t.Error("expected no substring match")
	}

	w := Entry{SANs: []string{"*.example.com"}}
	if !w.Covers("api.example.com") {
		t.Error("expected wildcard to cover subdomain")
	}
	if !w.Covers("example.com") {
		t.Error("expected wildcard to cover bare domain")
	}
	if w.Covers("api.other.com") {
		t.Error("expected wildcard not to cross registrable domain")
	}
}

func TestEntryKeyAndFingerprintAreStable(t *testing.T) {
	a := Entry{Serial: "S1", Issuer: "CA", SANs: []string{"example.com"}}
	b := Entry{Serial: "S1", Issuer: "CA", CommonName: "example.com"}
	if a.Key() != b.Key() {
		t.Error("expected identical keys for equivalent entries")
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("expected identical fingerprints for equivalent entries")
	}
	if len(a.Fingerprint()) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(a.Fingerprint()))
	}
}

func TestEntryStatusAndDays(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	future := Entry{NotBefore: now.AddDate(0, 0, -1), NotAfter: now.Add(24 * time.Hour)}
	if got := future.StatusAt(now); got != "valid" {
		t.Errorf("expected valid, got %s", got)
	}
	if got := future.DaysUntil(now); got != 1 {
		t.Errorf("expected 1 day, got %d", got)
	}

	expired := Entry{NotAfter: now.Add(-2 * time.Hour)}
	if got := expired.StatusAt(now); got != "expired" {
		t.Errorf("expected expired, got %s", got)
	}

	notYet := Entry{NotBefore: now.Add(2 * time.Hour)}
	if got := notYet.StatusAt(now); got != "not-yet-valid" {
		t.Errorf("expected not-yet-valid, got %s", got)
	}
}

func TestSplitNames(t *testing.T) {
	names := splitNames("example.com\nwww.example.com\nexample.com\n ")
	if len(names) != 2 {
		t.Fatalf("expected 2 unique names, got %d: %v", len(names), names)
	}
	if names[0] != "example.com" {
		t.Errorf("expected example.com first, got %s", names[0])
	}
	if splitNames("  ") != nil {
		t.Error("expected nil for empty input")
	}
}
