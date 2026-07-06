package discovery

import (
	"context"
	"fmt"
	"time"
)

type tlsScanner struct{ timeout time.Duration }

func NewTLSScanner(timeout time.Duration) Scanner {
	return &tlsScanner{timeout: timeout}
}

func (s *tlsScanner) Protocol() string { return "tls" }

func (s *tlsScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	return nil, fmt.Errorf("generic tls scanner not yet implemented")
}
