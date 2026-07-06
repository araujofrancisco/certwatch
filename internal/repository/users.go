package repository

import (
	"database/sql"
	"fmt"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type userRepo struct {
	db *database.DB
}

func (r *userRepo) Create(u *models.User) error {
	res, err := r.db.Exec(
		`INSERT INTO users (email, password, name) VALUES (?, ?, ?)`,
		u.Email, u.Password, u.Name,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	id, _ := res.LastInsertId()
	u.ID = id
	return nil
}

func (r *userRepo) FindByID(id int64) (*models.User, error) {
	row := r.db.QueryRow(
		`SELECT id, email, password, name, created_at, updated_at FROM users WHERE id = ?`, id,
	)
	return scanUser(row)
}

func (r *userRepo) FindByEmail(email string) (*models.User, error) {
	row := r.db.QueryRow(
		`SELECT id, email, password, name, created_at, updated_at FROM users WHERE email = ?`, email,
	)
	return scanUser(row)
}

func (r *userRepo) List() ([]*models.User, error) {
	rows, err := r.db.Query(
		`SELECT id, email, password, name, created_at, updated_at FROM users ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *userRepo) Update(u *models.User) error {
	_, err := r.db.Exec(
		`UPDATE users SET email = ?, password = ?, name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		u.Email, u.Password, u.Name, u.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func (r *userRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE id = ?`, id)
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
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
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
