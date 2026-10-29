package update

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// HelperFlag starts the program as the update helper:
//
//	<copy of the program> --apply-update <plan.json>
//
// The running program cannot overwrite its own executable, so it copies
// itself into the download directory, starts the copy with a plan and
// exits. The helper waits for the program (and its console windows) to
// exit, checks the package once more, runs the installer or replaces the
// files of a portable copy, records the outcome for the next start, and
// starts the program again.
const HelperFlag = "--apply-update"

// Files the helper uses in the download directory.
const (
	planFile   = "plan.json"
	resultFile = "result.json"
)

func helperName() string {
	if runtime.GOOS == "windows" {
		return "mdbmc-updater.exe"
	}
	return "mdbmc-updater"
}

// How long the helper waits for the program and its files to be released.
var (
	exitTimeout   = 60 * time.Second
	unlockTimeout = 60 * time.Second
)

// Plan tells the helper what to install.
type Plan struct {
	PID     int    `json:"pid"` // the program instance to wait for
	Exe     string `json:"exe"` // the program to replace and start again
	Mode    Mode   `json:"mode"`
	Package string `json:"package"`
	Asset   Asset  `json:"asset"`
	From    string `json:"from"`
	Version string `json:"version"`
	User    string `json:"user"`
	UserID  int64  `json:"userId"`
	Log     string `json:"log"` // log file to append to
}

// Result is the helper's outcome, read by the next start of the program.
type Result struct {
	From    string `json:"from"`
	Version string `json:"version"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Time    int64  `json:"time"`
	User    string `json:"user"`
	UserID  int64  `json:"userId"`
}

// DetectMode tells how the program at exe was installed: the NSIS installer
// leaves uninstall.exe next to it.
func DetectMode(exe string) Mode {
	if runtime.GOOS != "windows" {
		return ModeUnsupported
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "uninstall.exe")); err == nil {
		return ModeInstaller
	}
	return ModePortable
}

// Launch starts the helper for p from a copy of the running program in dir.
// The caller should exit soon after it returns.
func Launch(dir string, p Plan) error {
	if p.Mode == ModeUnsupported {
		return errors.New("当前系统不支持在线安装更新")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	helper := filepath.Join(dir, helperName())
	if err := copyFile(self, helper, 0o700); err != nil {
		return fmt.Errorf("准备更新程序失败: %w", err)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	planPath := filepath.Join(dir, planFile)
	if err := os.WriteFile(planPath, b, 0o600); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, resultFile))
	if err := startDetached(helper, []string{HelperFlag, planPath}, dir); err != nil {
		return fmt.Errorf("启动更新程序失败: %w", err)
	}
	return nil
}

// TakeResult returns the outcome of the last update, if the helper left
// one, and removes the helper's files.
func TakeResult(dir string) *Result {
	_ = os.Remove(filepath.Join(dir, planFile))
	_ = os.Remove(filepath.Join(dir, helperName())) // fails while the helper is still exiting; removed next time
	p := filepath.Join(dir, resultFile)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	_ = os.Remove(p)
	var r Result
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// errStillRunning means the old program did not exit; it is not started again.
var errStillRunning = errors.New("等待软件退出超时，已取消更新")

// RunHelper is the helper's main function.
func RunHelper(planPath string) {
	b, err := os.ReadFile(planPath)
	if err != nil {
		return
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return
	}
	if p.Log != "" {
		if f, err := os.OpenFile(p.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			defer f.Close()
			log.SetOutput(f)
		}
	}
	log.Printf("update: installing %s over %s (%s, %s)", p.Version, p.From, p.Mode, p.Package)
	err = apply(p)
	res := Result{From: p.From, Version: p.Version, Success: err == nil, Time: time.Now().Unix(), User: p.User, UserID: p.UserID}
	if err != nil {
		res.Error = err.Error()
		log.Printf("update: failed: %v", err)
	} else {
		log.Printf("update: installed %s", p.Version)
	}
	if b, err := json.Marshal(res); err == nil {
		_ = os.WriteFile(filepath.Join(filepath.Dir(planPath), resultFile), b, 0o600)
	}
	if errors.Is(err, errStillRunning) {
		return
	}
	if err := startDetached(p.Exe, nil, filepath.Dir(p.Exe)); err != nil {
		log.Printf("update: restart %s: %v", p.Exe, err)
	}
}

func apply(p Plan) error {
	if err := waitExit(p.PID, exitTimeout); err != nil {
		return errStillRunning
	}
	// Console windows and Java consoles close shortly after the program;
	// their files must be free before they can be replaced.
	dir := filepath.Dir(p.Exe)
	if err := waitDirFree(dir, p.Exe, unlockTimeout); err != nil {
		return err
	}
	if err := verifyFile(p.Package, p.Asset); err != nil {
		return err
	}
	switch p.Mode {
	case ModeInstaller:
		return runInstaller(p.Package, dir)
	case ModePortable:
		return applyPortable(p.Package, p.Exe)
	}
	return fmt.Errorf("不支持的安装方式 %q", p.Mode)
}

// applyPortable replaces the files of a portable copy (the program and its
// jre folder) with those in the zip at pkg. The zip is unpacked next to the
// program first; the old files are moved aside and put back if anything
// fails.
func applyPortable(pkg, exe string) error {
	dir := filepath.Dir(exe)
	staging := filepath.Join(dir, ".mdbmc-update")
	backup := filepath.Join(dir, ".mdbmc-old")
	_ = os.RemoveAll(staging)
	_ = os.RemoveAll(backup)
	defer os.RemoveAll(staging)

	tops, err := unzip(pkg, staging, filepath.Base(exe))
	if err != nil {
		return fmt.Errorf("解压更新包失败: %w", err)
	}
	if !tops[filepath.Base(exe)] {
		return errors.New("更新包中没有找到 mdbmc.exe")
	}
	if err := os.MkdirAll(backup, 0o700); err != nil {
		return err
	}
	names := make([]string, 0, len(tops))
	for n := range tops {
		names = append(names, n)
	}
	sort.Strings(names)
	var moved, placed []string
	rollback := func() {
		for _, n := range placed {
			_ = os.RemoveAll(filepath.Join(dir, n))
		}
		for _, n := range moved {
			_ = os.Rename(filepath.Join(backup, n), filepath.Join(dir, n))
		}
	}
	for _, n := range names {
		cur := filepath.Join(dir, n)
		if _, err := os.Lstat(cur); err == nil {
			if err := os.Rename(cur, filepath.Join(backup, n)); err != nil {
				rollback()
				return fmt.Errorf("替换 %s 失败: %w", n, err)
			}
			moved = append(moved, n)
		}
		if err := os.Rename(filepath.Join(staging, n), cur); err != nil {
			rollback()
			return fmt.Errorf("替换 %s 失败: %w", n, err)
		}
		placed = append(placed, n)
	}
	_ = os.RemoveAll(backup)
	return nil
}

// unzip extracts pkg into dst and returns the top-level names it created.
// The zip's mdbmc.exe is written as exeName, so a renamed program keeps its
// name.
func unzip(pkg, dst, exeName string) (map[string]bool, error) {
	zr, err := zip.OpenReader(pkg)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	tops := map[string]bool{}
	for _, f := range zr.File {
		// Archives made on Windows may use backslashes.
		name := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		if name == "." || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, ":") {
			return nil, fmt.Errorf("更新包中的路径不安全: %q", f.Name)
		}
		if name == "mdbmc.exe" {
			name = exeName
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		tops[strings.SplitN(name, "/", 2)[0]] = true
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := extractFile(f, target); err != nil {
			return nil, err
		}
	}
	return tops, nil
}

func extractFile(f *zip.File, target string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode().Perm()|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
