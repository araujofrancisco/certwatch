package discovery

import (
	"context"
	"fmt"
	"time"
)

type smtpScanner struct{ timeout time.Duration }

func NewSMTPScanner(timeout time.Duration) Scanner {
	return &smtpScanner{timeout: timeout}
}

func (s *smtpScanner) Protocol() string { return "smtp" }

func (s *smtpScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	return nil, fmt.Errorf("smtp scanner not yet implemented")
}
