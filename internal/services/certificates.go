package services

import (
	"time"

	"github.com/araujofrancisco/certwatch/internal/models"
)

func (s *CertificateService) ListCertificates() ([]*models.Certificate, error) {
	return s.certs.List()
}

func (s *CertificateService) ListCertificatesFiltered(f models.CertFilter) ([]*models.Certificate, error) {
	return s.certs.ListFiltered(f)
}

func (s *CertificateService) ListByDomain(domainID int64) ([]*models.Certificate, error) {
	return s.certs.ListByDomainID(domainID)
}

func (s *CertificateService) GetCertificate(id int64) (*models.Certificate, error) {
	return s.certs.FindByID(id)
}

func (s *CertificateService) ExpiringCertificates(thresholdDays int) ([]*models.Certificate, error) {
	all, err := s.certs.List()
	if err != nil {
		return nil, err
	}
	threshold := time.Now().AddDate(0, 0, thresholdDays)
	var expiring []*models.Certificate
	for _, c := range all {
		if !c.NotAfter.IsZero() && c.NotAfter.Before(threshold) && c.NotAfter.After(time.Now()) {
			expiring = append(expiring, c)
		}
	}
	return expiring, nil
}

func (s *CertificateService) PurgeErrors() (int64, error) {
	return s.certs.DeleteErrors()
}

func (s *CertificateService) PurgeErrorsByDomain(domainID int64) (int64, error) {
	return s.certs.DeleteErrorsByDomain(domainID)
}

func (s *CertificateService) ExpiredCertificates() ([]*models.Certificate, error) {
	all, err := s.certs.List()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var expired []*models.Certificate
	for _, c := range all {
		if !c.NotAfter.IsZero() && c.NotAfter.Before(now) {
			expired = append(expired, c)
		}
	}
	return expired, nil
}
