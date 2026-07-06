package repository

import (
	"database/sql"
	"fmt"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type notifProfileRepo struct {
	db *database.DB
}

func (r *notifProfileRepo) Create(p *models.NotificationProfile) error {
	res, err := r.db.Exec(
		`INSERT INTO notification_profiles (name, type, enabled, recipients, config) VALUES (?, ?, ?, ?, ?)`,
		p.Name, p.Type, boolToInt(p.Enabled), p.Recipients, p.Config,
	)
	if err != nil {
		return fmt.Errorf("create notification profile: %w", err)
	}
	id, _ := res.LastInsertId()
	p.ID = id
	return nil
}

func (r *notifProfileRepo) FindByID(id int64) (*models.NotificationProfile, error) {
	row := r.db.QueryRow(
		`SELECT id, name, type, enabled, recipients, config, created_at, updated_at FROM notification_profiles WHERE id = ?`, id,
	)
	return scanNotifProfile(row)
}

func (r *notifProfileRepo) List() ([]*models.NotificationProfile, error) {
	rows, err := r.db.Query(
		`SELECT id, name, type, enabled, recipients, config, created_at, updated_at FROM notification_profiles ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list notification profiles: %w", err)
	}
	defer rows.Close()
	var profiles []*models.NotificationProfile
	for rows.Next() {
		p, err := scanNotifProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (r *notifProfileRepo) Update(p *models.NotificationProfile) error {
	_, err := r.db.Exec(
		`UPDATE notification_profiles SET name = ?, type = ?, enabled = ?, recipients = ?, config = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		p.Name, p.Type, boolToInt(p.Enabled), p.Recipients, p.Config, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update notification profile: %w", err)
	}
	return nil
}

func (r *notifProfileRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM notification_profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete notification profile: %w", err)
	}
	return nil
}

func scanNotifProfile(s scanner) (*models.NotificationProfile, error) {
	var p models.NotificationProfile
	var createdAt, updatedAt sql.NullTime
	var enabled int
	err := s.Scan(&p.ID, &p.Name, &p.Type, &enabled, &p.Recipients, &p.Config, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("notification profile not found")
		}
		return nil, fmt.Errorf("scan notification profile: %w", err)
	}
	p.Enabled = enabled == 1
	if createdAt.Valid {
		p.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.Time
	}
	return &p, nil
}
