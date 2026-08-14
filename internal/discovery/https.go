package discovery

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/araujofrancisco/certwatch/internal/ctsearch"
)

type httpsScanner struct {
	timeout time.Duration
	roots   *x509.CertPool
}

func NewHTTPSScanner(timeout time.Duration) Scanner {
	return &httpsScanner{timeout: timeout}
}

// NewHTTPSScannerWithRoots builds an HTTPS scanner that verifies peer
// certificates against the given root pool. When roots is nil the system root
// pool is used. Tests and private-PKI setups can supply a custom pool.
func NewHTTPSScannerWithRoots(timeout time.Duration, roots *x509.CertPool) Scanner {
	return &httpsScanner{timeout: timeout, roots: roots}
}

func (s *httpsScanner) Protocol() string {
	return "https"
}

func (s *httpsScanner) Scan(ctx context.Context, domain string) (*Result, error) {
	addr := domain
	serverName := domain
	if host, _, err := net.SplitHostPort(domain); err == nil {
		serverName = host
	} else {
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
		ServerName:         serverName,
		RootCAs:            s.roots,
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
		Issuer:      ctsearch.NormalizeDN(leaf.Issuer.String()),
		Serial:      ctsearch.NormalizeSerial(serialToString(leaf.SerialNumber)),
		NotBefore:   leaf.NotBefore,
		NotAfter:    leaf.NotAfter,
		Fingerprint: fingerprint,
		Protocol:    "https",
		Status:      status,
		SANs:        leaf.DNSNames,
	}, nil
}

func serialToString(serial *big.Int) string {
	if serial == nil {
		return ""
	}
	return fmt.Sprintf("%x", serial)
}
