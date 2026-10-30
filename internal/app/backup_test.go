package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/console"
	"github.com/12312bg/mdbmc/internal/store"
)

func TestBackupAndRestore(t *testing.T) {
	h := newHarness(t)
	backups := &BackupService{a: h.app}
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	_, err = h.devices.Create(DeviceInput{Name: "kept", Host: "127.0.0.1", IPMIPort: 1, HTTPSPort: 1, Username: "u", Password: "p"})
	must(t, err)

	dir := t.TempDir()
	backup := filepath.Join(dir, "backup.db")
	h.app.SetFileDialogs(fakeDialogs{save: backup})
	got, err := backups.Backup()
	must(t, err)
	if got != backup {
		t.Fatalf("backup written to %q", got)
	}
	// Later changes are what the restore undoes.
	_, err = h.devices.Create(DeviceInput{Name: "later", Host: "127.0.0.2", IPMIPort: 1, HTTPSPort: 1, Username: "u", Password: "p"})
	must(t, err)

	// Files that are not usable backups are refused.
	junk := filepath.Join(dir, "junk.db")
	must(t, os.WriteFile(junk, []byte("not a database"), 0o600))
	future := filepath.Join(dir, "future.db")
	must(t, copyFile(backup, future))
	db, err := sql.Open("sqlite", future)
	must(t, err)
	_, err = db.Exec("PRAGMA user_version = 99")
	must(t, err)
	db.Close()
	live := filepath.Join(h.app.dataDir, dbFileName)
	for path, want := range map[string]string{junk: "不是本软件", future: "更新版本", live: "正在使用"} {
		h.app.SetFileDialogs(fakeDialogs{open: path})
		if _, err := backups.InspectRestore(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
	}

	h.app.SetFileDialogs(fakeDialogs{open: backup})
	f, err := backups.InspectRestore()
	must(t, err)
	if f.Users != 1 || f.Devices != 1 || f.File != "backup.db" {
		t.Fatalf("backup info %+v", f)
	}
	if _, err := backups.Restore("wrong"); err == nil {
		t.Fatal("restore with a wrong token")
	}
	res, err := backups.Restore(f.Token)
	must(t, err)
	if res.Restarting {
		t.Fatal("tests cannot restart")
	}
	if _, err := os.Stat(filepath.Join(h.app.dataDir, restorePendingDB)); err != nil {
		t.Fatal("backup not staged")
	}

	// The next start swaps the backup in and records it.
	h.app.Shutdown()
	a2, err := New(h.app.dataDir, func(console.Request) error { return nil })
	must(t, err)
	t.Cleanup(a2.Shutdown)
	devs, err := a2.store.ListDevices(context.Background())
	must(t, err)
	if len(devs) != 1 || devs[0].Name != "kept" {
		t.Fatalf("devices after restore: %+v", devs)
	}
	rows, _, err := a2.store.ListAudit(context.Background(), store.AuditQuery{Keyword: "backup.restore"})
	must(t, err)
	if len(rows) != 1 || !rows[0].Success || rows[0].Username != "admin" || !strings.Contains(rows[0].Detail, "before-restore") {
		t.Fatalf("restore audit %+v", rows)
	}
	matches, _ := filepath.Glob(filepath.Join(h.app.dataDir, dbFileName+".before-restore-*"))
	if len(matches) == 0 {
		t.Fatal("replaced database not kept")
	}
	if _, err := os.Stat(filepath.Join(h.app.dataDir, restorePendingDB)); !os.IsNotExist(err) {
		t.Fatal("staged backup left behind")
	}
}

func TestBackupNeedsAdministrator(t *testing.T) {
	h := newHarness(t)
	backups := &BackupService{a: h.app}
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	must(t, h.admin.CreateUser(UserInput{Username: "op", Role: "operator", Password: "Operator@1"}))
	h.authS.Logout()
	_, err = h.authS.Login("op", "Operator@1")
	must(t, err)
	must(t, h.authS.ChangePassword("Operator@1", "Operator@2"))
	h.app.SetFileDialogs(fakeDialogs{save: filepath.Join(t.TempDir(), "b.db")})
	if _, err := backups.Backup(); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("operator backup: %v", err)
	}
	if _, err := backups.InspectRestore(); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("operator restore: %v", err)
	}
}
