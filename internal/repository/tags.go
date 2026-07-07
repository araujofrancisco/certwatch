package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type tagRepo struct {
	db *database.DB
}

func (r *tagRepo) Create(name, color string) (*models.Tag, error) {
	res, err := r.db.Exec(
		`INSERT INTO tags (name, color) VALUES (?, ?)`,
		strings.ToLower(strings.TrimSpace(name)), color,
	)
	if err != nil {
		return nil, fmt.Errorf("create tag: %w", err)
	}
	id, _ := res.LastInsertId()
	return &models.Tag{ID: id, Name: name, Color: color}, nil
}

func (r *tagRepo) FindByID(id int64) (*models.Tag, error) {
	row := r.db.QueryRow(`SELECT id, name, color, created_at FROM tags WHERE id = ?`, id)
	return scanTag(row)
}

func (r *tagRepo) FindByName(name string) (*models.Tag, error) {
	row := r.db.QueryRow(`SELECT id, name, color, created_at FROM tags WHERE name = ?`,
		strings.ToLower(strings.TrimSpace(name)))
	return scanTag(row)
}

func (r *tagRepo) List() ([]*models.Tag, error) {
	rows, err := r.db.Query(`SELECT id, name, color, created_at FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	return scanTags(rows)
}

func (r *tagRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM domain_tags WHERE tag_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete domain_tags for tag: %w", err)
	}
	_, err = r.db.Exec(`DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	return nil
}

func (r *tagRepo) SetDomainTags(domainID int64, tagIDs []int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			fmt.Printf("tx rollback error: %v\n", err)
		}
	}()

	if _, err := tx.Exec(`DELETE FROM domain_tags WHERE domain_id = ?`, domainID); err != nil {
		return fmt.Errorf("delete domain_tags: %w", err)
	}

	for _, tid := range tagIDs {
		if _, err := tx.Exec(`INSERT INTO domain_tags (domain_id, tag_id) VALUES (?, ?)`, domainID, tid); err != nil {
			return fmt.Errorf("insert domain_tag: %w", err)
		}
	}

	return tx.Commit()
}

func (r *tagRepo) GetDomainTags(domainID int64) ([]*models.Tag, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.name, t.color, t.created_at
		FROM tags t
		JOIN domain_tags dt ON dt.tag_id = t.id
		WHERE dt.domain_id = ?
		ORDER BY t.name`, domainID)
	if err != nil {
		return nil, fmt.Errorf("get domain tags: %w", err)
	}
	defer rows.Close()
	return scanTags(rows)
}

func (r *tagRepo) ListByTagNames(names []string) ([]int64, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(names))
	args := make([]any, len(names))
	for i, n := range names {
		placeholders[i] = "?"
		args[i] = strings.ToLower(strings.TrimSpace(n))
	}

	query := fmt.Sprintf(`
		SELECT dt.domain_id
		FROM domain_tags dt
		JOIN tags t ON t.id = dt.tag_id
		WHERE t.name IN (%s)`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list by tag names: %w", err)
	}
	defer rows.Close()

	var ids []int64
	seen := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (r *tagRepo) GetTagsByDomainIDs(domainIDs []int64) (map[int64][]*models.Tag, error) {
	if len(domainIDs) == 0 {
		return make(map[int64][]*models.Tag), nil
	}
	placeholders := make([]string, len(domainIDs))
	args := make([]any, len(domainIDs))
	for i, id := range domainIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT dt.domain_id, t.id, t.name, t.color, t.created_at
		FROM domain_tags dt
		JOIN tags t ON t.id = dt.tag_id
		WHERE dt.domain_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY dt.domain_id, t.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("get tags by domain ids: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]*models.Tag)
	for rows.Next() {
		var domainID int64
		var tag models.Tag
		var createdAt sql.NullTime
		if err := rows.Scan(&domainID, &tag.ID, &tag.Name, &tag.Color, &createdAt); err != nil {
			return nil, err
		}
		if createdAt.Valid {
			tag.CreatedAt = createdAt.Time
		}
		result[domainID] = append(result[domainID], &tag)
	}
	return result, rows.Err()
}

func scanTag(s scanner) (*models.Tag, error) {
	var t models.Tag
	var createdAt sql.NullTime
	err := s.Scan(&t.ID, &t.Name, &t.Color, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag not found")
		}
		return nil, fmt.Errorf("scan tag: %w", err)
	}
	if createdAt.Valid {
		t.CreatedAt = createdAt.Time
	}
	return &t, nil
}

func scanTags(r *sql.Rows) ([]*models.Tag, error) {
	var tags []*models.Tag
	for r.Next() {
		t, err := scanTag(r)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, r.Err()
}
