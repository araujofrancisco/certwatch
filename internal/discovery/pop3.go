package discovery

import (
	"context"
	"fmt"
	"time"
)

type pop3Scanner struct{ timeout time.Duration }

func NewPOP3Scanner(timeout time.Duration) Scanner {
	return &pop3Scanner{timeout: timeout}
}

func (s *pop3Scanner) Protocol() string { return "pop3" }

func (s *pop3Scanner) Scan(ctx context.Context, domain string) (*Result, error) {
	return nil, fmt.Errorf("pop3 scanner not yet implemented")
}
