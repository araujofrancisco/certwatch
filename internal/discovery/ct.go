package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ctScanner struct {
	timeout time.Duration
	client  *http.Client
}

func NewCTScanner(timeout time.Duration) Scanner {
	ctTimeout := timeout
	if ctTimeout > 10*time.Second {
		ctTimeout = 10 * time.Second
	}
	return &ctScanner{
		timeout: timeout,
		client: &http.Client{
			Timeout: ctTimeout,
			Transport: &http.Transport{
				MaxIdleConns:    2,
				IdleConnTimeout: 30 * time.Second,
			},
		},
	}
}

func (s *ctScanner) Protocol() string { return "ct" }

type crtShEntry struct {
	IssuerName string `json:"issuer_name"`
	CommonName string `json:"common_name"`
	NameValue  string `json:"name_value"`
	SerialNum  string `json:"serial_number"`
	NotBefore  string `json:"not_before"`
	NotAfter   string `json:"not_after"`
}

func (s *ctScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	entries, err := s.query(ctx, domain)
	if err != nil {
		return nil, err
	}

	entry := entries[0]
	if !coversDomain(entry, domain) {
		return nil, fmt.Errorf("ct: no certificates cover %s", domain)
	}

	var notBefore, notAfter time.Time
	parseTime := func(s string) time.Time {
		t, err := time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			t, err = time.Parse("2006-01-02", s)
			if err != nil {
				return time.Time{}
			}
		}
		return t
	}
	notBefore = parseTime(entry.NotBefore)
	notAfter = parseTime(entry.NotAfter)

	status := "valid"
	now := time.Now()
	if !notAfter.IsZero() && now.After(notAfter) {
		status = "expired"
	} else if !notBefore.IsZero() && now.Before(notBefore) {
		status = "not-yet-valid"
	}

	subject := entry.CommonName
	if entry.NameValue != "" {
		names := strings.SplitN(entry.NameValue, "\n", 2)
		subject = names[0]
	}

	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(entry.SerialNum+"|"+entry.IssuerName+"|"+subject)))

	return &Result{
		Subject:     subject,
		Issuer:      entry.IssuerName,
		Serial:      entry.SerialNum,
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		Fingerprint: fingerprint,
		Protocol:    "ct",
		Status:      status,
	}, nil
}

func (s *ctScanner) query(ctx context.Context, domain string) ([]crtShEntry, error) {
	entries, err := s.fetch(ctx, domain)
	if err == nil && len(entries) > 0 {
		return entries, nil
	}

	parts := strings.Split(domain, ".")
	if len(parts) > 2 {
		registered := strings.Join(parts[len(parts)-2:], ".")
		entries, err = s.fetch(ctx, "%."+registered)
		if err == nil && len(entries) > 0 {
			return entries, nil
		}
	}

	return nil, fmt.Errorf("ct: no certificates found for %s", domain)
}

func (s *ctScanner) fetch(ctx context.Context, q string) ([]crtShEntry, error) {
	u := fmt.Sprintf("https://crt.sh/?q=%s&output=json", url.QueryEscape(q))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("ct: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ct: query crt.sh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ct: crt.sh returned status %d", resp.StatusCode)
	}

	var entries []crtShEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("ct: decode response: %w", err)
	}

	return entries, nil
}

func coversDomain(entry crtShEntry, domain string) bool {
	names := entry.NameValue
	if names == "" {
		names = entry.CommonName
	}
	for _, name := range strings.Split(names, "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if strings.EqualFold(name, domain) {
			return true
		}
		if strings.HasPrefix(name, "*.") {
			suffix := name[1:] // remove *, keep ".domain.com"
			if strings.HasSuffix(domain, suffix) {
				return true
			}
		}
	}
	return false
}
