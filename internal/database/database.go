package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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
	}

	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}

	slog.Info("database migrations complete")
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
    last_checked  DATETIME,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
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
