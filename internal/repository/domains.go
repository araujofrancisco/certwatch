package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type domainRepo struct {
	db *database.DB
}

func (r *domainRepo) Create(d *models.Domain) error {
	res, err := r.db.Exec(
		`INSERT INTO domains (domain, description, enabled, group_name) VALUES (?, ?, ?, ?)`,
		d.Domain, d.Description, boolToInt(d.Enabled), d.Group,
	)
	if err != nil {
		return fmt.Errorf("create domain: %w", err)
	}
	id, _ := res.LastInsertId()
	d.ID = id
	return nil
}

func (r *domainRepo) FindByID(id int64) (*models.Domain, error) {
	row := r.db.QueryRow(
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains WHERE id = ?`, id,
	)
	return scanDomain(row)
}

func (r *domainRepo) FindByDomain(domain string) (*models.Domain, error) {
	row := r.db.QueryRow(
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains WHERE domain = ?`, domain,
	)
	return scanDomain(row)
}

func (r *domainRepo) ListFiltered(filter models.DomainFilter) ([]*models.Domain, error) {
	var clauses []string
	var args []any

	if filter.Query != "" {
		clauses = append(clauses, "(domain LIKE ? OR description LIKE ?)")
		q := "%" + filter.Query + "%"
		args = append(args, q, q)
	}
	if filter.Enabled != nil {
		clauses = append(clauses, "enabled = ?")
		args = append(args, boolToInt(*filter.Enabled))
	}

	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	query := `SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains` + where + ` ORDER BY domain`
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", filter.Limit, filter.Offset())
	}
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list filtered domains: %w", err)
	}
	defer rows.Close()
	return scanDomains(rows)
}

func (r *domainRepo) CountFiltered(filter models.DomainFilter) (int, error) {
	var clauses []string
	var args []any

	if filter.Query != "" {
		clauses = append(clauses, "(domain LIKE ? OR description LIKE ?)")
		q := "%" + filter.Query + "%"
		args = append(args, q, q)
	}
	if filter.Enabled != nil {
		clauses = append(clauses, "enabled = ?")
		args = append(args, boolToInt(*filter.Enabled))
	}

	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	row := r.db.QueryRow(`SELECT COUNT(*) FROM domains`+where, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count filtered domains: %w", err)
	}
	return count, nil
}

func (r *domainRepo) List() ([]*models.Domain, error) {
	rows, err := r.db.Query(
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains ORDER BY domain`,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()
	return scanDomains(rows)
}

func (r *domainRepo) ListEnabled() ([]*models.Domain, error) {
	rows, err := r.db.Query(
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains WHERE enabled = 1 ORDER BY domain`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled domains: %w", err)
	}
	defer rows.Close()
	return scanDomains(rows)
}

func (r *domainRepo) Update(d *models.Domain) error {
	_, err := r.db.Exec(
		`UPDATE domains SET domain = ?, description = ?, enabled = ?, group_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		d.Domain, d.Description, boolToInt(d.Enabled), d.Group, d.ID,
	)
	if err != nil {
		return fmt.Errorf("update domain: %w", err)
	}
	return nil
}

func (r *domainRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM certificates WHERE domain_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete domain certificates: %w", err)
	}
	res, err := r.db.Exec(`DELETE FROM domains WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete domain rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("domain not found")
	}
	return nil
}

func scanDomain(s scanner) (*models.Domain, error) {
	var d models.Domain
	var createdAt, updatedAt sql.NullTime
	var enabled int
	err := s.Scan(&d.ID, &d.Domain, &d.Description, &enabled, &d.Group, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("domain not found")
		}
		return nil, fmt.Errorf("scan domain: %w", err)
	}
	d.Enabled = enabled == 1
	if createdAt.Valid {
		d.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		d.UpdatedAt = updatedAt.Time
	}
	return &d, nil
}

func scanDomains(r *sql.Rows) ([]*models.Domain, error) {
	var domains []*models.Domain
	for r.Next() {
		d, err := scanDomain(r)
		if err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, r.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
