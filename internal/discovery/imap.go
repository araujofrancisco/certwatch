package discovery

import (
	"context"
	"fmt"
	"time"
)

type imapScanner struct{ timeout time.Duration }

func NewIMAPScanner(timeout time.Duration) Scanner {
	return &imapScanner{timeout: timeout}
}

func (s *imapScanner) Protocol() string { return "imap" }

func (s *imapScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	return nil, fmt.Errorf("imap scanner not yet implemented")
}
