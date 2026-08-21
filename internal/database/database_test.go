package database

import (
	"database/sql"
	"os"
	"testing"
)

func openMemory(t *testing.T) *DB {
	t.Helper()
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open(\"sqlite\", \":memory:\"): %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_Close(t *testing.T) {
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Error("expected ping to succeed")
	}
	if err := db.Close(); err != nil {
		t.Error(err)
	}
}

func TestOpen_BadDriver(t *testing.T) {
	_, err := Open("nonexistent", ":memory:")
	if err == nil {
		t.Error("expected error for unknown driver")
	}
}

func TestOpen_InvalidDSN(t *testing.T) {
	_, err := Open("sqlite", "/nonexistent/dir/db.sqlite")
	if err == nil {
		t.Error("expected error for invalid DSN path")
	}
}

func TestMigrate_CreatesTables(t *testing.T) {
	db := openMemory(t)

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}

	expected := []string{"certificates", "domains", "users"}
	for _, want := range expected {
		found := false
		for _, got := range tables {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing table %q; got %v", want, tables)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := openMemory(t)

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal("second migrate should succeed:", err)
	}
}

func TestMigrate_UsersColumns(t *testing.T) {
	db := openMemory(t)
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`PRAGMA table_info(users)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}

	for _, want := range []string{"id", "email", "password", "name", "created_at", "updated_at"} {
		if !cols[want] {
			t.Errorf("missing column %q in users table", want)
		}
	}
}

func TestMigrate_CertificatesColumns(t *testing.T) {
	db := openMemory(t)
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`PRAGMA table_info(certificates)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}

	for _, want := range []string{"id", "domain_id", "issuer", "subject", "serial",
		"not_before", "not_after", "fingerprint", "protocol", "status", "last_checked",
		"created_at", "updated_at"} {
		if !cols[want] {
			t.Errorf("missing column %q in certificates table", want)
		}
	}
}

func TestMigrate_DomainsColumns(t *testing.T) {
	db := openMemory(t)
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`PRAGMA table_info(domains)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}

	for _, want := range []string{"id", "domain", "description", "enabled", "created_at", "updated_at"} {
		if !cols[want] {
			t.Errorf("missing column %q in domains table", want)
		}
	}
}

func TestEnsureDir_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	dsn := dir + "/sub/certwatch.db"

	if err := EnsureDir("sqlite", dsn); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir + "/sub"); os.IsNotExist(err) {
		t.Error("expected directory to exist")
	}
}

func TestEnsureDir_NoopForNonSQLite(t *testing.T) {
	if err := EnsureDir("postgres", "host=localhost dbname=test"); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureDir_NoopForCurrentDir(t *testing.T) {
	if err := EnsureDir("sqlite", "certwatch.db"); err != nil {
		t.Fatal(err)
	}
}

func TestMigrate_DeduplicatesCertificates(t *testing.T) {
	db := openMemory(t)
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	// Seed two domains so the dedup key is scoped per domain.
	if _, err := db.Exec(`INSERT INTO domains (domain) VALUES ('example.com'), ('other.com')`); err != nil {
		t.Fatal(err)
	}

	insert := func(domainID int64, serial, issuer string) int64 {
		t.Helper()
		res, err := db.Exec(`INSERT INTO certificates (domain_id, issuer, serial) VALUES (?, ?, ?)`, domainID, issuer, serial)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	// Same issuance twice with different issuer DN attribute orders → duplicate.
	insert(1, "030150c1d6a9ce829bac11d55e0f097d", "CN=RapidSSL TLS RSA CA G1,OU=www.digicert.com,O=DigiCert Inc,C=US")
	insert(1, "030150c1d6a9ce829bac11d55e0f097d", "C=US, O=DigiCert Inc, OU=www.digicert.com, CN=RapidSSL TLS RSA CA G1")
	// Colon-formatted serial of the same value → still a duplicate.
	insert(1, "03:01:50:c1:d6:a9:ce:82:9b:ac:11:d5:5e:0f:09:7d", "CN=RapidSSL TLS RSA CA G1,OU=www.digicert.com,O=DigiCert Inc,C=US")
	// Distinct issuance (different serial) → kept.
	insert(1, "0845894d10139b36f88005ed7e8a06d5", "C=US, O=DigiCert Inc, OU=www.digicert.com, CN=RapidSSL TLS RSA CA G1")
	// Same serial on a different domain → not a duplicate.
	insert(2, "030150c1d6a9ce829bac11d55e0f097d", "CN=RapidSSL TLS RSA CA G1,OU=www.digicert.com,O=DigiCert Inc,C=US")

	if err := db.deduplicateCertificates(); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM certificates`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 certificates after dedup, got %d", count)
	}

	// Running again must be a no-op (idempotent).
	if err := db.deduplicateCertificates(); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM certificates`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 certificates after second dedup, got %d", count)
	}
}

func TestMigrate_KeepsCertificateWithEmptySerial(t *testing.T) {
	db := openMemory(t)
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	// A row with an empty serial must never be deleted by the dedup pass.
	if _, err := db.Exec(`INSERT INTO domains (domain) VALUES ('example.com')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO certificates (domain_id, issuer, serial) VALUES (1, 'CN=CA,O=Org', '')`); err != nil {
		t.Fatal(err)
	}
	if err := db.deduplicateCertificates(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM certificates`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 certificate (empty serial kept), got %d", count)
	}
}
