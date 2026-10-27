package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/12312bg/mdbmc/internal/vault"
)

// User is a local account. KDFSalt, KDF and WrappedKey hold this user's copy
// of the vault data key.
type User struct {
	ID                 int64
	Username           string
	DisplayName        string
	Role               string
	KDFSalt            []byte
	KDF                vault.KDFParams
	WrappedKey         []byte
	Disabled           bool
	MustChangePassword bool
	CreatedAt          int64
	UpdatedAt          int64
	LastLoginAt        int64
}

const userCols = `id, username, display_name, role, kdf_salt, kdf_params, wrapped_key,
	disabled, must_change_password, created_at, updated_at, last_login_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var kdf string
	var disabled, mustChange int
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.KDFSalt, &kdf, &u.WrappedKey,
		&disabled, &mustChange, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(kdf), &u.KDF); err != nil {
		return nil, err
	}
	u.Disabled = disabled != 0
	u.MustChangePassword = mustChange != 0
	return &u, nil
}

// CountUsers returns the number of local users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// CountActiveAdmins returns the number of enabled administrators.
func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0").Scan(&n)
	return n, err
}

// UserByName looks up a user by case-insensitive username.
func (s *Store) UserByName(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE username = ?", username))
}

// UserByID looks up a user by id.
func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE id = ?", id))
}

// ListUsers returns all users ordered by username.
func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userCols+" FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CreateUser inserts u and sets its ID.
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	kdf, _ := json.Marshal(u.KDF)
	t := now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO users
		(username, display_name, role, kdf_salt, kdf_params, wrapped_key, disabled, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.DisplayName, u.Role, u.KDFSalt, string(kdf), u.WrappedKey,
		boolInt(u.Disabled), boolInt(u.MustChangePassword), t, t)
	if isUniqueErr(err) {
		return errors.New("用户名已存在")
	}
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	u.CreatedAt, u.UpdatedAt = t, t
	return nil
}

// UpdateUserProfile changes display name, role and disabled flag.
func (s *Store) UpdateUserProfile(ctx context.Context, id int64, displayName, role string, disabled bool) error {
	return s.execOne(ctx, `UPDATE users SET display_name = ?, role = ?, disabled = ?, updated_at = ? WHERE id = ?`,
		displayName, role, boolInt(disabled), now(), id)
}

// UpdateUserKey stores a newly wrapped data key (password change or reset).
func (s *Store) UpdateUserKey(ctx context.Context, id int64, salt []byte, kdf vault.KDFParams, wrapped []byte, mustChange bool) error {
	b, _ := json.Marshal(kdf)
	return s.execOne(ctx, `UPDATE users SET kdf_salt = ?, kdf_params = ?, wrapped_key = ?, must_change_password = ?, updated_at = ? WHERE id = ?`,
		salt, string(b), wrapped, boolInt(mustChange), now(), id)
}

// TouchLogin records a successful login.
func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE users SET last_login_at = ? WHERE id = ?", now(), id)
	return err
}

// DeleteUser removes a user.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.execOne(ctx, "DELETE FROM users WHERE id = ?", id)
}

func (s *Store) execOne(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
