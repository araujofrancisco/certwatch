package services

import (
	"github.com/araujofrancisco/certwatch/internal/models"
)

type PaginatedCertificates struct {
	Certificates []*models.Certificate `json:"certificates"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	Limit        int                   `json:"limit"`
}

func (s *CertificateService) ListCertificates() ([]*models.Certificate, error) {
	return s.certs.List()
}

func (s *CertificateService) ListCertificatesFiltered(f models.CertFilter) ([]*models.Certificate, error) {
	return s.certs.ListFiltered(f)
}

func (s *CertificateService) ListCertificatesPaginated(f models.CertFilter) (*PaginatedCertificates, error) {
	certs, err := s.certs.ListFiltered(f)
	if err != nil {
		return nil, err
	}
	total, err := s.certs.CountFiltered(f)
	if err != nil {
		return nil, err
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	return &PaginatedCertificates{Certificates: certs, Total: total, Page: page, Limit: f.Limit}, nil
}

func (s *CertificateService) PurgeExpired() (int64, error) {
	return s.certs.DeleteExpired()
}

func (s *CertificateService) PurgeExpiredByDomain(domainID int64) (int64, error) {
	return s.certs.DeleteExpiredByDomain(domainID)
}

func (s *CertificateService) PurgeErrors() (int64, error) {
	return s.certs.DeleteErrors()
}

func (s *CertificateService) PurgeErrorsByDomain(domainID int64) (int64, error) {
	return s.certs.DeleteErrorsByDomain(domainID)
}
