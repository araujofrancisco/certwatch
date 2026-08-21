package repository

import (
	"context"

	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type domainRepo struct {
	db *database.DB
}

func (r *domainRepo) Create(ctx context.Context, d *models.Domain) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO domains (domain, description, enabled, group_name) VALUES (?, ?, ?, ?)`,
		d.Domain, d.Description, boolToInt(d.Enabled), d.Group,
	)
	if err != nil {
		return wrapConstraint("create domain", err)
	}
	id, _ := res.LastInsertId()
	d.ID = id
	return nil
}

// CreateMany inserts every domain inside a single transaction: either the
// whole batch commits or nothing does. On failure the slice may be partially
// assigned IDs; callers must treat it as unchanged.
func (r *domainRepo) CreateMany(ctx context.Context, domains []*models.Domain) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("bulk create domains: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, d := range domains {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO domains (domain, description, enabled, group_name) VALUES (?, ?, ?, ?)`,
			d.Domain, d.Description, boolToInt(d.Enabled), d.Group,
		)
		if err != nil {
			return wrapConstraint(fmt.Sprintf("bulk create domain %s", d.Domain), err)
		}
		id, _ := res.LastInsertId()
		d.ID = id
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("bulk create domains: commit: %w", err)
	}
	return nil
}

func (r *domainRepo) FindByID(ctx context.Context, id int64) (*models.Domain, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains WHERE id = ?`, id,
	)
	return scanDomain(row)
}

func (r *domainRepo) FindByDomain(ctx context.Context, domain string) (*models.Domain, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains WHERE domain = ?`, domain,
	)
	return scanDomain(row)
}

func (r *domainRepo) ListFiltered(ctx context.Context, filter models.DomainFilter) ([]*models.Domain, error) {
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
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list filtered domains: %w", err)
	}
	defer rows.Close()
	return scanDomains(rows)
}

func (r *domainRepo) CountFiltered(ctx context.Context, filter models.DomainFilter) (int, error) {
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

	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM domains`+where, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count filtered domains: %w", err)
	}
	return count, nil
}

func (r *domainRepo) List(ctx context.Context) ([]*models.Domain, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains ORDER BY domain`,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()
	return scanDomains(rows)
}

func (r *domainRepo) ListEnabled(ctx context.Context) ([]*models.Domain, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, domain, description, enabled, group_name, created_at, updated_at FROM domains WHERE enabled = 1 ORDER BY domain`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled domains: %w", err)
	}
	defer rows.Close()
	return scanDomains(rows)
}

func (r *domainRepo) Update(ctx context.Context, d *models.Domain) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE domains SET domain = ?, description = ?, enabled = ?, group_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		d.Domain, d.Description, boolToInt(d.Enabled), d.Group, d.ID,
	)
	if err != nil {
		return wrapConstraint("update domain", err)
	}
	return nil
}

func (r *domainRepo) Delete(ctx context.Context, id int64) error {
	// Transactional so a partial failure cannot leave orphaned certificates
	// or tags behind. domain_tags is deleted explicitly as well: FK cascade
	// coverage must not be assumed.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete domain: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM certificates WHERE domain_id = ?`, id); err != nil {
		return fmt.Errorf("delete domain certificates: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM domain_tags WHERE domain_id = ?`, id); err != nil {
		return fmt.Errorf("delete domain tags: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM domains WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete domain rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("domain %w", ErrNotFound)
	}
	return tx.Commit()
}

func scanDomain(s scanner) (*models.Domain, error) {
	var d models.Domain
	var createdAt, updatedAt sql.NullTime
	var enabled int
	err := s.Scan(&d.ID, &d.Domain, &d.Description, &enabled, &d.Group, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("domain: %w", ErrNotFound)
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
