package services

import (
	"context"
	"time"

	"github.com/araujofrancisco/certwatch/internal/models"
	"github.com/araujofrancisco/certwatch/internal/repository"
)

type PaginatedCertificates struct {
	Certificates []*models.Certificate `json:"certificates"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	Limit        int                   `json:"limit"`
}

func (s *CertificateService) ListCertificates(ctx context.Context) ([]*models.Certificate, error) {
	return s.certs.List(ctx)
}

// ListLatestCertificates returns the most recently checked certificate per
// domain (used by the inventory report).
func (s *CertificateService) ListLatestCertificates(ctx context.Context) ([]*models.Certificate, error) {
	return s.certs.ListLatestByDomain(ctx)
}

// CountExpiryBuckets aggregates dashboard counts in SQL.
func (s *CertificateService) CountExpiryBuckets(ctx context.Context, warningStart, warningEnd time.Time) (repository.CertBucketCounts, error) {
	return s.certs.CountExpiryBuckets(ctx, warningStart, warningEnd)
}

// ListExpiringSoon returns certificates expiring inside the window, ordered
// by expiry, capped at limit.
func (s *CertificateService) ListExpiringSoon(ctx context.Context, from, before time.Time, limit int) ([]repository.ExpiringSoonCert, error) {
	return s.certs.ListExpiringSoon(ctx, from, before, limit)
}

func (s *CertificateService) ListCertificatesFiltered(ctx context.Context, f models.CertFilter) ([]*models.Certificate, error) {
	return s.certs.ListFiltered(ctx, f)
}

func (s *CertificateService) ListCertificatesPaginated(ctx context.Context, f models.CertFilter) (*PaginatedCertificates, error) {
	certs, err := s.certs.ListFiltered(ctx, f)
	if err != nil {
		return nil, err
	}
	total, err := s.certs.CountFiltered(ctx, f)
	if err != nil {
		return nil, err
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	return &PaginatedCertificates{Certificates: certs, Total: total, Page: page, Limit: f.Limit}, nil
}

func (s *CertificateService) PurgeExpired(ctx context.Context) (int64, error) {
	return s.certs.DeleteExpired(ctx)
}

func (s *CertificateService) PurgeExpiredByDomain(ctx context.Context, domainID int64) (int64, error) {
	return s.certs.DeleteExpiredByDomain(ctx, domainID)
}

func (s *CertificateService) PurgeErrors(ctx context.Context) (int64, error) {
	return s.certs.DeleteErrors(ctx)
}

func (s *CertificateService) PurgeErrorsByDomain(ctx context.Context, domainID int64) (int64, error) {
	return s.certs.DeleteErrorsByDomain(ctx, domainID)
}
