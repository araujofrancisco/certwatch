package repository

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors let callers distinguish outcomes without string matching.
// Repositories wrap these with context; callers test with errors.Is.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint
// failure from the modernc driver.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "UNIQUE CONSTRAINT FAILED") || strings.Contains(msg, "UNIQUE")
}

// wrapConstraint annotates constraint violations with ErrConflict while
// keeping the driver error in the chain; other errors pass through wrapped
// with context only.
func wrapConstraint(op string, err error) error {
	if isUniqueViolation(err) {
		return errors.Join(ErrConflict, fmt.Errorf("%s: %w", op, err))
	}
	return fmt.Errorf("%s: %w", op, err)
}
