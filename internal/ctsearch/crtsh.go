package ctsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// crtShProvider queries the crt.sh public certificate search service. crt.sh
// aggregates certificates from the public CT logs and exposes a simple JSON
// API that supports domain as well as serial/fingerprint text search.
type crtShProvider struct {
	baseURL string
	client  *http.Client
}

// NewCRTShProvider builds a crt.sh-backed provider. baseURL defaults to
// https://crt.sh when empty. client must be non-nil.
func NewCRTShProvider(baseURL string, client *http.Client) Provider {
	if baseURL == "" {
		baseURL = "https://crt.sh"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if client == nil {
		client = http.DefaultClient
	}
	return &crtShProvider{baseURL: baseURL, client: client}
}

func (p *crtShProvider) Name() string { return "crt.sh" }

func (p *crtShProvider) SearchByDomain(ctx context.Context, domain string, _ bool) ([]Entry, error) {
	return p.search(ctx, domain)
}

func (p *crtShProvider) search(ctx context.Context, q string) ([]Entry, error) {
	u := fmt.Sprintf("%s/?q=%s&output=json", p.baseURL, url.QueryEscape(q))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("crt.sh: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CertWatch/ctsearch")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crt.sh: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crt.sh: unexpected status %d", resp.StatusCode)
	}

	// crt.sh can return very large payloads for popular domains; cap to avoid
	// unbounded memory use.
	limited := io.LimitReader(resp.Body, 10<<20)
	var raw []crtShRow
	if err := json.NewDecoder(limited).Decode(&raw); err != nil {
		return nil, fmt.Errorf("crt.sh: decode response: %w", err)
	}

	entries := make([]Entry, 0, len(raw))
	for _, r := range raw {
		entries = append(entries, r.toEntry(time.Now()))
	}
	return entries, nil
}

// crtShRow mirrors the crt.sh `output=json` schema.
type crtShRow struct {
	IssuerName string `json:"issuer_name"`
	CommonName string `json:"common_name"`
	NameValue  string `json:"name_value"`
	SerialNum  string `json:"serial_number"`
	NotBefore  string `json:"not_before"`
	NotAfter   string `json:"not_after"`
}

func (r crtShRow) toEntry(now time.Time) Entry {
	names := splitNames(r.NameValue)
	if len(names) == 0 {
		names = []string{r.CommonName}
	}
	e := Entry{
		CommonName: r.CommonName,
		SANs:       names,
		Issuer:     r.IssuerName,
		Serial:     r.SerialNum,
		NotBefore:  parseCTTime(r.NotBefore),
		NotAfter:   parseCTTime(r.NotAfter),
		Source:     "crt.sh",
	}
	e.populate(now)
	return e
}

// splitNames splits a crt.sh `name_value` field (newline separated DNS names)
// into a trimmed, de-duplicated list.
func splitNames(nameValue string) []string {
	if strings.TrimSpace(nameValue) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, part := range strings.Split(nameValue, "\n") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lc := strings.ToLower(part)
		if _, ok := seen[lc]; ok {
			continue
		}
		seen[lc] = struct{}{}
		out = append(out, lc)
	}
	return out
}

// parseCTTime parses the two date layouts used by CT search providers
// (YYYY-MM-DDTHH:MM:SS and YYYY-MM-DD). It returns the zero time when the
// value cannot be parsed.
func parseCTTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
