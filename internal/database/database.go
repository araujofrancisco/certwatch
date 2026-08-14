package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
	driver string
	dsn    string
}

func Open(driver, dsn string) (*DB, error) {
	slog.Debug("opening database", "driver", driver, "dsn", dsn)

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if driver == "sqlite" {
		pragmas := []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=5000",
			"PRAGMA foreign_keys = ON",
		}
		for _, p := range pragmas {
			if _, err := db.Exec(p); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(time.Hour)
	}

	slog.Info("database connected", "driver", driver)

	return &DB{DB: db, driver: driver, dsn: dsn}, nil
}

func (db *DB) Close() error {
	slog.Info("closing database")
	return db.DB.Close()
}

func (db *DB) Migrate() error {
	slog.Info("running database migrations")

	migrations := []string{
		createUsersTable,
		createDomainsTable,
		createCertificatesTable,
		createNotificationProfilesTable,
		createTagsTable,
		createDomainTagsTable,
	}

	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}

	// Idempotent additive migrations for existing databases created before a
	// column was introduced (CREATE TABLE IF NOT EXISTS does not alter them).
	if err := addColumnIfMissing(db, "certificates", "sans", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	slog.Info("database migrations complete")
	return nil
}

// addColumnIfMissing adds a column to a table when it does not already exist.
// SQLite does not support "ADD COLUMN IF NOT EXISTS", so the operation is
// attempted and the duplicate-column error is ignored.
func addColumnIfMissing(db *DB, table, column, definition string) error {
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if count > 0 {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

func EnsureDir(driver, dsn string) error {
	if driver != "sqlite" {
		return nil
	}
	dir := filepath.Dir(dsn)
	if dir != "." && dir != "" {
		return os.MkdirAll(dir, 0700)
	}
	return nil
}

const createUsersTable = `
CREATE TABLE IF NOT EXISTS users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    email       TEXT    NOT NULL UNIQUE,
    password    TEXT    NOT NULL,
    name        TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createDomainsTable = `
CREATE TABLE IF NOT EXISTS domains (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    domain       TEXT    NOT NULL UNIQUE,
    description  TEXT    NOT NULL DEFAULT '',
    enabled      INTEGER NOT NULL DEFAULT 1,
    group_name   TEXT    NOT NULL DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createCertificatesTable = `
CREATE TABLE IF NOT EXISTS certificates (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id     INTEGER NOT NULL REFERENCES domains(id),
    issuer        TEXT    NOT NULL DEFAULT '',
    subject       TEXT    NOT NULL DEFAULT '',
    serial        TEXT    NOT NULL DEFAULT '',
    not_before    DATETIME,
    not_after     DATETIME,
    fingerprint   TEXT    NOT NULL DEFAULT '',
    protocol      TEXT    NOT NULL DEFAULT 'https',
    status        TEXT    NOT NULL DEFAULT 'unknown',
    sans          TEXT    NOT NULL DEFAULT '',
    last_checked  DATETIME,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createTagsTable = `
CREATE TABLE IF NOT EXISTS tags (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    color       TEXT    NOT NULL DEFAULT '#6c757d',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createDomainTagsTable = `
CREATE TABLE IF NOT EXISTS domain_tags (
    domain_id   INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (domain_id, tag_id)
);`

const createNotificationProfilesTable = `
CREATE TABLE IF NOT EXISTS notification_profiles (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    type        TEXT    NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    recipients  TEXT    NOT NULL DEFAULT '',
    config      TEXT    NOT NULL DEFAULT '{}',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);`
