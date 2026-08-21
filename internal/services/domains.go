package services

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/araujofrancisco/certwatch/internal/ctsearch"
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

func (s *DomainService) attachTags(domains []*models.Domain) {
	if len(domains) == 0 {
		return
	}
	ids := make([]int64, len(domains))
	for i, d := range domains {
		ids[i] = d.ID
	}
	tagMap, err := s.tags.GetTagsByDomainIDs(ids)
	if err != nil {
		return
	}
	for _, d := range domains {
		if ptags, ok := tagMap[d.ID]; ok {
			d.Tags = derefTags(ptags)
		}
	}
}

type PaginatedDomains struct {
	Domains []*models.Domain `json:"domains"`
	Total   int              `json:"total"`
	Page    int              `json:"page"`
	Limit   int              `json:"limit"`
}

func (s *DomainService) ListDomainsPaginated(filter models.DomainFilter) (*PaginatedDomains, error) {
	domains, err := s.domains.ListFiltered(filter)
	if err != nil {
		return nil, err
	}
	s.attachTags(domains)
	total, err := s.domains.CountFiltered(filter)
	if err != nil {
		return nil, err
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	return &PaginatedDomains{Domains: domains, Total: total, Page: page, Limit: filter.Limit}, nil
}

func (s *DomainService) ListDomains() ([]*models.Domain, error) {
	domains, err := s.domains.List()
	if err != nil {
		return nil, err
	}
	s.attachTags(domains)
	return domains, nil
}

func (s *DomainService) ListDomainsFiltered(f models.DomainFilter) ([]*models.Domain, error) {
	domains, err := s.domains.ListFiltered(f)
	if err != nil {
		return nil, err
	}
	s.attachTags(domains)
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
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	// HTTPS is fast (a single handshake); CT aggregation fans out across
	// multiple providers and should get the bulk of the scan budget so a
	// slow provider does not starve the rest.
	ctBudget := timeout - 5*time.Second
	if ctBudget < 15*time.Second {
		ctBudget = 15 * time.Second
	}
	priorityOrder := []struct {
		protocol string
		timeout  time.Duration
	}{
		{"https", 5 * time.Second},
		{"ct", ctBudget},
	}

	var lastErr error
	var saved []*models.Certificate
	for _, p := range priorityOrder {
		scanner := s.scanners.ForProtocol(p.protocol)
		if scanner == nil {
			continue
		}

		scanCtx, cancel := context.WithTimeout(ctx, p.timeout)
		results, scanErr := scanAll(scanner, scanCtx, d.Domain)
		cancel()
		if scanErr != nil {
			lastErr = scanErr
			continue
		}
		for _, r := range results {
			if r == nil {
				continue
			}
			// Never ingest expired certificates. CT logs replay every
			// historical issuance for a domain, so without this filter a
			// purged expired cert would be re-added on the next scan. The
			// predicate mirrors DeleteExpired/DeleteExpiredByDomain so what
			// the purge removes is exactly what ingestion refuses to store.
			if !r.NotAfter.IsZero() && r.NotAfter.Before(time.Now()) {
				continue
			}
			saved = append(saved, s.saveCertificate(d.ID, r))
		}
	}

	if len(saved) == 0 {
		if lastErr == nil {
			// Scanners succeeded but found no live certificate (all results
			// were expired). Not an error: record nothing and let the domain
			// simply have no stored cert.
			slog.Info("scan found no valid certificates", "domain_id", d.ID, "domain", d.Domain)
			return nil, nil
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

	return saved[0], nil
}

// scanAll gathers every certificate a scanner reports for a domain. Scanners
// that implement MultiScanner (e.g. CT) return their full result set; plain
// scanners (e.g. HTTPS, which serves a single leaf) return a single result.
func scanAll(scanner discovery.Scanner, ctx context.Context, domain string) ([]*discovery.Result, error) {
	if ms, ok := scanner.(discovery.MultiScanner); ok {
		return ms.ScanAll(ctx, domain)
	}
	r, err := scanner.Scan(ctx, domain)
	if err != nil {
		return nil, err
	}
	return []*discovery.Result{r}, nil
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
		SANs:        result.SANs,
		LastChecked: time.Now(),
	}

	// Dedup against every stored certificate for the domain (not just the
	// latest) by fingerprint or serial+issuer, so a scan that returns
	// multiple certs does not create duplicates on subsequent scans.
	if existing := s.findExistingCert(domainID, result); existing != nil {
		return s.updateCert(existing, result, cert)
	}

	if err := s.certs.Create(cert); err != nil {
		slog.Error("failed to save certificate", "domain_id", domainID, "error", err)
	}

	// Renewal purge: when a scan inserts a NEW valid certificate, opportunistically
	// remove old expired certificates for the same domain so history does not
	// accumulate across renewal cycles. This only fires for genuinely new valid
	// certs — inserting an expired historical cert (common from CT logs) must not
	// trigger a purge, since that cert itself is expired and would be deleted
	// immediately, and a purge must never run while expired certs are being
	// inserted in bulk (it could delete valid rows that an interleaving scan
	// has not yet normalized).
	if cert.Status == "valid" {
		if n, err := s.certs.DeleteExpiredByDomain(domainID); err != nil {
			slog.Error("failed to purge expired certificates on renewal", "domain_id", domainID, "error", err)
		} else if n > 0 {
			slog.Info("purged expired certificates on renewal", "domain_id", domainID, "deleted", n)
		}
	}

	return cert
}

func (s *DomainService) findExistingCert(domainID int64, result *discovery.Result) *models.Certificate {
	existing, err := s.certs.ListByDomainID(domainID)
	if err != nil {
		return nil
	}
	// Fingerprint match first: it is the strongest signal (HTTPS uses the
	// real SHA-256 of the DER body; CT providers derive a stable key hash).
	for _, c := range existing {
		if c.Fingerprint != "" && c.Fingerprint == result.Fingerprint {
			return c
		}
	}
	// Serial+issuer fallback, comparing canonical forms so rows written before
	// DN/serial normalization still match (e.g. ctlogs.dev CN-first issuer vs
	// CertSpotter C-first, or colon-separated serials).
	for _, c := range existing {
		if c.Serial == "" || result.Serial == "" {
			continue
		}
		if ctsearch.NormalizeSerial(c.Serial) == ctsearch.NormalizeSerial(result.Serial) &&
			ctsearch.NormalizeDN(c.Issuer) == ctsearch.NormalizeDN(result.Issuer) {
			return c
		}
	}
	// Empty-serial fallback: when a provider omits the serial (CertSpotter for
	// some issuances), the serial+issuer path cannot match. Fall back to a
	// match on normalized issuer + subject + the same expiry date, so distinct
	// renewals from the same CA (same issuer+subject, different validity
	// windows) are not collapsed into one row.
	for _, c := range existing {
		if c.Serial != "" && result.Serial != "" {
			continue
		}
		if c.Issuer == "" || result.Issuer == "" || c.Subject == "" || result.Subject == "" {
			continue
		}
		if ctsearch.NormalizeDN(c.Issuer) != ctsearch.NormalizeDN(result.Issuer) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(c.Subject), strings.TrimSpace(result.Subject)) {
			continue
		}
		// Match on the same calendar day so providers that truncate seconds
		// (ctlogs.dev shows 23:59:00 where the cert is 23:59:59) still dedup,
		// while distinct renewals — which expire on different days — stay
		// separate rows.
		if !sameDay(c.NotAfter, result.NotAfter) {
			continue
		}
		return c
	}
	return nil
}

// sameDay reports whether two instants fall on the same UTC calendar day. Zero
// values never match so an unknown expiry never collapses distinct certs.
func sameDay(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func (s *DomainService) updateCert(existing *models.Certificate, result *discovery.Result, fresh *models.Certificate) *models.Certificate {
	existing.Status = result.Status
	existing.LastChecked = time.Now()
	existing.NotAfter = result.NotAfter
	existing.NotBefore = result.NotBefore
	existing.Issuer = result.Issuer
	existing.Subject = result.Subject
	existing.SANs = result.SANs
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

		s.EnqueueScan(s.backgroundCtx, d.ID, false)

		summary.Created++
		results = append(results, res)
	}

	summary.Total = summary.Created + summary.Skipped + summary.Errors
	return &BulkAddResponse{Results: results, Summary: summary}
}

func (s *DomainService) ScanAllDomains(ctx context.Context) error {
	domains, err := s.domains.ListEnabled()
	if err != nil {
		return err
	}

	// Enqueue every enabled domain for a background scan. The queue worker pool
	// provides the concurrency cap and deduplication; this call only submits
	// the work and returns.
	for _, d := range domains {
		s.EnqueueScan(ctx, d.ID, false)
	}
	return nil
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
