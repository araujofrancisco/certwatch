package services

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/araujofrancisco/certwatch/internal/auth"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type LoginResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

func (s *AuthService) Register(email, password, name string) (*models.User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("invalid email address")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	hashed, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Email:    email,
		Password: hashed,
		Name:     name,
	}
	if err := s.users.Create(user); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("registration failed")
		}
		return nil, fmt.Errorf("registration failed")
	}

	created, err := s.users.FindByID(user.ID)
	if err != nil {
		return nil, fmt.Errorf("registration failed")
	}
	return created, nil
}

func (s *AuthService) ChangePassword(userID int64, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	user, err := s.users.FindByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	if err := auth.CheckPassword(user.Password, currentPassword); err != nil {
		return fmt.Errorf("current password is incorrect")
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	user.Password = hash
	return s.users.Update(user)
}

func (s *AuthService) Login(email, password string) (*LoginResponse, error) {
	user, err := s.users.FindByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	if err := auth.CheckPassword(user.Password, password); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	token, err := s.auth.GenerateToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{Token: token, User: user}, nil
}
