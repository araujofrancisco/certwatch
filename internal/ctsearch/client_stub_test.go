package ctsearch

import (
	"context"
	"errors"
	"time"
)

// stubProvider is a configurable Provider for exercising the aggregator.
type stubProvider struct {
	name      string
	entries   []Entry
	domainErr error
	calls     int
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) SearchByDomain(ctx context.Context, domain string, includeSubdomains bool) ([]Entry, error) {
	s.calls++
	if s.domainErr != nil {
		return nil, s.domainErr
	}
	entries := tagSources(s.entries, s.name)
	if includeSubdomains {
		return entries, nil
	}
	return filterCovers(entries, domain), nil
}

// tagSources returns a copy of entries with Source overridden to the given
// provider name, mirroring how real providers label their results.
func tagSources(entries []Entry, source string) []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	for i := range out {
		out[i].Source = source
	}
	return out
}

// failOnceProvider delegates to p but fails its first domain call, so the
// aggregator's single-retry behavior can be asserted.
type failOnceProvider struct {
	p        Provider
	failed   bool
	attempts int
}

func (f *failOnceProvider) Name() string { return f.p.Name() }

func (f *failOnceProvider) SearchByDomain(ctx context.Context, domain string, include bool) ([]Entry, error) {
	f.attempts++
	if !f.failed {
		f.failed = true
		return nil, errors.New("transient")
	}
	return f.p.SearchByDomain(ctx, domain, include)
}

// slowProvider blocks until ctx is done, simulating a provider that hangs and
// hits the caller's deadline.
type slowProvider struct{ name string }

func (s *slowProvider) Name() string { return s.name }

func (s *slowProvider) SearchByDomain(ctx context.Context, domain string, include bool) ([]Entry, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func newTestClient(providers ...Provider) *Client {
	return NewClient(Config{Timeout: time.Second, MaxQPS: 1000}, providers...)
}

func baseEntry(serial string) Entry {
	return Entry{
		Serial:    serial,
		Issuer:    "CA",
		SANs:      []string{"sub.example.com"},
		NotBefore: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}
