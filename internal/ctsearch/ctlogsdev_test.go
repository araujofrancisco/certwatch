package ctsearch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The fixtures mirror the server-rendered HTML that ctlogs.dev actually serves:
// a search results page with one row per certificate and per-certificate detail
// pages composed of <dl> blocks.

const ctLogsResultsPage = `<!doctype html>
<html lang="en"><head><title>example.com — ctlogs.dev</title></head>
<body>
<nav><span class="ctx">search</span><code>example.com</code></nav>
<main class="wrap">
  <p class="meta"><b>2</b> results &middot; <b>12</b> ms</p>
  <div class="tablewrap">
  <table>
    <thead><tr><th>Match</th><th>Valid until</th><th>Lifetime</th><th>Issuer</th><th>Algorithm</th><th>SANs</th></tr></thead>
    <tbody>
      <tr>
        <td><a href="/cert/000000b50624d0ab311558780b7d5213b9631831" target="_blank">example.com</a></td>
        <td>2026-10-27</td>
        <td title="from 2026-07-29">90 d</td>
        <td>SSL Corporation</td>
        <td>ECDSA P-256</td>
        <td>2</td>
      </tr>
      <tr>
        <td><a href="/cert/00000012be85c3a855ab63a7031bc71e12e85177" target="_blank">example.com</a></td>
        <td>2025-05-15</td>
        <td title="from 2024-05-15">90 d</td>
        <td>Sectigo Limited</td>
        <td>RSA 2048</td>
        <td>2</td>
      </tr>
    </tbody>
  </table>
  </div>
</main>
</body>
</html>`

const ctLogsEmptyPage = `<main class="wrap"><p class="meta"><b>0</b> results &middot; <b>3</b> ms</p></main>`

func ctLogsDetailPage(id string) string {
	issuer := "Cloudflare TLS Issuing ECC CA 3"
	serial := "0624d0ab311558780b7d5213b9631831"
	notBefore := "2026-07-29 22:10 UTC"
	notAfter := "2026-10-27 22:17 UTC"
	if id == "00000012be85c3a855ab63a7031bc71e12e85177" {
		issuer = "Sectigo Limited"
		serial = "1A:2B:3C:4D:5E"
		notBefore = "2024-05-15 00:00 UTC"
		notAfter = "2025-05-15 00:00 UTC"
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><title>example.com — ctlogs.dev</title></head>
<body>
<main class="wrap">
<dl class="kv">
<dt>Common name</dt><dd>example.com</dd>
<dt>Distinguished name</dt><dd>CN=example.com</dd>
<dt>SAN &middot; dns</dt><dd>example.com, *.example.com</dd>
</dl>
<dl class="kv">
<dt>Distinguished name</dt><dd>CN=%s,O=SSL Corporation,C=US</dd>
<dt>Serial number</dt><dd>%s</dd>
</dl>
<dl class="kv">
<dt>Not before</dt><dd>%s</dd>
<dt>Not after</dt><dd>%s</dd>
</dl>
</main>
</body>
</html>`, issuer, serial, notBefore, notAfter)
}

func newCTLogsTestServer(t *testing.T, resultsPage string, failDetail map[string]bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, resultsPage)
		case strings.HasPrefix(r.URL.Path, "/cert/"):
			id := strings.TrimPrefix(r.URL.Path, "/cert/")
			if failDetail[id] {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, ctLogsDetailPage(id))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCTLogsDevParsesResultsAndDetails(t *testing.T) {
	srv := newCTLogsTestServer(t, ctLogsResultsPage, nil)
	p := NewCTLogsDevProvider(srv.URL, srv.Client())

	entries, err := p.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	e := entries[0]
	if e.Source != "ctlogs.dev" {
		t.Errorf("expected source ctlogs.dev, got %s", e.Source)
	}
	if e.CommonName != "example.com" {
		t.Errorf("expected common name example.com, got %s", e.CommonName)
	}
	if len(e.SANs) != 2 || e.SANs[0] != "example.com" || e.SANs[1] != "*.example.com" {
		t.Errorf("unexpected SANs: %v", e.SANs)
	}
	if !strings.Contains(e.Issuer, "Cloudflare TLS Issuing ECC CA 3") {
		t.Errorf("expected Cloudflare issuer, got %s", e.Issuer)
	}
	if e.Serial != "0624d0ab311558780b7d5213b9631831" {
		t.Errorf("unexpected serial %s", e.Serial)
	}
	if !e.NotBefore.Equal(time.Date(2026, 7, 29, 22, 10, 0, 0, time.UTC)) {
		t.Errorf("unexpected not_before %v", e.NotBefore)
	}
	if !e.NotAfter.Equal(time.Date(2026, 10, 27, 22, 17, 0, 0, time.UTC)) {
		t.Errorf("unexpected not_after %v", e.NotAfter)
	}
	if !e.Covers("example.com") || !e.Covers("api.example.com") {
		t.Error("expected entries to cover the domain and wildcard subdomains")
	}
}

func TestCTLogsDevNonWildcardMultiSAN(t *testing.T) {
	const detail = `<!doctype html><html><body>
<main class="wrap">
<dl class="kv">
<dt>Common name</dt><dd>example.com</dd>
<dt>Distinguished name</dt><dd>CN=example.com</dd>
<dt>SAN &middot; dns</dt><dd>example.com, www.example.com, mail.example.com</dd>
</dl>
<dl class="kv">
<dt>Distinguished name</dt><dd>CN=Let's Encrypt CA,O=Let's Encrypt,C=US</dd>
<dt>Serial number</dt><dd>04AABBCC</dd>
</dl>
<dl class="kv">
<dt>Not before</dt><dd>2026-07-01 00:00 UTC</dd>
<dt>Not after</dt><dd>2026-09-29 00:00 UTC</dd>
</dl>
</main>
</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<main class="wrap"><table><tbody><tr><td><a href="/cert/1234abcd" target="_blank">example.com</a></td></tr></tbody></table></main>`)
		case r.URL.Path == "/cert/1234abcd":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, detail)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := NewCTLogsDevProvider(srv.URL, srv.Client())
	entries, err := p.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	want := []string{"example.com", "www.example.com", "mail.example.com"}
	if len(e.SANs) != len(want) {
		t.Fatalf("expected SANs %v, got %v", want, e.SANs)
	}
	for i, w := range want {
		if e.SANs[i] != w {
			t.Errorf("SAN[%d] = %q, want %q", i, e.SANs[i], w)
		}
	}
	if !e.Covers("www.example.com") {
		t.Error("expected the entry to cover www.example.com")
	}
}

func TestCTLogsDevNormalizesSerial(t *testing.T) {
	srv := newCTLogsTestServer(t, ctLogsResultsPage, nil)
	p := NewCTLogsDevProvider(srv.URL, srv.Client())

	entries, err := p.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	// The second fixture certificate uses colon-separated hex; the provider
	// normalizes it to bare lowercase hex for cross-provider dedup.
	if entries[1].Serial != "1a2b3c4d5e" {
		t.Errorf("expected normalized serial, got %s", entries[1].Serial)
	}
}

func TestCTLogsDevEmptyResults(t *testing.T) {
	srv := newCTLogsTestServer(t, ctLogsEmptyPage, nil)
	p := NewCTLogsDevProvider(srv.URL, srv.Client())

	entries, err := p.SearchByDomain(context.Background(), "empty.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(entries))
	}
}

func TestCTLogsDevHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	p := NewCTLogsDevProvider(srv.URL, srv.Client())
	if _, err := p.SearchByDomain(context.Background(), "example.com", false); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestCTLogsDevSkipsFailedDetailPages(t *testing.T) {
	srv := newCTLogsTestServer(t, ctLogsResultsPage, map[string]bool{
		"00000012be85c3a855ab63a7031bc71e12e85177": true,
	})
	p := NewCTLogsDevProvider(srv.URL, srv.Client())

	entries, err := p.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (failed detail skipped), got %d", len(entries))
	}
	if entries[0].Serial != "0624d0ab311558780b7d5213b9631831" {
		t.Errorf("expected the healthy detail page entry, got %s", entries[0].Serial)
	}
}

func TestCTLogsDevRespectsMaxCerts(t *testing.T) {
	srv := newCTLogsTestServer(t, ctLogsResultsPage, nil)
	p := NewCTLogsDevProvider(srv.URL, srv.Client()).(*ctLogsDevProvider)
	p.maxCerts = 1

	entries, err := p.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry when maxCerts=1, got %d", len(entries))
	}
}
