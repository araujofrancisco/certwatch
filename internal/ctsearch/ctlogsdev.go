package ctsearch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ctLogsDevProvider queries the ctlogs.dev certificate transparency search
// index (https://ctlogs.dev). ctlogs.dev continuously ingests every public CT
// log (Google, Cloudflare, DigiCert, Let's Encrypt, Sectigo and others) into a
// single searchable index. It has no public JSON API, so the provider parses
// the server-rendered search results and per-certificate detail pages.
//
// The provider is intentionally isolated here so its HTML parsing is a
// contained, testable dependency: the aggregator's failover absorbs any markup
// or availability regression, and the shared rate limiter keeps request rates
// gentle since ctlogs.dev does not publish quotas.
type ctLogsDevProvider struct {
	baseURL string
	client  *http.Client
	// maxCerts bounds how many certificate detail pages are fetched per search.
	maxCerts int
	// maxResponseBytes caps a single HTML page download.
	maxResponseBytes int64
	// gate, when non-nil, runs before each HTTP request so the search call and
	// its per-certificate detail fetches all share the aggregator's global
	// rate limiter instead of bypassing it.
	gate func(context.Context) error
}

// NewCTLogsDevProvider builds a ctlogs.dev-backed provider. baseURL defaults to
// https://ctlogs.dev when empty. client must be non-nil.
func NewCTLogsDevProvider(baseURL string, client *http.Client) Provider {
	if baseURL == "" {
		baseURL = "https://ctlogs.dev"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if client == nil {
		client = http.DefaultClient
	}
	return &ctLogsDevProvider{
		baseURL:          baseURL,
		client:           client,
		maxCerts:         100,
		maxResponseBytes: 4 << 20,
	}
}

func (p *ctLogsDevProvider) Name() string { return "ctlogs.dev" }

// SetRequestGate attaches a per-request throttle. It exists so the aggregator
// can make this provider's individual page fetches participate in the shared
// rate limit.
func (p *ctLogsDevProvider) SetRequestGate(g func(context.Context) error) {
	p.gate = g
}

func (p *ctLogsDevProvider) SearchByDomain(ctx context.Context, domain string, _ bool) ([]Entry, error) {
	u := fmt.Sprintf("%s/search?q=%s", p.baseURL, url.QueryEscape(domain))

	body, err := p.fetch(ctx, u)
	if err != nil {
		return nil, err
	}

	certIDs, err := parseSearchResults(body)
	if err != nil {
		return nil, fmt.Errorf("ctlogs.dev: parse search results: %w", err)
	}
	if len(certIDs) > p.maxCerts {
		certIDs = certIDs[:p.maxCerts]
	}

	entries := make([]Entry, 0, len(certIDs))
	for _, id := range certIDs {
		detail, err := p.fetch(ctx, fmt.Sprintf("%s/cert/%s", p.baseURL, url.PathEscape(id)))
		if err != nil {
			// A single detail page failing should not fail the whole search;
			// the aggregator can still use whatever we parsed.
			continue
		}
		e, ok := parseCertDetail(detail, time.Now())
		if !ok || e.SubjectName() == "" {
			continue
		}
		e.Source = p.Name()
		entries = append(entries, e)
	}
	return entries, nil
}

// fetch GETs u, following redirects (the search endpoint redirects to a
// hashed result URL), and returns the response body capped at maxResponseBytes.
func (p *ctLogsDevProvider) fetch(ctx context.Context, u string) ([]byte, error) {
	if p.gate != nil {
		if err := p.gate(ctx); err != nil {
			return nil, fmt.Errorf("ctlogs.dev: rate limit wait: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("ctlogs.dev: build request: %w", err)
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "CertWatch/ctsearch")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ctlogs.dev: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ctlogs.dev: unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, p.maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("ctlogs.dev: read response: %w", err)
	}
	return data, nil
}

// parseSearchResults extracts the certificate detail-page IDs from a ctlogs.dev
// domain search results page. The results table lists one row per certificate
// with a link to /cert/<id>; a page with zero results contains no such links.
func parseSearchResults(body []byte) ([]string, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	var ids []string
	seen := make(map[string]struct{})
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key != "href" {
					continue
				}
				const prefix = "/cert/"
				if strings.HasPrefix(attr.Val, prefix) && len(attr.Val) > len(prefix) {
					id := attr.Val[len(prefix):]
					if _, dup := seen[id]; !dup {
						seen[id] = struct{}{}
						ids = append(ids, id)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return ids, nil
}

// parseCertDetail maps a ctlogs.dev certificate detail page onto an Entry. It
// returns ok=false when the page does not contain a usable certificate.
// The page is a set of <dl> blocks whose <dt>/<dd> pairs carry the fields.
func parseCertDetail(body []byte, now time.Time) (Entry, bool) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return Entry{}, false
	}

	var commonName string
	var sanText string
	var issuer string
	var serial string
	var notBefore, notAfter string
	var dns []string

	var textOf func(*html.Node) string
	textOf = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		var sb strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			sb.WriteString(textOf(c))
		}
		return sb.String()
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "dl" {
			// Collect each dt/dd sibling pair within this block.
			var items []struct{ label, value string }
			var curLabel string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode {
					continue
				}
				switch c.Data {
				case "dt":
					curLabel = strings.TrimSpace(textOf(c))
				case "dd":
					items = append(items, struct{ label, value string }{
						label: strings.TrimSpace(curLabel),
						value: strings.TrimSpace(textOf(c)),
					})
				}
			}
			for _, it := range items {
				key := normalizeLabel(it.label)
				switch {
				case key == "common name":
					if commonName == "" {
						commonName = it.value
					}
				case key == "san dns":
					sanText = it.value
				case key == "distinguished name":
					// Subject DN appears before the issuer DN; the issuer is
					// the last DN on the page.
					issuer = it.value
			case key == "serial number":
				serial = NormalizeSerial(it.value)
				case key == "not before":
					notBefore = it.value
				case key == "not after":
					notAfter = it.value
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if commonName == "" && sanText == "" {
		return Entry{}, false
	}
	// The SAN list is a comma-separated set of DNS names (wildcards included)
	// on every detail page, so always split it; fall back to the common name
	// only when the page carries no usable SAN list.
	if sanText != "" {
		dns = splitDetailNames(sanText)
	}
	if len(dns) == 0 && commonName != "" {
		dns = []string{commonName}
	}

	e := Entry{
		CommonName: commonName,
		SANs:       dns,
		Issuer:     issuer,
		Serial:     serial,
		NotBefore:  parseCTDetailTime(notBefore),
		NotAfter:   parseCTDetailTime(notAfter),
	}
	e.populate(now)
	return e, true
}

// normalizeLabel maps the dt text to a stable lowercase key, collapsing the
// HTML entity for the middle dot used in "SAN · dns".
func normalizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "·", " ")
	s = strings.ReplaceAll(s, "\u00b7", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// splitDetailNames splits the SAN list (a comma-separated set of DNS names,
// possibly including wildcards) into trimmed, de-duplicated, lowercased names.
func splitDetailNames(s string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, part := range strings.Split(s, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// parseCTDetailTime parses the ctlogs.dev detail page date layout
// ("2026-07-29 22:10 UTC"). It returns the zero time when parsing fails.
func parseCTDetailTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04 MST", "2006-01-02 15:04:05 MST", "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}
