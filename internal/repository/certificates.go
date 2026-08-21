package repository

import (
	"context"

	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

func encodeSANs(sans []string) (string, error) {
	if len(sans) == 0 {
		return "", nil
	}
	b, err := json.Marshal(sans)
	if err != nil {
		return "", fmt.Errorf("encode sans: %w", err)
	}
	return string(b), nil
}

type certRepo struct {
	db *database.DB
}

// utcOrZero normalizes timestamps to UTC before they reach SQLite. The
// driver serializes time.Time with its original offset, so mixed-offset
// values would break lexicographic datetime comparisons. Zero times become
// NULL — otherwise they would serialize as "0001-01-01T00:00:00Z" and pollute
// range comparisons.
func utcOrZero(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

func (r *certRepo) Create(ctx context.Context, c *models.Certificate) error {
	sans, err := encodeSANs(c.SANs)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO certificates (domain_id, issuer, subject, serial, not_before, not_after, fingerprint, protocol, status, sans, last_checked)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.DomainID, c.Issuer, c.Subject, c.Serial, utcOrZero(c.NotBefore), utcOrZero(c.NotAfter),
		c.Fingerprint, c.Protocol, c.Status, sans, utcOrZero(c.LastChecked),
	)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

func (r *certRepo) FindByID(ctx context.Context, id int64) (*models.Certificate, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, sans, last_checked, created_at, updated_at
		 FROM certificates WHERE id = ?`, id,
	)
	return scanCert(row)
}

func (r *certRepo) ListByDomainID(ctx context.Context, domainID int64) ([]*models.Certificate, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, sans, last_checked, created_at, updated_at
		 FROM certificates WHERE domain_id = ? ORDER BY not_after DESC`, domainID,
	)
	if err != nil {
		return nil, fmt.Errorf("list certificates by domain: %w", err)
	}
	defer rows.Close()
	return scanCerts(rows)
}

func (r *certRepo) ListFiltered(ctx context.Context, filter models.CertFilter) ([]*models.Certificate, error) {
	var clauses []string
	var args []any

	if filter.Query != "" {
		clauses = append(clauses, "(subject LIKE ? OR issuer LIKE ?)")
		q := "%" + filter.Query + "%"
		args = append(args, q, q)
	}
	if filter.DomainID != nil {
		clauses = append(clauses, "domain_id = ?")
		args = append(args, *filter.DomainID)
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Protocol != "" {
		clauses = append(clauses, "protocol = ?")
		args = append(args, filter.Protocol)
	}
	if filter.Expiring > 0 {
		clauses = append(clauses, "not_after > datetime('now') AND not_after <= datetime('now', '+' || ? || ' days')")
		args = append(args, filter.Expiring)
	}
	if filter.Expired {
		clauses = append(clauses, "not_after < datetime('now')")
	}

	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	query := `SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
	          fingerprint, protocol, status, sans, last_checked, created_at, updated_at
	         FROM certificates` + where + ` ORDER BY not_after DESC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", filter.Limit, filter.Offset())
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list filtered certificates: %w", err)
	}
	defer rows.Close()
	return scanCerts(rows)
}

func (r *certRepo) CountFiltered(ctx context.Context, filter models.CertFilter) (int, error) {
	var clauses []string
	var args []any

	if filter.Query != "" {
		clauses = append(clauses, "(subject LIKE ? OR issuer LIKE ?)")
		q := "%" + filter.Query + "%"
		args = append(args, q, q)
	}
	if filter.DomainID != nil {
		clauses = append(clauses, "domain_id = ?")
		args = append(args, *filter.DomainID)
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Protocol != "" {
		clauses = append(clauses, "protocol = ?")
		args = append(args, filter.Protocol)
	}
	if filter.Expiring > 0 {
		clauses = append(clauses, "not_after > datetime('now') AND not_after <= datetime('now', '+' || ? || ' days')")
		args = append(args, filter.Expiring)
	}
	if filter.Expired {
		clauses = append(clauses, "not_after < datetime('now')")
	}

	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM certificates`+where, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count filtered certificates: %w", err)
	}
	return count, nil
}

func (r *certRepo) List(ctx context.Context) ([]*models.Certificate, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, sans, last_checked, created_at, updated_at
		 FROM certificates ORDER BY not_after DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	defer rows.Close()
	return scanCerts(rows)
}

func (r *certRepo) Update(ctx context.Context, c *models.Certificate) error {
	sans, err := encodeSANs(c.SANs)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE certificates SET issuer = ?, subject = ?, serial = ?, not_before = ?, not_after = ?,
		     fingerprint = ?, protocol = ?, status = ?, sans = ?, last_checked = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		c.Issuer, c.Subject, c.Serial, utcOrZero(c.NotBefore), utcOrZero(c.NotAfter),
		c.Fingerprint, c.Protocol, c.Status, sans, utcOrZero(c.LastChecked), c.ID,
	)
	if err != nil {
		return fmt.Errorf("update certificate: %w", err)
	}
	return nil
}

func (r *certRepo) DeleteErrors(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM certificates WHERE status = 'error'`)
	if err != nil {
		return 0, fmt.Errorf("delete error certificates: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *certRepo) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM certificates WHERE not_after < datetime('now')`)
	if err != nil {
		return 0, fmt.Errorf("delete expired certificates: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *certRepo) DeleteExpiredByDomain(ctx context.Context, domainID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM certificates WHERE domain_id = ? AND not_after < datetime('now')`, domainID)
	if err != nil {
		return 0, fmt.Errorf("delete expired certificates by domain: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *certRepo) DeleteErrorsByDomain(ctx context.Context, domainID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM certificates WHERE domain_id = ? AND status = 'error'`, domainID)
	if err != nil {
		return 0, fmt.Errorf("delete error certificates by domain: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *certRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM certificates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete certificate: %w", err)
	}
	return nil
}

func scanCert(s scanner) (*models.Certificate, error) {
	var c models.Certificate
	var notBefore, notAfter, lastChecked, createdAt, updatedAt sql.NullTime
	var sans string
	err := s.Scan(&c.ID, &c.DomainID, &c.Issuer, &c.Subject, &c.Serial,
		&notBefore, &notAfter, &c.Fingerprint, &c.Protocol, &c.Status,
		&sans, &lastChecked, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("certificate: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("scan certificate: %w", err)
	}
	if notBefore.Valid {
		c.NotBefore = notBefore.Time
	}
	if notAfter.Valid {
		c.NotAfter = notAfter.Time
	}
	if lastChecked.Valid {
		c.LastChecked = lastChecked.Time
	}
	if createdAt.Valid {
		c.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		c.UpdatedAt = updatedAt.Time
	}
	if sans != "" {
		if err := json.Unmarshal([]byte(sans), &c.SANs); err != nil {
			return nil, fmt.Errorf("decode sans: %w", err)
		}
	}
	return &c, nil
}

func scanCerts(r *sql.Rows) ([]*models.Certificate, error) {
	var certs []*models.Certificate
	for r.Next() {
		c, err := scanCert(r)
		if err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, r.Err()
}

// CertBucketCounts holds dashboard expiry-bucket aggregates computed in SQL
// so the dashboard stays O(1) regardless of table size.
type CertBucketCounts struct {
	Healthy int
	Warning int
	Expired int
}

// CountExpiryBuckets counts certificates as expired (not_after <
// warningStart), warning (warningStart <= not_after < warningEnd), or healthy
// (>= warningEnd). Rows with a NULL not_after are failed scans (no real
// certificate was seen) and belong in no bucket. Callers pass precomputed
// cutoff times so the bucket boundaries live in one place, next to their
// consumers. Both stored values and bound parameters are UTC (see utcOrZero),
// so plain string comparison is order-correct.
func (r *certRepo) CountExpiryBuckets(ctx context.Context, warningStart, warningEnd time.Time) (CertBucketCounts, error) {
	var c CertBucketCounts
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN not_after IS NOT NULL AND not_after >= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN not_after IS NOT NULL AND not_after >= ? AND not_after < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN not_after IS NOT NULL AND not_after < ? THEN 1 ELSE 0 END), 0)
		FROM certificates`,
		warningEnd.UTC(), warningStart.UTC(), warningEnd.UTC(), warningStart.UTC(),
	).Scan(&c.Healthy, &c.Warning, &c.Expired)
	if err != nil {
		return CertBucketCounts{}, fmt.Errorf("count expiry buckets: %w", err)
	}
	return c, nil
}

// ExpiringSoonCert is a certificate inside the expiry window, joined with its
// domain name for dashboard rendering.
type ExpiringSoonCert struct {
	CertificateID int64     `json:"certificate_id"`
	DomainID      int64     `json:"domain_id"`
	Domain        string    `json:"domain"`
	Issuer        string    `json:"issuer"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (r *certRepo) ListExpiringSoon(ctx context.Context, from, before time.Time, limit int) ([]ExpiringSoonCert, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.domain_id, d.domain, c.issuer, c.not_after
		FROM certificates c
		JOIN domains d ON d.id = c.domain_id
		WHERE c.not_after IS NOT NULL AND c.not_after >= ? AND c.not_after < ?
		ORDER BY c.not_after ASC
		LIMIT ?`,
		from.UTC(), before.UTC(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list expiring soon: %w", err)
	}
	defer rows.Close()

	var out []ExpiringSoonCert
	for rows.Next() {
		var e ExpiringSoonCert
		if err := rows.Scan(&e.CertificateID, &e.DomainID, &e.Domain, &e.Issuer, &e.ExpiresAt); err != nil {
			return nil, fmt.Errorf("list expiring soon: scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListLatestByDomain returns the most recently checked certificate for each
// domain. Ties on last_checked resolve to the newest row (highest id), so
// exactly one row per domain is returned.
func (r *certRepo) ListLatestByDomain(ctx context.Context) ([]*models.Certificate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.domain_id, c.issuer, c.subject, c.serial, c.not_before, c.not_after,
		       c.fingerprint, c.protocol, c.status, c.sans, c.last_checked, c.created_at, c.updated_at
		 FROM certificates c
		 WHERE c.id = (
		     SELECT id FROM certificates c2
		     WHERE c2.domain_id = c.domain_id
		     ORDER BY last_checked DESC, id DESC
		     LIMIT 1
		 )`,
	)
	if err != nil {
		return nil, fmt.Errorf("list latest certs by domain: %w", err)
	}
	defer rows.Close()
	return scanCerts(rows)
}
