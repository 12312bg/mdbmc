// Package store persists users, devices, credentials, groups, settings and
// the audit log in a local SQLite database.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("记录不存在")

// Store wraps the SQLite connection.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer at a time; a small pool avoids lock churn.
	db.SetMaxOpenConns(4)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

var migrations = []string{
	`
CREATE TABLE users (
	id                   INTEGER PRIMARY KEY,
	username             TEXT NOT NULL UNIQUE COLLATE NOCASE,
	display_name         TEXT NOT NULL DEFAULT '',
	role                 TEXT NOT NULL,
	kdf_salt             BLOB NOT NULL,
	kdf_params           TEXT NOT NULL,
	wrapped_key          BLOB NOT NULL,
	disabled             INTEGER NOT NULL DEFAULT 0,
	must_change_password INTEGER NOT NULL DEFAULT 0,
	created_at           INTEGER NOT NULL,
	updated_at           INTEGER NOT NULL,
	last_login_at        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE device_groups (
	id         INTEGER PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
	created_at INTEGER NOT NULL
);

CREATE TABLE credentials (
	id           INTEGER PRIMARY KEY,
	name         TEXT NOT NULL UNIQUE COLLATE NOCASE,
	username     TEXT NOT NULL,
	password_enc BLOB NOT NULL,
	note         TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	updated_at   INTEGER NOT NULL
);

CREATE TABLE devices (
	id              INTEGER PRIMARY KEY,
	name            TEXT NOT NULL,
	host            TEXT NOT NULL,
	ipmi_port       INTEGER NOT NULL DEFAULT 623,
	https_port      INTEGER NOT NULL DEFAULT 443,
	protocol        TEXT NOT NULL DEFAULT 'auto',
	group_id        INTEGER REFERENCES device_groups(id) ON DELETE SET NULL,
	credential_id   INTEGER REFERENCES credentials(id) ON DELETE RESTRICT,
	username        TEXT NOT NULL DEFAULT '',
	password_enc    BLOB,
	note            TEXT NOT NULL DEFAULT '',
	vendor          TEXT NOT NULL DEFAULT '',
	manufacturer    TEXT NOT NULL DEFAULT '',
	model           TEXT NOT NULL DEFAULT '',
	serial          TEXT NOT NULL DEFAULT '',
	bmc_firmware    TEXT NOT NULL DEFAULT '',
	bios_version    TEXT NOT NULL DEFAULT '',
	has_ipmi        INTEGER NOT NULL DEFAULT 0,
	has_redfish     INTEGER NOT NULL DEFAULT 0,
	tls_fingerprint TEXT NOT NULL DEFAULT '',
	power_state     TEXT NOT NULL DEFAULT 'unknown',
	health          TEXT NOT NULL DEFAULT 'unknown',
	reachable       INTEGER NOT NULL DEFAULT 0,
	last_error      TEXT NOT NULL DEFAULT '',
	checked_at      INTEGER NOT NULL DEFAULT 0,
	identified_at   INTEGER NOT NULL DEFAULT 0,
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL,
	UNIQUE (host, ipmi_port, https_port)
);
CREATE INDEX devices_group ON devices(group_id);

CREATE TABLE inventory (
	device_id  INTEGER PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
	data       TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE audit_log (
	id       INTEGER PRIMARY KEY,
	ts       INTEGER NOT NULL,
	user_id  INTEGER NOT NULL DEFAULT 0,
	username TEXT NOT NULL DEFAULT '',
	action   TEXT NOT NULL,
	target   TEXT NOT NULL DEFAULT '',
	success  INTEGER NOT NULL,
	detail   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX audit_ts ON audit_log(ts);

CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`,
}

func now() int64 { return time.Now().Unix() }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueErr reports whether err is a UNIQUE constraint violation.
func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isForeignKeyErr reports whether err is a FOREIGN KEY constraint violation.
func isForeignKeyErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
