package discovery

import (
	"context"
	"fmt"
	"time"
)

type ldapScanner struct{ timeout time.Duration }

func NewLDAPScanner(timeout time.Duration) Scanner {
	return &ldapScanner{timeout: timeout}
}

func (s *ldapScanner) Protocol() string { return "ldap" }

func (s *ldapScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	return nil, fmt.Errorf("ldap scanner not yet implemented")
}
