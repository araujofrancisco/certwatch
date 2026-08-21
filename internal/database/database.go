package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/araujofrancisco/certwatch/internal/ctsearch"
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
		createTagsTable,
		createDomainTagsTable,
		// Removed with the unused NotificationProfile repository; drops the
		// orphaned table from databases created by older versions.
		`DROP TABLE IF EXISTS notification_profiles;`,
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

	// Deduplicate certificate rows that the pre-normalization scanner wrote
	// (the same issuance reported by CT providers with different issuer DN
	// attribute orders). Safe to run on every start: after the first pass no
	// duplicate keys remain, so subsequent runs delete nothing.
	if err := db.deduplicateCertificates(); err != nil {
		return err
	}

	slog.Info("database migrations complete")
	return nil
}

// deduplicateCertificates removes duplicate certificate rows for the same
// domain that share the same normalized serial + normalized issuer, keeping
// only the most recently created row. Duplicates arise when CT providers
// format the same issuer DN in different attribute orders (ctlogs.dev emits
// CN-first, CertSpotter emits C-first) so the pre-normalization dedup treated
// one issuance as two rows.
func (db *DB) deduplicateCertificates() error {
	rows, err := db.Query(`SELECT id, domain_id, serial, issuer FROM certificates ORDER BY id`)
	if err != nil {
		return fmt.Errorf("deduplicate certificates: query: %w", err)
	}
	defer rows.Close()

	type certRow struct {
		id       int64
		domainID int64
		serial   string
		issuer   string
	}
	var certs []certRow
	for rows.Next() {
		var r certRow
		if err := rows.Scan(&r.id, &r.domainID, &r.serial, &r.issuer); err != nil {
			return fmt.Errorf("deduplicate certificates: scan: %w", err)
		}
		if r.serial == "" {
			continue
		}
		certs = append(certs, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("deduplicate certificates: rows: %w", err)
	}

	// Key is (domain_id, normalized serial, normalized issuer). Rows arrive
	// ordered by id ascending, so the first row for each key is the keeper.
	firstID := make(map[string]int64)
	var toDelete []int64
	for _, c := range certs {
		key := fmt.Sprintf("%d|%s|%s", c.domainID, ctsearch.NormalizeSerial(c.serial), ctsearch.NormalizeDN(c.issuer))
		if keeper, ok := firstID[key]; ok {
			toDelete = append(toDelete, c.id)
			slog.Info("removing duplicate certificate", "cert_id", c.id, "keeping", keeper, "domain_id", c.domainID)
			continue
		}
		firstID[key] = c.id
	}
	if len(toDelete) == 0 {
		return nil
	}

	for _, id := range toDelete {
		if _, err := db.Exec(`DELETE FROM certificates WHERE id = ?`, id); err != nil {
			return fmt.Errorf("deduplicate certificates: delete %d: %w", id, err)
		}
	}
	slog.Info("removed duplicate certificates", "deleted", len(toDelete))
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
