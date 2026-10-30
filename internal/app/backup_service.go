package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/store"
)

// BackupService backs the database up and restores it; administrators only.
//
// A restore replaces the database file, which is open while the program
// runs: the chosen backup is copied next to it and swapped in at the next
// start, keeping the replaced database as mdbmc.db.before-restore-<time>.
type BackupService struct{ a *App }

const (
	dbFileName         = "mdbmc.db"
	restorePendingDB   = "restore-pending.db"
	restorePendingNote = "restore-pending.json"
	backupPattern      = "*.db"
)

// BackupFile describes a backup chosen for restoring.
type BackupFile struct {
	Token   string `json:"token"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Users   int    `json:"users"`
	Devices int    `json:"devices"`
}

// RestoreResult tells whether the program restarts by itself to finish the
// restore.
type RestoreResult struct {
	Restarting bool `json:"restarting"`
}

type pendingRestore struct {
	token   string
	path    string
	userID  int64
	created time.Time
}

// restoreNote is left for the next start, which records the restore.
type restoreNote struct {
	File   string `json:"file"`
	User   string `json:"user"`
	UserID int64  `json:"userId"`
}

var restoreMu sync.Mutex

// SetRestart lets a restore restart the program.
func (a *App) SetRestart(restart func()) { a.restart = restart }

// Backup saves a copy of the database chosen with the save dialog. It
// returns the file written, or "" when the user cancelled.
func (s *BackupService) Backup() (string, error) {
	sess, err := s.a.auth.Require(auth.PermSettingsManage)
	if err != nil {
		return "", err
	}
	path, err := s.a.saveDialog("备份数据库", "mdbmc-备份-"+time.Now().Format("20060102-1504")+".db", "数据库备份", backupPattern)
	if err != nil || path == "" {
		return "", err
	}
	if s.a.isLiveDB(path) {
		return "", errors.New("不能覆盖正在使用的数据库，请换一个位置")
	}
	err = s.a.store.Backup(s.a.bg(), path)
	s.a.audit(sess, "backup.create", filepath.Base(path), err, "")
	if err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) isLiveDB(path string) bool {
	live, err1 := os.Stat(filepath.Join(a.dataDir, dbFileName))
	other, err2 := os.Stat(path)
	return err1 == nil && err2 == nil && os.SameFile(live, other)
}

// InspectRestore asks for a backup and checks it; Restore then uses it. It
// returns nil when the user cancelled.
func (s *BackupService) InspectRestore() (*BackupFile, error) {
	sess, err := s.a.auth.Require(auth.PermSettingsManage)
	if err != nil {
		return nil, err
	}
	path, err := s.a.openDialog("选择要恢复的数据库备份", "数据库备份", backupPattern)
	if err != nil || path == "" {
		return nil, err
	}
	return s.a.inspectRestore(sess, path)
}

func (a *App) inspectRestore(sess *auth.Session, path string) (*BackupFile, error) {
	if a.isLiveDB(path) {
		return nil, errors.New("这是正在使用的数据库，请选择备份文件")
	}
	info, err := store.InspectBackup(a.bg(), path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	token := make([]byte, 16)
	_, _ = rand.Read(token)
	f := &BackupFile{Token: hex.EncodeToString(token), File: filepath.Base(path), Size: fi.Size(),
		ModTime: fi.ModTime().Unix(), Users: info.Users, Devices: info.Devices}
	restoreMu.Lock()
	a.restorePending = &pendingRestore{token: f.Token, path: path, userID: sess.UserID, created: time.Now()}
	restoreMu.Unlock()
	return f, nil
}

// Restore stages the inspected backup and restarts the program, which
// swaps it in. Accounts and passwords become those of the backup.
func (s *BackupService) Restore(token string) (*RestoreResult, error) {
	sess, err := s.a.auth.Require(auth.PermSettingsManage)
	if err != nil {
		return nil, err
	}
	restoreMu.Lock()
	p := s.a.restorePending
	if p == nil || p.token != token || p.userID != sess.UserID || time.Since(p.created) > 30*time.Minute {
		restoreMu.Unlock()
		return nil, errors.New("请重新选择备份文件")
	}
	s.a.restorePending = nil
	restoreMu.Unlock()

	err = s.a.stageRestore(sess, p.path)
	s.a.audit(sess, "backup.restore", filepath.Base(p.path), err, "重启后生效")
	if err != nil {
		return nil, err
	}
	res := &RestoreResult{Restarting: s.a.restart != nil}
	if res.Restarting {
		go func() {
			// Let this call return to the window before it closes.
			time.Sleep(800 * time.Millisecond)
			s.a.closeConsoles()
			s.a.restart()
		}()
	}
	return res, nil
}

// stageRestore copies the backup next to the database and checks the copy.
func (a *App) stageRestore(sess *auth.Session, src string) error {
	dst := filepath.Join(a.dataDir, restorePendingDB)
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("复制备份失败：%w", err)
	}
	if _, err := store.InspectBackup(a.bg(), dst); err != nil {
		os.Remove(dst)
		return err
	}
	note, _ := json.Marshal(restoreNote{File: filepath.Base(src), User: sess.Username, UserID: sess.UserID})
	return os.WriteFile(filepath.Join(a.dataDir, restorePendingNote), note, 0o600)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// applyPendingRestore swaps a staged backup in before the database is
// opened. The replaced database (with its WAL files, which must not be
// applied to the backup) is kept under a dated name.
func applyPendingRestore(dir string) (*restoreNote, string, error) {
	pending := filepath.Join(dir, restorePendingDB)
	if _, err := os.Stat(pending); err != nil {
		return nil, "", nil
	}
	note := &restoreNote{}
	if b, err := os.ReadFile(filepath.Join(dir, restorePendingNote)); err == nil {
		_ = json.Unmarshal(b, note)
	}
	db := filepath.Join(dir, dbFileName)
	kept := db + ".before-restore-" + time.Now().Format("20060102-150405")
	var moved []string
	undo := func() {
		for _, sfx := range moved {
			_ = os.Rename(kept+sfx, db+sfx)
		}
	}
	for _, sfx := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(db + sfx); err == nil {
			if err := os.Rename(db+sfx, kept+sfx); err != nil {
				undo()
				return nil, "", err
			}
			moved = append(moved, sfx)
		}
	}
	if err := os.Rename(pending, db); err != nil {
		undo()
		return nil, "", err
	}
	_ = os.Remove(filepath.Join(dir, restorePendingNote))
	return note, filepath.Base(kept), nil
}

// recordRestore notes a restore done at startup in the restored database.
func (a *App) recordRestore(note *restoreNote, kept string, restoreErr error) {
	e := &store.AuditEntry{Action: "backup.restore", Success: restoreErr == nil}
	if note != nil {
		e.UserID, e.Username, e.Target = note.UserID, note.User, note.File
		e.Detail = "已恢复；原数据库保存为 " + kept
	}
	if restoreErr != nil {
		e.Detail = "恢复失败，仍使用原数据库：" + restoreErr.Error()
	}
	if err := a.store.AddAudit(context.Background(), e); err != nil {
		log.Printf("audit: %v", err)
	}
	log.Printf("restore: %s", strings.TrimSpace(e.Detail))
}
