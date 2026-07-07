package discovery

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"math/big"
	"net"
	"time"
)

type httpsScanner struct {
	timeout time.Duration
}

func NewHTTPSScanner(timeout time.Duration) Scanner {
	return &httpsScanner{timeout: timeout}
}

func (s *httpsScanner) Protocol() string {
	return "https"
}

func (s *httpsScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	addr := domain
	if _, _, err := net.SplitHostPort(domain); err != nil {
		addr = net.JoinHostPort(domain, "443")
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial %s: %w", domain, err)
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         domain,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("tls handshake %s: %w", domain, err)
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates presented by %s", domain)
	}

	leaf := certs[0]
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(leaf.Raw))

	status := "valid"
	now := time.Now()
	if now.Before(leaf.NotBefore) {
		status = "not-yet-valid"
	} else if now.After(leaf.NotAfter) {
		status = "expired"
	}

	return &Result{
		Subject:     leaf.Subject.String(),
		Issuer:      leaf.Issuer.String(),
		Serial:      serialToString(leaf.SerialNumber),
		NotBefore:   leaf.NotBefore,
		NotAfter:    leaf.NotAfter,
		Fingerprint: fingerprint,
		Protocol:    "https",
		Status:      status,
	}, nil
}

func serialToString(serial *big.Int) string {
	if serial == nil {
		return ""
	}
	return fmt.Sprintf("%x", serial)
}
