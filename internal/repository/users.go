package repository

import (
	"context"

	"database/sql"
	"errors"
	"fmt"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type userRepo struct {
	db *database.DB
}

func (r *userRepo) Create(ctx context.Context, u *models.User) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (email, password, name) VALUES (?, ?, ?)`,
		u.Email, u.Password, u.Name,
	)
	if err != nil {
		return wrapConstraint("create user", err)
	}
	id, _ := res.LastInsertId()
	u.ID = id
	return nil
}

func (r *userRepo) FindByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, password, name, created_at, updated_at FROM users WHERE id = ?`, id,
	)
	return scanUser(row)
}

func (r *userRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, password, name, created_at, updated_at FROM users WHERE email = ?`, email,
	)
	return scanUser(row)
}

func (r *userRepo) Update(ctx context.Context, u *models.User) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET email = ?, password = ?, name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		u.Email, u.Password, u.Name, u.ID,
	)
	if err != nil {
		return wrapConstraint("update user", err)
	}
	return nil
}

func (r *userRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (*models.User, error) {
	var u models.User
	var createdAt, updatedAt sql.NullTime
	err := s.Scan(&u.ID, &u.Email, &u.Password, &u.Name, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	if createdAt.Valid {
		u.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		u.UpdatedAt = updatedAt.Time
	}
	return &u, nil
}
