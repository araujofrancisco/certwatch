package discovery

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/araujofrancisco/certwatch/internal/ctsearch"
)

// ctScanner wraps the ctsearch aggregator client behind the discovery Scanner
// interface. It implements both Scanner (best single result) and MultiScanner
// (all matching CT entries).
type ctScanner struct {
	client  *ctsearch.Client
	timeout time.Duration
}

// NewCTScanner builds a CT scanner with a default HTTP client and timeout.
func NewCTScanner(timeout time.Duration) Scanner {
	return NewCTScannerWithClient(nil, timeout)
}

// NewCTScannerWithClient builds a CT scanner using the provided HTTP client
// (or a default one when nil). The timeout bounds each provider request.
func NewCTScannerWithClient(client *http.Client, timeout time.Duration) Scanner {
	cfg := ctsearch.DefaultConfig()
	if timeout > 0 {
		cfg.Timeout = timeout
	}
	if client == nil {
		client = &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        4,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     30 * time.Second,
			},
		}
	}
	return &ctScanner{
		client:  ctsearch.NewClient(cfg, buildProviders(client)...),
		timeout: cfg.Timeout,
	}
}

func buildProviders(client *http.Client) []ctsearch.Provider {
	return []ctsearch.Provider{
		ctsearch.NewCertSpotterProvider("", "", client),
		ctsearch.NewCTLogsDevProvider("", client),
	}
}

func (s *ctScanner) Protocol() string { return "ct" }

func (s *ctScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	results, err := s.ScanAll(ctx, domain)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("ct: no certificates cover %s", domain)
	}
	return results[0], nil
}

func (s *ctScanner) ScanAll(ctx context.Context, domain string) ([]*Result, error) {
	res, err := s.client.SearchByDomain(ctx, domain, false)
	if err != nil {
		return nil, err
	}
	if len(res.Results) == 0 {
		return nil, fmt.Errorf("ct: no certificates cover %s", domain)
	}

	results := make([]*Result, 0, len(res.Results))
	for _, e := range res.Results {
		results = append(results, &Result{
			Subject:     e.Subject,
			Issuer:      e.Issuer,
			Serial:      e.Serial,
			NotBefore:   e.NotBefore,
			NotAfter:    e.NotAfter,
			Fingerprint: e.Fingerprint(),
			Protocol:    "ct",
			Status:      e.Status,
			SANs:        e.Names(),
		})
	}
	return results, nil
}