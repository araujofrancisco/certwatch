package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/models"
)

func (s *DomainService) AddDomain(domain, description string) (*models.Domain, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if !isValidDomain(domain) {
		return nil, fmt.Errorf("invalid domain name: %q", domain)
	}
	d := &models.Domain{
		Domain:      domain,
		Description: description,
		Enabled:     true,
	}
	if err := s.domains.Create(d); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("domain already exists")
		}
		return nil, err
	}
	return d, nil
}

func (s *DomainService) GetDomain(id int64) (*models.Domain, error) {
	return s.domains.FindByID(id)
}

func (s *DomainService) ListDomains() ([]*models.Domain, error) {
	return s.domains.List()
}

func (s *DomainService) ListDomainsFiltered(f models.DomainFilter) ([]*models.Domain, error) {
	return s.domains.ListFiltered(f)
}

func (s *DomainService) DeleteDomain(id int64) error {
	return s.domains.Delete(id)
}

func (s *DomainService) ScanDomain(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
	d, err := s.domains.FindByID(domainID)
	if err != nil {
		return nil, err
	}

	priorityOrder := []string{"https", "ct", "smtp", "imap", "pop3", "ldap", "ftp", "tls"}
	scannerTimeouts := map[string]time.Duration{
		"https": 5 * time.Second,
		"ct":    10 * time.Second,
	}

	var lastErr error
	for _, protocol := range priorityOrder {
		scanner := s.scanners.ForProtocol(protocol)
		if scanner == nil {
			continue
		}

		perTimeout := scannerTimeouts[protocol]
		if perTimeout == 0 {
			perTimeout = 2 * time.Second
		}

		scanCtx, cancel := context.WithTimeout(ctx, perTimeout)
		result, err := scanner.Scan(scanCtx, d.Domain)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		return s.saveCertificate(d.ID, result), nil
	}

	cert := &models.Certificate{
		DomainID:    d.ID,
		Protocol:    "unknown",
		Status:      "error",
		LastChecked: time.Now(),
	}
	_ = s.certs.Create(cert)
	return cert, fmt.Errorf("all scanners failed: %w", lastErr)
}

func (s *DomainService) saveCertificate(domainID int64, result *discovery.Result) *models.Certificate {
	cert := &models.Certificate{
		DomainID:    domainID,
		Issuer:      result.Issuer,
		Subject:     result.Subject,
		Serial:      result.Serial,
		NotBefore:   result.NotBefore,
		NotAfter:    result.NotAfter,
		Fingerprint: result.Fingerprint,
		Protocol:    result.Protocol,
		Status:      result.Status,
		LastChecked: time.Now(),
	}

	existing, err := s.certs.LatestForDomain(domainID)
	if err == nil {
		if existing.Fingerprint != "" && existing.Fingerprint == result.Fingerprint {
			return s.updateCert(existing, result, cert)
		}
		if existing.Serial != "" && existing.Serial == result.Serial && existing.Issuer == result.Issuer {
			return s.updateCert(existing, result, cert)
		}
	}

	_ = s.certs.Create(cert)
	return cert
}

func (s *DomainService) updateCert(existing *models.Certificate, result *discovery.Result, fresh *models.Certificate) *models.Certificate {
	existing.Status = result.Status
	existing.LastChecked = time.Now()
	existing.NotAfter = result.NotAfter
	existing.NotBefore = result.NotBefore
	existing.Issuer = result.Issuer
	existing.Subject = result.Subject
	if result.Fingerprint != "" {
		existing.Fingerprint = result.Fingerprint
	}
	existing.Protocol = result.Protocol
	_ = s.certs.Update(existing)
	return existing
}

func (s *DomainService) ScanAllDomains(ctx context.Context, timeout time.Duration) ([]*models.Certificate, error) {
	domains, err := s.domains.ListEnabled()
	if err != nil {
		return nil, err
	}
	var certs []*models.Certificate
	for _, d := range domains {
		cert, err := s.ScanDomain(ctx, d.ID, timeout)
		if err != nil {
			continue
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

func isValidDomain(domain string) bool {
	if len(domain) > 253 {
		return false
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 63 {
			return false
		}
		if p[0] == '-' || p[len(p)-1] == '-' {
			return false
		}
		for _, c := range p {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}
