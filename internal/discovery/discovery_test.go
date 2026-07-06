package discovery

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	s := NewHTTPSScanner(5 * time.Second)
	r.Register(s)

	if r.ForProtocol("https") == nil {
		t.Error("expected https scanner")
	}
	if r.ForProtocol("nonexistent") != nil {
		t.Error("expected nil for nonexistent protocol")
	}
	if len(r.All()) != 1 {
		t.Errorf("expected 1 scanner, got %d", len(r.All()))
	}
}

func TestHTTPScannerInterface(t *testing.T) {
	s := NewHTTPSScanner(5 * time.Second)
	if s.Protocol() != "https" {
		t.Errorf("expected https, got %s", s.Protocol())
	}
}

func TestHTTPSScannerWithTestServer(t *testing.T) {
	certPEM, keyPEM := generateTestCertPair(t)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		tlsConn := conn.(*tls.Conn)
		tlsConn.Handshake()
		conn.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	s := NewHTTPSScanner(5 * time.Second)
	result, err := s.Scan(context.Background(), ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if result.Subject == "" {
		t.Error("expected subject to be non-empty")
	}
	if result.Fingerprint == "" {
		t.Error("expected fingerprint to be non-empty")
	}
	if result.Status != "valid" {
		t.Errorf("expected valid, got %s", result.Status)
	}
}

func TestHTTPSScannerConnectionRefused(t *testing.T) {
	s := NewHTTPSScanner(1 * time.Second)
	_, err := s.Scan(context.Background(), "127.0.0.1:1")
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestScannerStubs(t *testing.T) {
	timeout := 100 * time.Millisecond
	scanners := []Scanner{
		NewSMTPScanner(timeout),
		NewIMAPScanner(timeout),
		NewPOP3Scanner(timeout),
		NewLDAPScanner(timeout),
		NewFTPScanner(timeout),
		NewTLSScanner(timeout),
		NewCTScanner(timeout),
	}
	for _, s := range scanners {
		_, err := s.Scan(context.Background(), "example.com")
		if err == nil {
			t.Errorf("%s scanner: expected error, got nil", s.Protocol())
		}
	}
}

func generateTestCertPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         true,
		BasicConstraintsValid: true,
		DNSNames:              []string{"test.example.com"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
