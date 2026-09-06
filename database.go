package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type Database interface {
	Close() error
	Ping() error
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Begin() (*sql.Tx, error)
	GetType() string
	Rebind(query string) string
}

type SQLiteDatabase struct {
	db *sql.DB
}

type PostgreSQLDatabase struct {
	db *sql.DB
}

func NewDatabase(cfg DatabaseConfig, dataDir string) (Database, error) {
	switch strings.ToLower(cfg.Type) {
	case "postgresql", "postgres", "pg":
		return newPostgreSQLDatabase(cfg.PostgreSQL)
	case "sqlite", "":
		return newSQLiteDatabase(cfg.SQLite, dataDir)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

func newSQLiteDatabase(cfg SQLiteConfig, dataDir string) (*SQLiteDatabase, error) {
	dbPath := cfg.Path
	if !strings.HasPrefix(dbPath, "/") {
		dbPath = dataDir + "/" + dbPath
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return &SQLiteDatabase{db: db}, nil
}

func newPostgreSQLDatabase(cfg PostgreSQLConfig) (*PostgreSQLDatabase, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgresql DSN is required")
	}

	if err := ensurePostgresDatabase(cfg.DSN); err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open postgresql: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgresql: %w", err)
	}

	return &PostgreSQLDatabase{db: db}, nil
}

// ensurePostgresDatabase connects to the target database and, if it does not
// exist, creates it automatically by connecting to the maintenance database
// ("postgres") first.
func ensurePostgresDatabase(dsn string) error {
	probe, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open postgresql: %w", err)
	}
	probeErr := probe.Ping()
	_ = probe.Close()
	if probeErr == nil {
		return nil
	}
	if !isMissingDatabaseError(probeErr) {
		return fmt.Errorf("connect postgresql: %w", probeErr)
	}

	dbname, err := pgDatabaseName(dsn)
	if err != nil {
		return fmt.Errorf("parse postgresql dsn: %w", err)
	}
	if dbname == "" {
		return fmt.Errorf("cannot determine database name from DSN (missing dbname)")
	}

	adminDSN, err := pgAdminDSN(dsn)
	if err != nil {
		return fmt.Errorf("build admin postgresql dsn: %w", err)
	}
	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		return fmt.Errorf("open postgresql admin connection: %w", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		return fmt.Errorf("cannot connect to postgres maintenance database for auto-create: %w", err)
	}

	quoted := `"` + strings.ReplaceAll(dbname, `"`, `""`) + `"`
	if _, err := admin.Exec(`CREATE DATABASE ` + quoted); err != nil {
		return fmt.Errorf("auto-create database %s: %w", quoted, err)
	}
	return nil
}

// pgDatabaseName extracts the dbname/database parameter from a lib/pq DSN
// (URL or key=value form).
func pgDatabaseName(dsn string) (string, error) {
	kv, err := parsePostgresKV(dsn)
	if err != nil {
		return "", err
	}
	if kv["dbname"] != "" {
		return kv["dbname"], nil
	}
	return kv["database"], nil
}

// pgAdminDSN returns a DSN pointed at the "postgres" maintenance database,
// preserving the user, host and options from the original DSN.
func pgAdminDSN(dsn string) (string, error) {
	kv, err := parsePostgresKV(dsn)
	if err != nil {
		return "", err
	}
	delete(kv, "dbname")
	delete(kv, "database")
	kv["dbname"] = "postgres"

	parts := make([]string, 0, len(kv))
	for k, v := range kv {
		if strings.ContainsAny(v, " '\\") {
			v = "'" + strings.ReplaceAll(v, "'", "''") + "'"
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " "), nil
}

// parsePostgresKV parses a lib/pq DSN into a key/value map. Both URL form
// (postgres://...) and key=value form are accepted.
func parsePostgresKV(dsn string) (map[string]string, error) {
	if strings.Contains(dsn, "://") {
		parsed, err := pq.ParseURL(dsn)
		if err != nil {
			return nil, err
		}
		dsn = parsed
	}

	kv := make(map[string]string)
	for _, field := range splitDSNFields(dsn) {
		idx := strings.IndexByte(field, '=')
		if idx < 0 {
			return nil, fmt.Errorf("invalid dsn field %q", field)
		}
		key := strings.TrimSpace(field[:idx])
		val := strings.TrimSpace(field[idx+1:])
		if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
			val = val[1 : len(val)-1]
			val = strings.ReplaceAll(val, "''", "'")
		}
		kv[key] = val
	}
	return kv, nil
}

// splitDSNFields splits a key=value DSN on spaces, honouring single-quoted values.
func splitDSNFields(dsn string) []string {
	var fields []string
	var cur strings.Builder
	inQuote := false
	for _, r := range dsn {
		switch {
		case r == '\'':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	return fields
}

// isMissingDatabaseError reports whether err indicates the target PostgreSQL
// database does not exist (SQLSTATE 3D000 / invalid_catalog_name).
func isMissingDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "3D000" || pqErr.Code == "42P01"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "does not exist.")
}

func (d *SQLiteDatabase) Close() error {
	return d.db.Close()
}

func (d *SQLiteDatabase) Ping() error {
	return d.db.Ping()
}

func (d *SQLiteDatabase) Exec(query string, args ...any) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *SQLiteDatabase) Query(query string, args ...any) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

func (d *SQLiteDatabase) QueryRow(query string, args ...any) *sql.Row {
	return d.db.QueryRow(query, args...)
}

func (d *SQLiteDatabase) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

func (d *SQLiteDatabase) GetType() string {
	return "sqlite"
}

func (d *SQLiteDatabase) Rebind(query string) string {
	return query
}

// rebindPostgres converts ? placeholders to $1, $2, ... style used by PostgreSQL.
func rebindPostgres(query string) string {
	var b strings.Builder
	n := 0
	inQuote := false
	var quoteChar byte
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case inQuote:
			b.WriteByte(c)
			if c == quoteChar {
				// handle doubled quotes ('' or "")
				if i+1 < len(query) && query[i+1] == quoteChar {
					b.WriteByte(query[i+1])
					i++
				} else {
					inQuote = false
				}
			}
		case c == '\'' || c == '"':
			inQuote = true
			quoteChar = c
			b.WriteByte(c)
		case c == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			// line comment
			for i < len(query) && query[i] != '\n' {
				b.WriteByte(query[i])
				i++
			}
			if i < len(query) {
				b.WriteByte(query[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func (d *PostgreSQLDatabase) Close() error {
	return d.db.Close()
}

func (d *PostgreSQLDatabase) Ping() error {
	return d.db.Ping()
}

func (d *PostgreSQLDatabase) Exec(query string, args ...any) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *PostgreSQLDatabase) Query(query string, args ...any) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

func (d *PostgreSQLDatabase) QueryRow(query string, args ...any) *sql.Row {
	return d.db.QueryRow(query, args...)
}

func (d *PostgreSQLDatabase) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

func (d *PostgreSQLDatabase) GetType() string {
	return "postgresql"
}

func (d *PostgreSQLDatabase) Rebind(query string) string {
	return rebindPostgres(query)
}

func initSQLiteDB(db Database) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA synchronous = NORMAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS requests (
			id TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			query TEXT,
			client_ip TEXT,
			backend_url TEXT,
			model TEXT,
			is_streaming INTEGER NOT NULL DEFAULT 0,
			status_code INTEGER NOT NULL DEFAULT 0,
			error_text TEXT,
			request_bytes INTEGER NOT NULL DEFAULT 0,
			response_bytes INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			cached_prompt_tokens INTEGER NOT NULL DEFAULT 0,
			cache_hit_pct REAL NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			prompt_ms REAL NOT NULL DEFAULT 0,
			completion_ms REAL NOT NULL DEFAULT 0,
			total_ms REAL NOT NULL DEFAULT 0,
			first_byte_ms REAL NOT NULL DEFAULT 0,
			chunks_count INTEGER NOT NULL DEFAULT 0,
			request_raw_path TEXT,
			response_raw_path TEXT,
			user_agent TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_requests_created_at ON requests(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_requests_path ON requests(path);`,
		`CREATE INDEX IF NOT EXISTS idx_requests_status_code ON requests(status_code);`,
		`CREATE TABLE IF NOT EXISTS backend_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME NOT NULL,
			backend_url TEXT NOT NULL,
			metric_name TEXT NOT NULL,
			metric_value REAL NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_backend_metrics_created_at ON backend_metrics(created_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func initPostgreSQLDB(db Database) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS requests (
			id TEXT PRIMARY KEY,
			created_at TIMESTAMP NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			query TEXT,
			client_ip TEXT,
			backend_url TEXT,
			model TEXT,
			is_streaming BOOLEAN NOT NULL DEFAULT FALSE,
			status_code INTEGER NOT NULL DEFAULT 0,
			error_text TEXT,
			request_bytes BIGINT NOT NULL DEFAULT 0,
			response_bytes BIGINT NOT NULL DEFAULT 0,
			prompt_tokens BIGINT NOT NULL DEFAULT 0,
			cached_prompt_tokens BIGINT NOT NULL DEFAULT 0,
			cache_hit_pct DOUBLE PRECISION NOT NULL DEFAULT 0,
			completion_tokens BIGINT NOT NULL DEFAULT 0,
			total_tokens BIGINT NOT NULL DEFAULT 0,
			prompt_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
			completion_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
			first_byte_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
			chunks_count BIGINT NOT NULL DEFAULT 0,
			request_raw_path TEXT,
			response_raw_path TEXT,
			user_agent TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_requests_created_at ON requests(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_requests_path ON requests(path);`,
		`CREATE INDEX IF NOT EXISTS idx_requests_status_code ON requests(status_code);`,
		`CREATE TABLE IF NOT EXISTS backend_metrics (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMP NOT NULL,
			backend_url TEXT NOT NULL,
			metric_name TEXT NOT NULL,
			metric_value DOUBLE PRECISION NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_backend_metrics_created_at ON backend_metrics(created_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func InitDB(db Database, dbType string) error {
	switch strings.ToLower(dbType) {
	case "postgresql", "postgres", "pg":
		return initPostgreSQLDB(db)
	default:
		return initSQLiteDB(db)
	}
}
