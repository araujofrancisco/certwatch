package discovery

import (
	"context"
	"sync"
	"time"
)

type Result struct {
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	Serial      string    `json:"serial"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	Fingerprint string    `json:"fingerprint"`
	Protocol    string    `json:"protocol"`
	Status      string    `json:"status"`
	SANs        []string  `json:"sans"`
}

type Scanner interface {
	Protocol() string
	Scan(ctx context.Context, domain string) (*Result, error)
}

// MultiScanner is an optional interface a Scanner can implement to return all
// certificates discovered for a domain (e.g. every matching CT log entry),
// rather than just the most relevant one.
type MultiScanner interface {
	Protocol() string
	ScanAll(ctx context.Context, domain string) ([]*Result, error)
}

type Registry struct {
	mu       sync.RWMutex
	scanners map[string]Scanner
}

func NewRegistry() *Registry {
	return &Registry{scanners: make(map[string]Scanner)}
}

func (r *Registry) Register(s Scanner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanners[s.Protocol()] = s
}

func (r *Registry) ForProtocol(protocol string) Scanner {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.scanners[protocol]
}

func (r *Registry) All() []Scanner {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Scanner, 0, len(r.scanners))
	for _, s := range r.scanners {
		out = append(out, s)
	}
	return out
}
