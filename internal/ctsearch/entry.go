package ctsearch

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// Entry is a normalized certificate record returned by a Certificate
// Transparency search provider. Every provider (crt.sh, CertSpotter, ...) is
// normalized into this shape so the rest of the system never depends on a
// single vendor's JSON schema.
type Entry struct {
	// CommonName is the certificate subject common name.
	CommonName string `json:"common_name"`
	// SANs lists all DNS names covered by the certificate.
	SANs []string `json:"san"`
	// Issuer is the human-readable issuer (CA) name.
	Issuer string `json:"issuer"`
	// Serial is the certificate serial number.
	Serial string `json:"serial"`
	// NotBefore / NotAfter bound the certificate validity window.
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	// Source identifies which provider returned this entry (e.g. "crt.sh").
	Source string `json:"source"`

	// Subject is the display subject: the first SAN when present, else CN.
	Subject string `json:"subject"`
	// Status and DaysLeft are precomputed for convenient display. They are
	// derived from NotBefore/NotAfter and are not authoritative persistence
	// state.
	Status   string `json:"status"`
	DaysLeft int    `json:"days_left"`
}

// populate derives display fields that callers could otherwise compute
// themselves (status, subject, days left). Callers that need stricter
// semantics should use Status()/DaysUntil() directly instead.
func (e *Entry) populate(now time.Time) {
	e.Subject = e.SubjectName()
	e.Status = e.StatusAt(now)
	e.DaysLeft = e.DaysUntil(now)
}

// SubjectName returns the first SAN or the common name, used for display and
// deduplication.
func (e Entry) SubjectName() string {
	for _, name := range e.SANs {
		if name != "" {
			return name
		}
	}
	return e.CommonName
}

// Names returns all DNS names covered by the certificate, falling back to the
// common name when no SANs are present.
func (e Entry) Names() []string {
	if len(e.SANs) > 0 {
		return e.SANs
	}
	if e.CommonName != "" {
		return []string{e.CommonName}
	}
	return nil
}

// Covers reports whether this certificate covers the given domain, either by
// exact match or via a wildcard SAN (e.g. *.example.com covers api.example.com).
func (e Entry) Covers(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	for _, name := range e.Names() {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if name == domain {
			return true
		}
		if strings.HasPrefix(name, "*.") {
			suffix := name[1:] // keep leading dot: ".example.com"
			if strings.HasSuffix(domain, suffix) {
				return true
			}
			if domain == name[2:] {
				return true
			}
		}
	}
	return false
}

// Key returns a stable dedup key for a distinct issuance across providers:
// serial | issuer | subject. This mirrors the dedup scheme used by the main
// certificate repository so findings can be correlated with stored certificates.
func (e Entry) Key() string {
	return strings.Join([]string{e.Serial, e.Issuer, e.SubjectName()}, "|")
}

// Fingerprint returns a derived SHA-256 fingerprint of the dedup key. The free
// CT search providers do not expose the raw DER body (which is what a real
// SHA-256-of-DER fingerprint needs), so this derived value is used as a stable
// correlation key for preprocessing and UI display. It is not the standard
// fingerprint of the certificate bytes.
func (e Entry) Fingerprint() string {
	sum := sha256.Sum256([]byte(e.Key()))
	return fmt.Sprintf("%x", sum)
}

// StatusAt classifies the certificate relative to the given instant.
func (e Entry) StatusAt(now time.Time) string {
	if !e.NotBefore.IsZero() && now.Before(e.NotBefore) {
		return "not-yet-valid"
	}
	if !e.NotAfter.IsZero() && now.After(e.NotAfter) {
		return "expired"
	}
	return "valid"
}

// DaysUntil returns the whole number of days remaining until expiry (negative
// after expiry, or a sentinel when the expiry is unknown).
func (e Entry) DaysUntil(now time.Time) int {
	if e.NotAfter.IsZero() {
		return 0
	}
	if e.NotAfter.Equal(now) {
		return 0
	}
	d := e.NotAfter.Sub(now).Hours() / 24
	if d < 0 {
		d -= 0.5 // negative values round toward -inf so "already expired" reads as <= 0
	}
	return int(d)
}
