package ctsearch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClientSearchByDomainDedupesAcrossProviders(t *testing.T) {
	dupe := baseEntry("S1") // same Key from both providers
	p1 := &stubProvider{name: "a", entries: []Entry{dupe, baseEntry("S2")}}
	p2 := &stubProvider{name: "b", entries: []Entry{dupe}}

	res, err := newTestClient(p1, p2).SearchByDomain(context.Background(), "example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 deduped results, got %d", len(res.Results))
	}
	if len(res.ProvidersQueried) != 2 {
		t.Errorf("expected 2 providers queried, got %d", len(res.ProvidersQueried))
	}
}

func TestClientSearchByDomainDedupesIssuerDNVariants(t *testing.T) {
	now := time.Now()
	issuerCNFirst := "CN=RapidSSL TLS RSA CA G1,OU=www.digicert.com,O=DigiCert Inc,C=US"
	issuerCFirst := "C=US, O=DigiCert Inc, OU=www.digicert.com, CN=RapidSSL TLS RSA CA G1"

	mk := func(issuer string) Entry {
		e := Entry{
			CommonName: "example.com",
			SANs:       []string{"example.com"},
			Issuer:     issuer,
			Serial:     "030150c1d6a9ce829bac11d55e0f097d",
			NotBefore:  now.Add(-24 * time.Hour),
			NotAfter:   now.Add(30 * 24 * time.Hour),
		}
		e.populate(now)
		return e
	}

	// Same issuance reported by both providers with different issuer DN
	// attribute orders. Normalization must make their dedup keys equal.
	p1 := &stubProvider{name: "ctlogs.dev", entries: []Entry{mk(issuerCNFirst)}}
	p2 := &stubProvider{name: "CertSpotter", entries: []Entry{mk(issuerCFirst)}}

	res, err := newTestClient(p1, p2).SearchByDomain(context.Background(), "example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 deduped result, got %d", len(res.Results))
	}
}

func TestClientFailoverWhenFirstProviderFails(t *testing.T) {
	p1 := &stubProvider{name: "a", domainErr: errors.New("crt.sh down")}
	p2 := &stubProvider{name: "b", entries: []Entry{baseEntry("S9")}}

	res, err := newTestClient(p1, p2).SearchByDomain(context.Background(), "sub.example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 result from failover provider, got %d", len(res.Results))
	}
	if res.Results[0].Source != "b" {
		t.Errorf("expected failover source b, got %s", res.Results[0].Source)
	}
}

func TestClientKeepsResultsWhenLaterProviderHitsDeadline(t *testing.T) {
	good := &stubProvider{name: "a", entries: []Entry{baseEntry("S1"), baseEntry("S2")}}
	slow := &slowProvider{name: "slow"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	res, err := newTestClient(good, slow).SearchByDomain(ctx, "example.com", true)
	if err != nil {
		t.Fatalf("expected partial results preserved, got error: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 results from the fast provider, got %d", len(res.Results))
	}
	if res.Results[0].Source != "a" {
		t.Errorf("expected results from fast provider, got source %s", res.Results[0].Source)
	}
}

func TestClientAllProvidersFailReturnsError(t *testing.T) {
	p1 := &stubProvider{name: "a", domainErr: errors.New("down1")}
	p2 := &stubProvider{name: "b", domainErr: errors.New("down2")}

	_, err := newTestClient(p1, p2).SearchByDomain(context.Background(), "example.com", false)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestClientFiltersNonCoveringWhenNotIncludeSubdomains(t *testing.T) {
	cover := baseEntry("S1")
	unrelated := Entry{Serial: "S2", SANs: []string{"other.com"}}
	p := &stubProvider{name: "a", entries: []Entry{cover, unrelated}}

	res, err := newTestClient(p).SearchByDomain(context.Background(), "sub.example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected only covering entry, got %d", len(res.Results))
	}

	res2, err := newTestClient(p).SearchByDomain(context.Background(), "sub.example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Results) != 2 {
		t.Fatalf("expected both when include_subdomains, got %d", len(res2.Results))
	}
}

func TestClientRetriesTransientErrorOnce(t *testing.T) {
	p := &stubProvider{name: "a", entries: []Entry{baseEntry("S1")}}
	flapper := &failOnceProvider{p: p}

	res, err := newTestClient(flapper).SearchByDomain(context.Background(), "sub.example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected result after retry, got %d", len(res.Results))
	}
	if flapper.attempts != 2 {
		t.Errorf("expected 2 attempts (1 fail + 1 retry), got %d", flapper.attempts)
	}
}

func TestNewDefaultClientRespectsProviderOrder(t *testing.T) {
	c := NewDefaultClient(Config{Timeout: time.Second, MaxQPS: 1000, Providers: []string{"certspotter", "ctlogsdev"}})
	if len(c.providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(c.providers))
	}
	if c.providers[0].Name() != "CertSpotter" {
		t.Errorf("expected CertSpotter first, got %s", c.providers[0].Name())
	}
	if c.providers[1].Name() != "ctlogs.dev" {
		t.Errorf("expected ctlogs.dev second, got %s", c.providers[1].Name())
	}
}

func TestNewDefaultClientSkipsUnknownProviders(t *testing.T) {
	c := NewDefaultClient(Config{Providers: []string{"nope", "certspotter"}})
	if len(c.providers) != 1 || c.providers[0].Name() != "CertSpotter" {
		t.Fatalf("expected only the known provider, got %d providers", len(c.providers))
	}
}

func TestNewDefaultClientDefaults(t *testing.T) {
	c := NewDefaultClient(Config{})
	names := make([]string, len(c.providers))
	for i, p := range c.providers {
		names[i] = p.Name()
	}
	want := []string{"CertSpotter", "ctlogs.dev"}
	for i, w := range want {
		if i >= len(names) || names[i] != w {
			t.Fatalf("expected default order %v, got %v", want, names)
		}
	}
}

func TestClientSkipsProviderInCooldown(t *testing.T) {
	always := &stubProvider{name: "a", domainErr: errors.New("down")}
	c := newTestClient(always)
	c.breaker.Cooldown = time.Hour
	c.breaker.MaxFailures = 2

	// Two failures trip the cooldown.
	for i := 0; i < 2; i++ {
		if _, err := c.SearchByDomain(context.Background(), "example.com", false); err == nil {
			t.Fatalf("call %d: expected error while provider is up", i+1)
		}
	}

	// Third call: provider is skipped, so no error and no results.
	res, err := c.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatalf("expected no error when provider is skipped, got %v", err)
	}
	if len(res.ProvidersQueried) != 0 || len(res.Results) != 0 {
		t.Errorf("expected provider skipped entirely, got %+v", res)
	}
}

func TestBreakerResetsOnSuccess(t *testing.T) {
	p := &stubProvider{name: "a", entries: []Entry{baseEntry("S1")}, domainErr: errors.New("down")}
	c := newTestClient(p)
	c.breaker.Cooldown = time.Hour
	c.breaker.MaxFailures = 2

	// One failure stays below the threshold.
	if _, err := c.SearchByDomain(context.Background(), "sub.example.com", false); err == nil {
		t.Fatal("expected first failure")
	}

	// A success resets the failure count.
	p.domainErr = nil
	if _, err := c.SearchByDomain(context.Background(), "sub.example.com", false); err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}

	// Two failures again trip the cooldown.
	p.domainErr = errors.New("down")
	for i := 0; i < 2; i++ {
		if _, err := c.SearchByDomain(context.Background(), "example.com", false); err == nil {
			t.Fatalf("call %d: expected error", i+1)
		}
	}
	res, err := c.SearchByDomain(context.Background(), "example.com", false)
	if err != nil {
		t.Fatalf("expected skip after re-trip, got %v", err)
	}
	if len(res.ProvidersQueried) != 0 {
		t.Errorf("expected provider skipped, got %+v", res)
	}
}

func TestSearchOrderNewestFirst(t *testing.T) {
	older := baseEntry("S1")
	older.NotBefore = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := baseEntry("S2")
	newer.NotBefore = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &stubProvider{name: "a", entries: []Entry{older, newer}}

	res, err := newTestClient(p).SearchByDomain(context.Background(), "example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res.Results))
	}
	if res.Results[0].Serial != "S2" {
		t.Errorf("expected newest first, got %s", res.Results[0].Serial)
	}
	if res.Results[0].Source != "a" {
		t.Errorf("expected newest entry source a, got %s", res.Results[0].Source)
	}
}
