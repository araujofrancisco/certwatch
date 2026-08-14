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

// certSpotterProvider queries the SSLMate Certificate Search API
// (api.certspotter.com). SSLMate ingests 40+ public CT logs and indexes them
// by domain name.
type certSpotterProvider struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewCertSpotterProvider builds a CertSpotter-backed provider. baseURL
// defaults to https://api.certspotter.com. When token is empty, anonymous
// (rate limited) queries are used; supplying an API token increases the quota.
func NewCertSpotterProvider(baseURL, token string, client *http.Client) Provider {
	if baseURL == "" {
		baseURL = "https://api.certspotter.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if client == nil {
		client = http.DefaultClient
	}
	return &certSpotterProvider{baseURL: baseURL, token: token, client: client}
}

func (p *certSpotterProvider) Name() string { return "CertSpotter" }

func (p *certSpotterProvider) SearchByDomain(ctx context.Context, domain string, includeSubdomains bool) ([]Entry, error) {
	u := fmt.Sprintf("%s/v1/issuances?domain=%s&include_subdomains=%t&expand=issuer",
		p.baseURL, url.QueryEscape(domain), includeSubdomains)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("certspotter: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CertWatch/ctsearch")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("certspotter: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("certspotter: rate limited (429)")
		}
		return nil, fmt.Errorf("certspotter: unexpected status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, 10<<20)
	raw, err := decodeCertSpotter(limited)
	if err != nil {
		return nil, fmt.Errorf("certspotter: decode response: %w", err)
	}

	entries := make([]Entry, 0, len(raw))
	for _, r := range raw {
		entries = append(entries, r.toEntry(time.Now()))
	}
	return entries, nil
}

// decodeCertSpotter handles the two response shapes CertSpotter has used: an
// object wrapping an "issuances" array, and (historically) a bare top-level
// array.
func decodeCertSpotter(r io.Reader) ([]certSpotterRow, error) {
	data, err := io.ReadAll(io.LimitReader(r, 10<<20))
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Issuances json.RawMessage `json:"issuances"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Issuances) > 0 {
		var rows []certSpotterRow
		if err := json.Unmarshal(envelope.Issuances, &rows); err == nil {
			return rows, nil
		}
	}

	var rows []certSpotterRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// certSpotterRow mirrors the CertSpotter issuances API schema.
type certSpotterRow struct {
	SerialNumber string `json:"serial_number"`
	CommonName   string `json:"common_name"`
	NameValue    string `json:"name_value"`
	NotBefore    string `json:"not_before"`
	NotAfter     string `json:"not_after"`
	Issuer       struct {
		Name string `json:"name"`
	} `json:"issuer"`
}

func (r certSpotterRow) toEntry(now time.Time) Entry {
	names := splitNames(r.NameValue)
	if len(names) == 0 {
		names = []string{strings.TrimSpace(r.CommonName)}
	}
	e := Entry{
		CommonName: strings.TrimSpace(r.CommonName),
		SANs:       names,
		Issuer:     r.Issuer.Name,
		Serial:     NormalizeSerial(r.SerialNumber),
		NotBefore:  parseCTTime(r.NotBefore),
		NotAfter:   parseCTTime(r.NotAfter),
		Source:     "CertSpotter",
	}
	e.populate(now)
	return e
}
