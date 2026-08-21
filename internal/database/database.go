package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

	if driver == "sqlite" {
		dsn = withSQLitePragmas(dsn)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if driver == "sqlite" {
		// PRAGMAs are applied via the DSN (withSQLitePragmas) so every pooled
		// connection gets them: foreign_keys and busy_timeout are
		// per-connection settings and would be silently lost once
		// database/sql recycles the first connection.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(time.Hour)
	}

	slog.Info("database connected", "driver", driver)

	return &DB{DB: db, driver: driver, dsn: dsn}, nil
}

// withSQLitePragmas appends per-connection PRAGMA parameters to a SQLite DSN,
// preserving any user-supplied query parameters.
func withSQLitePragmas(dsn string) string {
	pragmas := []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=foreign_keys(1)",
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + strings.Join(pragmas, "&")
}

func (db *DB) Close() error {
	slog.Info("closing database")
	return db.DB.Close()
}

func (db *DB) Migrate() error {
	slog.Info("running database migrations")

	if err := db.ensureSchemaVersionTable(); err != nil {
		return err
	}

	current, err := db.currentSchemaVersion()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := db.applyMigration(m); err != nil {
			return err
		}
		slog.Info("applied migration", "version", m.version, "name", m.name)
		current = m.version
	}

	// Deduplicate certificate rows that the pre-normalization scanner wrote
	// (the same issuance reported by CT providers with different issuer DN
	// attribute orders). Safe to run on every start: after the first pass no
	// duplicate keys remain, so subsequent runs delete nothing.
	if err := db.deduplicateCertificates(); err != nil {
		return err
	}

	slog.Info("database migrations complete", "version", current)
	return nil
}

type migration struct {
	version int
	name    string
	stmts   []string
	// fn is an optional custom step (e.g. guarded ALTER TABLE) executed in
	// the same transaction as stmts.
	fn func(tx *sql.Tx) error
}

// migrations is the ordered schema history. Every statement must be safe to
// re-run against databases created before version tracking existed (they all
// start from version 0): CREATE TABLE IF NOT EXISTS, DROP TABLE IF EXISTS,
// CREATE INDEX IF NOT EXISTS, or the guarded addColumnIfMissing helper.
var migrations = []migration{
	{
		version: 1,
		name:    "core-tables",
		stmts: []string{
			createUsersTable,
			createDomainsTable,
			createCertificatesTable,
			createTagsTable,
			createDomainTagsTable,
		},
	},
	{
		version: 2,
		name:    "drop-notification-profiles",
		stmts: []string{
			// Removed with the unused NotificationProfile repository; drops
			// the orphaned table from databases created by older versions.
			`DROP TABLE IF EXISTS notification_profiles;`,
		},
	},
	{
		version: 3,
		name:    "certificate-sans",
		fn: func(tx *sql.Tx) error {
			return addColumnIfMissingTx(tx, "certificates", "sans", "TEXT NOT NULL DEFAULT ''")
		},
	},
	{
		version: 4,
		name:    "notification-dedup",
		stmts:   []string{createNotificationDedupTable},
	},
	{
		version: 5,
		name:    "indexes",
		stmts: []string{
			`CREATE INDEX IF NOT EXISTS idx_certificates_domain_id ON certificates(domain_id);`,
			`CREATE INDEX IF NOT EXISTS idx_certificates_not_after ON certificates(not_after);`,
			`CREATE INDEX IF NOT EXISTS idx_certificates_status ON certificates(status);`,
			`CREATE INDEX IF NOT EXISTS idx_domains_enabled ON domains(enabled);`,
			`CREATE INDEX IF NOT EXISTS idx_domains_group_name ON domains(group_name);`,
			`CREATE INDEX IF NOT EXISTS idx_domain_tags_tag_id ON domain_tags(tag_id);`,
		},
	},
	{
		version: 6,
		name:    "normalize-zero-timestamps",
		stmts: []string{
			// Older builds serialized Go zero times as literal strings
			// instead of NULL, breaking expiry range comparisons.
			`UPDATE certificates SET not_after = NULL WHERE not_after LIKE '0001-01-01%';`,
			`UPDATE certificates SET not_before = NULL WHERE not_before LIKE '0001-01-01%';`,
			`UPDATE certificates SET last_checked = NULL WHERE last_checked LIKE '0001-01-01%';`,
		},
	},
}

const createSchemaVersionTable = `
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

func (db *DB) ensureSchemaVersionTable() error {
	if _, err := db.Exec(createSchemaVersionTable); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}
	return nil
}

func (db *DB) currentSchemaVersion() (int, error) {
	row := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	var version int
	if err := row.Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// applyMigration runs a migration's statements inside a transaction together
// with its version record, so a failed migration leaves no partial state.
func (db *DB) applyMigration(m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("migration %d (%s): begin: %w", m.version, m.name, err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, stmt := range m.stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migration %d (%s) stmt %d: %w", m.version, m.name, i+1, err)
		}
	}
	if m.fn != nil {
		if err := m.fn(tx); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version); err != nil {
		return fmt.Errorf("migration %d (%s): record version: %w", m.version, m.name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %d (%s): commit: %w", m.version, m.name, err)
	}
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

// addColumnIfMissingTx adds a column to a table when it does not already
// exist. SQLite does not support "ADD COLUMN IF NOT EXISTS", so the operation
// is guarded by a pragma lookup. Safe to re-run.
func addColumnIfMissingTx(tx *sql.Tx, table, column, definition string) error {
	row := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if count > 0 {
		return nil
	}
	if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
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

const createNotificationDedupTable = `
CREATE TABLE IF NOT EXISTS notification_dedup (
    key         TEXT PRIMARY KEY,
    notified_at DATETIME NOT NULL
);`
