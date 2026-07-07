package services

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/models"
)

func (s *DomainService) AddDomain(domain, description, group string) (*models.Domain, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if !isValidDomain(domain) {
		return nil, fmt.Errorf("invalid domain name: %q", domain)
	}
	description = strings.TrimSpace(description)
	group = strings.TrimSpace(group)
	if len(description) > 500 {
		return nil, fmt.Errorf("description too long (max 500)")
	}
	if len(group) > 100 {
		return nil, fmt.Errorf("group too long (max 100)")
	}
	d := &models.Domain{
		Domain:      domain,
		Description: description,
		Group:       group,
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
	d, err := s.domains.FindByID(id)
	if err != nil {
		return nil, err
	}
	ptags, err := s.tags.GetDomainTags(d.ID)
	if err == nil {
		d.Tags = derefTags(ptags)
	}
	return d, nil
}

func (s *DomainService) ListDomains() ([]*models.Domain, error) {
	domains, err := s.domains.List()
	if err != nil {
		return nil, err
	}
	for _, d := range domains {
		ptags, err := s.tags.GetDomainTags(d.ID)
		if err == nil {
			d.Tags = derefTags(ptags)
		}
	}
	return domains, nil
}

func (s *DomainService) ListDomainsFiltered(f models.DomainFilter) ([]*models.Domain, error) {
	domains, err := s.domains.ListFiltered(f)
	if err != nil {
		return nil, err
	}
	for _, d := range domains {
		ptags, err := s.tags.GetDomainTags(d.ID)
		if err == nil {
			d.Tags = derefTags(ptags)
		}
	}
	return domains, nil
}

func derefTags(ptags []*models.Tag) []models.Tag {
	tags := make([]models.Tag, len(ptags))
	for i, t := range ptags {
		tags[i] = *t
	}
	return tags
}

func (s *DomainService) DeleteDomain(id int64) error {
	return s.domains.Delete(id)
}

func (s *DomainService) ScanDomain(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
	d, err := s.domains.FindByID(domainID)
	if err != nil {
		return nil, err
	}

	priorityOrder := []struct {
		protocol string
		timeout  time.Duration
	}{
		{"https", 5 * time.Second},
		{"ct", 10 * time.Second},
	}

	var lastErr error
	for _, p := range priorityOrder {
		scanner := s.scanners.ForProtocol(p.protocol)
		if scanner == nil {
			continue
		}

		scanCtx, cancel := context.WithTimeout(ctx, p.timeout)
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
	if err := s.certs.Create(cert); err != nil {
		slog.Error("failed to save error cert", "domain_id", d.ID, "error", err)
	}
	return cert, fmt.Errorf("all scanners failed: %w", lastErr)
}

func (s *DomainService) UpdateDomain(id int64, domain, description, group string, enabled bool, tags []string) (*models.Domain, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if !isValidDomain(domain) {
		return nil, fmt.Errorf("invalid domain name: %q", domain)
	}
	description = strings.TrimSpace(description)
	group = strings.TrimSpace(group)
	if len(description) > 500 {
		return nil, fmt.Errorf("description too long (max 500)")
	}
	if len(group) > 100 {
		return nil, fmt.Errorf("group too long (max 100)")
	}

	d, err := s.domains.FindByID(id)
	if err != nil {
		return nil, err
	}

	d.Domain = domain
	d.Description = description
	d.Group = group
	d.Enabled = enabled

	if err := s.domains.Update(d); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("domain already exists")
		}
		return nil, err
	}

	if tags != nil {
		if err := s.SetDomainTags(d.ID, tags); err != nil {
			return nil, err
		}
	}

	return s.GetDomain(d.ID)
}

func (s *DomainService) SetDomainTags(domainID int64, tagNames []string) error {
	tags, err := s.ensureTags(tagNames)
	if err != nil {
		return err
	}
	var ids []int64
	for _, t := range tags {
		ids = append(ids, t.ID)
	}
	return s.tags.SetDomainTags(domainID, ids)
}

func (s *DomainService) ensureTags(names []string) ([]*models.Tag, error) {
	var result []*models.Tag
	for _, n := range names {
		tag, err := s.tags.FindByName(n)
		if err != nil {
			tag, err = s.tags.Create(n, randomTagColor())
			if err != nil {
				return nil, err
			}
		}
		result = append(result, tag)
	}
	return result, nil
}

func randomTagColor() string {
	palette := []string{
		"#0d6efd", "#6610f2", "#6f42c1", "#d63384", "#dc3545",
		"#fd7e14", "#ffc107", "#198754", "#20c997", "#0dcaf0",
	}
	return palette[rand.Intn(len(palette))]
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

	if err := s.certs.Create(cert); err != nil {
		slog.Error("failed to save certificate", "domain_id", domainID, "error", err)
	}
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
	if err := s.certs.Update(existing); err != nil {
		slog.Error("failed to update certificate", "cert_id", existing.ID, "error", err)
	}
	return existing
}

type BulkDomainEntry struct {
	Domain      string
	Description string
	Group       string
	Tags        []string
}

type BulkAddResult struct {
	Domain      string   `json:"domain"`
	Status      string   `json:"status"` // "created", "skipped", "error"
	Error       string   `json:"error,omitempty"`
	Description string   `json:"description,omitempty"`
	Group       string   `json:"group,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type BulkAddSummary struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Errors  int `json:"errors"`
}

type BulkAddResponse struct {
	Results []*BulkAddResult `json:"results"`
	Summary BulkAddSummary   `json:"summary"`
}

func (s *DomainService) BulkAddDomains(pairs []BulkDomainEntry) *BulkAddResponse {
	var results []*BulkAddResult
	var summary BulkAddSummary

	for _, p := range pairs {
		res := &BulkAddResult{Domain: p.Domain, Description: p.Description, Group: p.Group, Tags: p.Tags}

		domain := strings.TrimSpace(strings.ToLower(p.Domain))
		if domain == "" {
			res.Status = "skipped"
			res.Error = "empty domain"
			summary.Skipped++
			results = append(results, res)
			continue
		}
		if !isValidDomain(domain) {
			res.Status = "error"
			res.Error = "invalid domain name"
			summary.Errors++
			results = append(results, res)
			continue
		}

		existing, err := s.domains.FindByDomain(domain)
		if err == nil && existing != nil {
			res.Status = "skipped"
			res.Error = "already exists"
			summary.Skipped++
			results = append(results, res)
			continue
		}

		d := &models.Domain{
			Domain:      domain,
			Description: p.Description,
			Group:       p.Group,
			Enabled:     true,
		}
		if err := s.domains.Create(d); err != nil {
			res.Status = "error"
			res.Error = err.Error()
			summary.Errors++
			results = append(results, res)
			continue
		}

		if len(p.Tags) > 0 {
			if err := s.SetDomainTags(d.ID, p.Tags); err != nil {
				slog.Error("failed to set tags on bulk import", "domain_id", d.ID, "error", err)
			}
		}

		res.Status = "created"

		go func(id int64) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := s.ScanDomain(ctx, id, 30*time.Second); err != nil {
				slog.Error("bulk import background scan failed", "domain_id", id, "error", err)
			}
		}(d.ID)

		summary.Created++
		results = append(results, res)
	}

	summary.Total = summary.Created + summary.Skipped + summary.Errors
	return &BulkAddResponse{Results: results, Summary: summary}
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
