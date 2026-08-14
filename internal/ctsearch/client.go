package ctsearch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Config controls the behavior of the aggregator Client.
type Config struct {
	// Timeout bounds each individual provider request.
	Timeout time.Duration
	// MaxQPS caps the global request rate across all providers (defensive: the
	// free tiers of the included providers have tight quotas and will ban
	// aggressive clients). Values <= 0 fall back to 1 query per second.
	MaxQPS float64
	// CertSpotterToken is the optional SSLMate API token. When empty, anonymous
	// (more heavily rate limited) queries are used.
	CertSpotterToken string
	// Providers lists the provider names to query, in priority order. Names
	// must be members of ProviderNames. When empty, the default order
	// (CertSpotter, ctlogs.dev) is used.
	Providers []string
}

// DefaultConfig returns conservative defaults suitable for anonymous use of the
// free providers.
func DefaultConfig() Config {
	return Config{Timeout: 10 * time.Second, MaxQPS: 1, Providers: DefaultProviders()}
}

// DefaultProviders returns the default provider priority order. CertSpotter is
// queried first because its JSON API returns the full certificate set for a
// domain in a single request; ctlogs.dev follows as failover when CertSpotter
// is unreachable or throttled. The operator can override the set and order via
// config.
func DefaultProviders() []string {
	return []string{"certspotter", "ctlogsdev"}
}

// ProviderNames is the set of provider names the aggregator can construct.
var ProviderNames = map[string]bool{"ctlogsdev": true, "certspotter": true}

// QueryResult is the normalized outcome of a search request. It is JSON-ready
// and used directly by the API layer.
type QueryResult struct {
	Type              string   `json:"type"`
	Query             string   `json:"query"`
	IncludeSubdomains bool     `json:"include_subdomains"`
	ProvidersQueried  []string `json:"providers_queried"`
	Results           []Entry  `json:"results"`
}

// Client fans out a lookup across the configured providers, applying
// failover, deduplication and rate limiting. It is safe for concurrent use.
type Client struct {
	providers []Provider
	// limiter throttles aggregate requests across all providers.
	limiter *intervalLimiter
	// breaker records consecutive failures per provider so a persistently
	// dead provider is skipped instead of stalling every lookup.
	breaker *failureBreaker
}

// NewClient builds a Client over the given providers. See Config for defaults.
func NewClient(cfg Config, providers ...Provider) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultConfig().Timeout
	}
	qps := cfg.MaxQPS
	if qps <= 0 {
		qps = DefaultConfig().MaxQPS
	}
	return &Client{
		providers: providers,
		limiter:   newIntervalLimiter(time.Duration(float64(time.Second) / qps)),
		breaker:   newFailureBreaker(),
	}
}

// NewDefaultClient wires the built-in providers behind a single shared Client.
// The provider set and priority order come from cfg.Providers (falling back to
// DefaultProviders). This is the recommended production entry point: it gives
// failover so the loss of one provider does not break CT discovery, and a
// single rate limiter shared by all of them.
func NewDefaultClient(cfg Config) *Client {
	names := cfg.Providers
	if len(names) == 0 {
		names = DefaultProviders()
	}
	httpClient := func() *http.Client { return httpClientBuilder(cfg.Timeout) }
	factories := map[string]func() Provider{
		"ctlogsdev":   func() Provider { return NewCTLogsDevProvider("", httpClient()) },
		"certspotter": func() Provider { return NewCertSpotterProvider("", cfg.CertSpotterToken, httpClient()) },
	}
	providers := make([]Provider, 0, len(names))
	for _, name := range names {
		if factory, ok := factories[name]; ok {
			providers = append(providers, factory())
		}
	}
	return NewClient(cfg, providers...)
}

// SearchByDomain returns certificates related to domain. When includeSubdomains
// is false, only entries that actually cover the domain (exact match or a
// relevant wildcard) are returned; when true, all related entries, including
// subdomains, are returned.
func (c *Client) SearchByDomain(ctx context.Context, domain string, includeSubdomains bool) (QueryResult, error) {
	res := QueryResult{Type: "domain", Query: domain, IncludeSubdomains: includeSubdomains}
	err := c.collect(ctx, func(p Provider) ([]Entry, error) {
		entries, err := p.SearchByDomain(ctx, domain, includeSubdomains)
		if err != nil {
			return nil, err
		}
		if !includeSubdomains {
			entries = filterCovers(entries, domain)
		}
		return entries, nil
	}, &res)
	if err != nil {
		return res, err
	}
	sortEntries(res.Results)
	return res, nil
}

// collect runs fn over each provider, merging results and applying failover.
// Providers are queried concurrently so a slow or unreachable provider does
// not consume the whole budget and starve the rest; the shared rate limiter
// still serializes the actual network requests. Providers that have been
// failing repeatedly are skipped for a cooldown window so one dead source does
// not stall the whole lookup. Providers that fail transiently are retried once.
// If every provider fails, the last non-context error is returned. Entries are
// de-duplicated on Entry.Key().
func (c *Client) collect(ctx context.Context, fn func(Provider) ([]Entry, error), res *QueryResult) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		lastErr error
		order   []string
	)
	seen := make(map[string]struct{})

	for _, p := range c.providers {
		if c.breaker.skip(p.Name()) {
			continue
		}
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			entries, err := c.call(ctx, fn, p)
			mu.Lock()
			defer mu.Unlock()
			order = append(order, p.Name())
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					// The overall budget ran out. Keep whatever earlier
					// providers already returned instead of throwing it away —
					// a slow provider must not erase other providers' results.
					if len(seen) == 0 {
						lastErr = err
					}
					return
				}
				c.breaker.record(p.Name(), err)
				lastErr = err
				return
			}
			c.breaker.record(p.Name(), nil)
			for _, e := range entries {
				if _, dup := seen[e.Key()]; dup {
					continue
				}
				seen[e.Key()] = struct{}{}
				res.Results = append(res.Results, e)
			}
		}(p)
	}
	wg.Wait()
	res.ProvidersQueried = order

	if len(res.Results) == 0 && lastErr != nil {
		return fmt.Errorf("all CT search providers failed: %w", lastErr)
	}
	return nil
}

// call applies the shared rate limit and a single retry for transient errors.
func (c *Client) call(ctx context.Context, fn func(Provider) ([]Entry, error), p Provider) ([]Entry, error) {
	attempt := func() ([]Entry, error) {
		if err := c.limiter.wait(ctx); err != nil {
			return nil, err
		}
		// Each provider's HTTP client carries Config.Timeout, which bounds the
		// actual network request, while the caller-owned ctx covers cancellation
		// and any outer deadline.
		return fn(p)
	}

	entries, err := attempt()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// One bounded retry on transient provider failures.
		entries, err = attempt()
	}
	return entries, err
}

func filterCovers(entries []Entry, domain string) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Covers(domain) {
			out = append(out, e)
		}
	}
	return out
}

// sortEntries orders results by issuance date, newest first.
func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].NotBefore.After(entries[j].NotBefore)
	})
}

// intervalLimiter is a minimal token gate that guarantees a minimum delay
// between calls. It is intentionally simple (no burst) because the included
// providers reward gentle request patterns.
type intervalLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func newIntervalLimiter(interval time.Duration) *intervalLimiter {
	if interval <= 0 {
		interval = time.Second
	}
	return &intervalLimiter{interval: interval}
}

func (l *intervalLimiter) wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := time.Now()
		if !now.Before(l.next) {
			l.next = now.Add(l.interval)
			l.mu.Unlock()
			return nil
		}
		d := l.next.Sub(now)
		l.mu.Unlock()

		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// failureBreaker tracks consecutive provider failures and opens a short
// cooldown so a persistently unreachable provider is skipped rather than
// stalling every lookup with repeated timeouts. It is safe for concurrent use.
type failureBreaker struct {
	mu       sync.Mutex
	next     map[string]time.Time
	failures map[string]int

	// Cooldown is the window a provider is skipped after tripping.
	Cooldown time.Duration
	// MaxFailures is the consecutive-failure threshold that opens the cooldown.
	MaxFailures int
}

func newFailureBreaker() *failureBreaker {
	return &failureBreaker{
		next:        make(map[string]time.Time),
		failures:    make(map[string]int),
		Cooldown:    5 * time.Minute,
		MaxFailures: 2,
	}
}

// skip reports whether the provider is currently in its cooldown window.
func (b *failureBreaker) skip(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Now().Before(b.next[name])
}

// record updates the failure count for name. err == nil resets the count.
func (b *failureBreaker) record(name string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		delete(b.failures, name)
		return
	}
	b.failures[name]++
	if b.failures[name] < b.MaxFailures {
		return
	}
	if !time.Now().Before(b.next[name]) {
		slog.Warn("ct search provider temporarily disabled after repeated failures",
			"provider", name, "failures", b.failures[name], "cooldown", b.Cooldown)
	}
	b.next[name] = time.Now().Add(b.Cooldown)
}
