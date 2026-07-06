package discovery

import (
	"context"
	"fmt"
	"time"
)

type ftpScanner struct{ timeout time.Duration }

func NewFTPScanner(timeout time.Duration) Scanner {
	return &ftpScanner{timeout: timeout}
}

func (s *ftpScanner) Protocol() string { return "ftp" }

func (s *ftpScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	return nil, fmt.Errorf("ftp scanner not yet implemented")
}
