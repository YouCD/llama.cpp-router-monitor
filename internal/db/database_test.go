package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"llama_proxy/internal/config"
)

func TestParsePostgresKVURL(t *testing.T) {
	kv, err := parsePostgresKV("postgres://user:pass@localhost:5432/proxy?sslmode=disable")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if kv["user"] != "user" {
		t.Fatalf("user=%q", kv["user"])
	}
	if kv["dbname"] != "proxy" {
		t.Fatalf("dbname=%q", kv["dbname"])
	}
	if kv["sslmode"] != "disable" {
		t.Fatalf("sslmode=%q", kv["sslmode"])
	}
}

func TestParsePostgresKVKeyValue(t *testing.T) {
	kv, err := parsePostgresKV("host=localhost port=5432 user=u password='p w' dbname=my_db sslmode=disable")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if kv["host"] != "localhost" {
		t.Fatalf("host=%q", kv["host"])
	}
	if kv["password"] != "p w" {
		t.Fatalf("password=%q", kv["password"])
	}
	if kv["dbname"] != "my_db" {
		t.Fatalf("dbname=%q", kv["dbname"])
	}
}

func TestPgDatabaseName(t *testing.T) {
	cases := []struct {
		dsn  string
		want string
	}{
		{"postgres://user:pass@localhost:5432/proxy?sslmode=disable", "proxy"},
		{"host=localhost dbname=appdb user=u", "appdb"},
		{"host=localhost database=legacydb user=u", "legacydb"},
		{"host=localhost user=u", ""},
	}
	for _, c := range cases {
		got, err := pgDatabaseName(c.dsn)
		if err != nil {
			t.Fatalf("pgDatabaseName(%q): %v", c.dsn, err)
		}
		if got != c.want {
			t.Fatalf("pgDatabaseName(%q)=%q want %q", c.dsn, got, c.want)
		}
	}
}

func TestPgAdminDSN(t *testing.T) {
	dsn, err := pgAdminDSN("postgres://user:pass@localhost:5432/proxy?sslmode=disable")
	if err != nil {
		t.Fatalf("pgAdminDSN: %v", err)
	}
	kv, err := parsePostgresKV(dsn)
	if err != nil {
		t.Fatalf("parse admin dsn: %v", err)
	}
	if kv["dbname"] != "postgres" {
		t.Fatalf("admin dbname=%q want postgres", kv["dbname"])
	}
	if kv["user"] != "user" {
		t.Fatalf("admin user=%q", kv["user"])
	}
	if kv["sslmode"] != "disable" {
		t.Fatalf("admin sslmode=%q", kv["sslmode"])
	}
}

func TestIsMissingDatabaseError(t *testing.T) {
	pqErr := &pgconn.PgError{Code: "3D000", Message: `database "nope" does not exist`}
	if !isMissingDatabaseError(pqErr) {
		t.Fatal("expected 3D000 to be recognized as missing database")
	}

	if isMissingDatabaseError(errors.New("connection refused")) {
		t.Fatal("connection refused should not be missing database")
	}

	if !isMissingDatabaseError(errors.New(`pq: database "foo" does not exist`)) {
		t.Fatal("expected text match for missing database")
	}
}

func TestPgAdminDSNQuotesSpecialValue(t *testing.T) {
	dsn, err := pgAdminDSN("host=localhost user=u password='p w' dbname=proxy sslmode=disable")
	if err != nil {
		t.Fatalf("pgAdminDSN: %v", err)
	}
	if !strings.Contains(dsn, "password='p w'") {
		t.Fatalf("admin dsn should quote password with space: %q", dsn)
	}
}

func TestDatabaseTypeDetection(t *testing.T) {
	dataDir := t.TempDir()
	sqlCfg := config.DatabaseConfig{Type: "sqlite", SQLite: config.SQLiteConfig{Path: "proxy.db"}}
	database, err := NewDatabase(sqlCfg, dataDir, "debug")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer CloseDatabase(database)

	if database.Dialector.Name() != "sqlite" {
		t.Fatalf("sqlite type=%q", database.Dialector.Name())
	}
	if IsPostgresDB(database) {
		t.Fatalf("sqlite reported as postgres")
	}
}

func TestInitPostgreSQLDBSQL(t *testing.T) {
	// Verify PostgreSQL DDL uses native types and no SQLite-only syntax.
	all := strings.Join(pgDDLStatements, "\n")
	for _, want := range []string{"TIMESTAMPTZ", "BOOLEAN", "BIGINT", "SERIAL", "DOUBLE PRECISION"} {
		if !strings.Contains(all, want) {
			t.Fatalf("pg DDL missing %q", want)
		}
	}
	if strings.Contains(all, "AUTOINCREMENT") {
		t.Fatalf("pg DDL must not use AUTOINCREMENT")
	}
}
