package ctsearch

import (
	"context"
)

// Provider is one Certificate Transparency search backend. Implementations
// must be safe for concurrent use.
type Provider interface {
	// Name returns a stable identifier for the provider. It is used as the
	// Source on every Entry and reported back to callers / the UI.
	Name() string

	// SearchByDomain returns certificates related to the given domain. When
	// includeSubdomains is false the provider may still return wildcard and
	// subdomain matches; the aggregator is responsible for the final exactness
	// contract described on Client.SearchByDomain.
	SearchByDomain(ctx context.Context, domain string, includeSubdomains bool) ([]Entry, error)
}
