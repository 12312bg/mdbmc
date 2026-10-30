package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Backup writes a consistent copy of the database to path, replacing any
// file there.
func (s *Store) Backup(ctx context.Context, path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}

// BackupInfo describes a database backup.
type BackupInfo struct {
	Users   int
	Devices int
}

// InspectBackup checks, read-only, that path is a sound database of this
// program that this version can open.
func InspectBackup(ctx context.Context, path string) (*BackupInfo, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	notOurs := errors.New("该文件不是本软件的数据库备份")
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version < 1 {
		return nil, notOurs
	}
	if version > len(migrations) {
		return nil, errors.New("该备份来自更新版本的软件，请先把本软件升级到同一版本或更新版本")
	}
	var check string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return nil, fmt.Errorf("备份文件已损坏（%s）", check)
	}
	var info BackupInfo
	if err := db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM users), (SELECT COUNT(*) FROM devices)").
		Scan(&info.Users, &info.Devices); err != nil {
		return nil, notOurs
	}
	if info.Users == 0 {
		return nil, errors.New("备份中没有用户账号，无法用于恢复")
	}
	return &info, nil
}
