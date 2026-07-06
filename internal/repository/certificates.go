package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type certRepo struct {
	db *database.DB
}

func (r *certRepo) Create(c *models.Certificate) error {
	res, err := r.db.Exec(
		`INSERT INTO certificates (domain_id, issuer, subject, serial, not_before, not_after, fingerprint, protocol, status, last_checked)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.DomainID, c.Issuer, c.Subject, c.Serial, c.NotBefore, c.NotAfter,
		c.Fingerprint, c.Protocol, c.Status, c.LastChecked,
	)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

func (r *certRepo) FindByID(id int64) (*models.Certificate, error) {
	row := r.db.QueryRow(
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, last_checked, created_at, updated_at
		 FROM certificates WHERE id = ?`, id,
	)
	return scanCert(row)
}

func (r *certRepo) ListByDomainID(domainID int64) ([]*models.Certificate, error) {
	rows, err := r.db.Query(
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, last_checked, created_at, updated_at
		 FROM certificates WHERE domain_id = ? ORDER BY not_after DESC`, domainID,
	)
	if err != nil {
		return nil, fmt.Errorf("list certificates by domain: %w", err)
	}
	defer rows.Close()
	return scanCerts(rows)
}

func (r *certRepo) LatestForDomain(domainID int64) (*models.Certificate, error) {
	row := r.db.QueryRow(
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, last_checked, created_at, updated_at
		 FROM certificates WHERE domain_id = ? ORDER BY last_checked DESC LIMIT 1`, domainID,
	)
	return scanCert(row)
}

func (r *certRepo) ListFiltered(filter models.CertFilter) ([]*models.Certificate, error) {
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

	rows, err := r.db.Query(
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, last_checked, created_at, updated_at
		 FROM certificates`+where+` ORDER BY not_after DESC`, args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list filtered certificates: %w", err)
	}
	defer rows.Close()
	return scanCerts(rows)
}

func (r *certRepo) List() ([]*models.Certificate, error) {
	rows, err := r.db.Query(
		`SELECT id, domain_id, issuer, subject, serial, not_before, not_after,
		        fingerprint, protocol, status, last_checked, created_at, updated_at
		 FROM certificates ORDER BY not_after DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	defer rows.Close()
	return scanCerts(rows)
}

func (r *certRepo) Update(c *models.Certificate) error {
	_, err := r.db.Exec(
		`UPDATE certificates SET issuer = ?, subject = ?, serial = ?, not_before = ?, not_after = ?,
		     fingerprint = ?, protocol = ?, status = ?, last_checked = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		c.Issuer, c.Subject, c.Serial, c.NotBefore, c.NotAfter,
		c.Fingerprint, c.Protocol, c.Status, c.LastChecked, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update certificate: %w", err)
	}
	return nil
}

func (r *certRepo) DeleteErrors() (int64, error) {
	res, err := r.db.Exec(`DELETE FROM certificates WHERE status = 'error'`)
	if err != nil {
		return 0, fmt.Errorf("delete error certificates: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *certRepo) DeleteErrorsByDomain(domainID int64) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM certificates WHERE domain_id = ? AND status = 'error'`, domainID)
	if err != nil {
		return 0, fmt.Errorf("delete error certificates by domain: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *certRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM certificates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete certificate: %w", err)
	}
	return nil
}

func scanCert(s scanner) (*models.Certificate, error) {
	var c models.Certificate
	var notBefore, notAfter, lastChecked, createdAt, updatedAt sql.NullTime
	err := s.Scan(&c.ID, &c.DomainID, &c.Issuer, &c.Subject, &c.Serial,
		&notBefore, &notAfter, &c.Fingerprint, &c.Protocol, &c.Status,
		&lastChecked, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("certificate not found")
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
