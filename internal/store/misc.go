package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Credential is a reusable BMC username/password profile.
type Credential struct {
	ID          int64
	Name        string
	Username    string
	PasswordEnc []byte
	Note        string
	DeviceCount int
	CreatedAt   int64
	UpdatedAt   int64
}

// ListCredentials returns all credential profiles with usage counts.
func (s *Store) ListCredentials(ctx context.Context) ([]*Credential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, c.username, c.password_enc, c.note, c.created_at, c.updated_at,
		(SELECT COUNT(*) FROM devices d WHERE d.credential_id = c.id)
		FROM credentials c ORDER BY c.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.Name, &c.Username, &c.PasswordEnc, &c.Note, &c.CreatedAt, &c.UpdatedAt, &c.DeviceCount); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// CredentialByID returns one credential profile.
func (s *Store) CredentialByID(ctx context.Context, id int64) (*Credential, error) {
	var c Credential
	err := s.db.QueryRowContext(ctx, `SELECT id, name, username, password_enc, note, created_at, updated_at FROM credentials WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Username, &c.PasswordEnc, &c.Note, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

// CreateCredential inserts c and sets its ID.
func (s *Store) CreateCredential(ctx context.Context, c *Credential) error {
	t := now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO credentials (name, username, password_enc, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, c.Name, c.Username, c.PasswordEnc, c.Note, t, t)
	if isUniqueErr(err) {
		return fmt.Errorf("凭据名称“%s”已存在", c.Name)
	}
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

// UpdateCredential saves a credential profile; keepPassword leaves the stored
// password untouched.
func (s *Store) UpdateCredential(ctx context.Context, c *Credential, keepPassword bool) error {
	q := "UPDATE credentials SET name = ?, username = ?, note = ?, updated_at = ?"
	args := []any{c.Name, c.Username, c.Note, now()}
	if !keepPassword {
		q += ", password_enc = ?"
		args = append(args, c.PasswordEnc)
	}
	q += " WHERE id = ?"
	args = append(args, c.ID)
	err := s.execOne(ctx, q, args...)
	if isUniqueErr(err) {
		return fmt.Errorf("凭据名称“%s”已存在", c.Name)
	}
	return err
}

// DeleteCredential removes a profile that no device uses.
func (s *Store) DeleteCredential(ctx context.Context, id int64) error {
	err := s.execOne(ctx, "DELETE FROM credentials WHERE id = ?", id)
	if isForeignKeyErr(err) {
		return errors.New("仍有设备在使用此凭据，请先为这些设备更换凭据")
	}
	return err
}

// Settings returns all key/value settings.
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SaveSettings upserts key/value settings in one transaction.
func (s *Store) SaveSettings(ctx context.Context, kv map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AuditEntry is one audit log row.
type AuditEntry struct {
	ID       int64
	TS       int64
	UserID   int64
	Username string
	Action   string
	Target   string
	Success  bool
	Detail   string
}

// AddAudit appends an audit entry.
func (s *Store) AddAudit(ctx context.Context, e *AuditEntry) error {
	if e.TS == 0 {
		e.TS = now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_log (ts, user_id, username, action, target, success, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, e.TS, e.UserID, e.Username, e.Action, e.Target, boolInt(e.Success), e.Detail)
	return err
}

// AuditQuery filters the audit log.
type AuditQuery struct {
	Keyword string
	Since   int64
	Until   int64
	Offset  int
	Limit   int
}

// ListAudit returns matching entries (newest first) and the total count.
func (s *Store) ListAudit(ctx context.Context, q AuditQuery) ([]*AuditEntry, int, error) {
	var where []string
	var args []any
	if q.Keyword != "" {
		where = append(where, "(username LIKE ? OR action LIKE ? OR target LIKE ? OR detail LIKE ?)")
		kw := "%" + q.Keyword + "%"
		args = append(args, kw, kw, kw, kw)
	}
	if q.Since > 0 {
		where = append(where, "ts >= ?")
		args = append(args, q.Since)
	}
	if q.Until > 0 {
		where = append(where, "ts <= ?")
		args = append(args, q.Until)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_log"+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, user_id, username, action, target, success, detail FROM audit_log`+
		cond+" ORDER BY id DESC LIMIT ? OFFSET ?", append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ok int
		if err := rows.Scan(&e.ID, &e.TS, &e.UserID, &e.Username, &e.Action, &e.Target, &ok, &e.Detail); err != nil {
			return nil, 0, err
		}
		e.Success = ok != 0
		out = append(out, &e)
	}
	return out, total, rows.Err()
}
