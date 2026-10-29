package update

import (
	"archive/zip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// The test binary doubles as a long-running program for TestWaitDirFree.
const sleepEnv = "MDBMC_UPDATE_TEST_SLEEP"

func TestMain(m *testing.M) {
	if os.Getenv(sleepEnv) != "" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// startCopy runs a copy of the test binary from dir as name; stop (also run
// when the test ends) ends it.
func startCopy(t *testing.T, dir, name string) (cmd *exec.Cmd, stop func()) {
	t.Helper()
	exe := filepath.Join(dir, name)
	if err := copyFile(os.Args[0], exe, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(exe)
	cmd.Env = append(os.Environ(), sleepEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop = func() {
		once.Do(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
	}
	t.Cleanup(stop)
	return cmd, stop
}

func TestWaitDirFree(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mdbmc.exe")
	_, stop := startCopy(t, dir, "mdbmc.exe")
	if err := waitDirFree(dir, exe, 500*time.Millisecond); err == nil || !strings.Contains(err.Error(), "mdbmc.exe") {
		t.Fatalf("running program not detected: %v", err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		stop()
	}()
	if err := waitDirFree(dir, exe, 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

func makeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// exitedPID returns the PID of a process that has already exited.
func exitedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func portablePlan(t *testing.T, pkgFiles map[string]string) (Plan, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the restarted program is a shell script")
	}
	app := t.TempDir()
	writeFile(t, app, "mdbmc.exe", "#!/bin/sh\nexit 0\n")
	if err := os.MkdirAll(filepath.Join(app, "jre", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(app, "jre", "bin"), "javaw.exe", "old java")
	writeFile(t, filepath.Join(app, "jre"), "old-only.txt", "old")
	writeFile(t, app, "notes.txt", "user file")

	work := t.TempDir()
	pkg := filepath.Join(work, "mdbmc-portable-windows-amd64.zip")
	makeZip(t, pkg, pkgFiles)
	sum, size, err := hashFile(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return Plan{
		PID: exitedPID(t), Exe: filepath.Join(app, "mdbmc.exe"), Mode: ModePortable, Package: pkg,
		Asset: Asset{Name: filepath.Base(pkg), Size: size, SHA256: sum}, From: "1.0.0", Version: "1.1.0",
		User: "admin", UserID: 1, Log: filepath.Join(work, "mdbmc.log"),
	}, work
}

func runPlan(t *testing.T, p Plan, work string) *Result {
	t.Helper()
	b, _ := json.Marshal(p)
	planPath := filepath.Join(work, planFile)
	if err := os.WriteFile(planPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	RunHelper(planPath)
	r := TakeResult(work)
	if r == nil {
		t.Fatal("no result")
	}
	if TakeResult(work) != nil {
		t.Fatal("result reported twice")
	}
	return r
}

func TestPortableUpdate(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "restarted")
	p, work := portablePlan(t, map[string]string{
		"mdbmc.exe": "#!/bin/sh\ntouch " + marker + "\n",
		// Windows' Compress-Archive may write backslashes.
		`jre\bin\javaw.exe`: "new java",
		"jre/lib/rt.jar":    "jar",
	})
	app := filepath.Dir(p.Exe)
	r := runPlan(t, p, work)
	if !r.Success || r.Error != "" || r.Version != "1.1.0" || r.From != "1.0.0" || r.User != "admin" {
		t.Fatalf("result %+v", r)
	}
	if got := readFile(t, p.Exe); !strings.Contains(got, marker) {
		t.Fatalf("program not replaced: %q", got)
	}
	if readFile(t, filepath.Join(app, "jre", "bin", "javaw.exe")) != "new java" || readFile(t, filepath.Join(app, "jre", "lib", "rt.jar")) != "jar" {
		t.Fatal("jre not replaced")
	}
	if _, err := os.Stat(filepath.Join(app, "jre", "old-only.txt")); !os.IsNotExist(err) {
		t.Fatal("old jre files left behind")
	}
	if readFile(t, filepath.Join(app, "notes.txt")) != "user file" {
		t.Fatal("unrelated file touched")
	}
	for _, n := range []string{".mdbmc-update", ".mdbmc-old"} {
		if _, err := os.Stat(filepath.Join(app, n)); !os.IsNotExist(err) {
			t.Fatalf("%s left behind", n)
		}
	}
	// The new program is started.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("program not restarted")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(readFile(t, p.Log), "update: installed 1.1.0") {
		t.Fatal("not logged")
	}
}

func TestPortableUpdateKeepsOldFilesOnFailure(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"unsafe path": {"mdbmc.exe": "new", "../evil.txt": "x"},
		"no program":  {"jre/bin/javaw.exe": "new java"},
	} {
		t.Run(name, func(t *testing.T) {
			p, work := portablePlan(t, files)
			r := runPlan(t, p, work)
			if r.Success || r.Error == "" {
				t.Fatalf("result %+v", r)
			}
			app := filepath.Dir(p.Exe)
			if readFile(t, p.Exe) != "#!/bin/sh\nexit 0\n" || readFile(t, filepath.Join(app, "jre", "bin", "javaw.exe")) != "old java" {
				t.Fatal("old files changed")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(app), "evil.txt")); !os.IsNotExist(err) {
				t.Fatal("file written outside the program folder")
			}
		})
	}
}

func TestHelperRechecksPackage(t *testing.T) {
	p, work := portablePlan(t, map[string]string{"mdbmc.exe": "new"})
	if err := os.WriteFile(p.Package, []byte("swapped after download"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runPlan(t, p, work)
	if r.Success || !strings.Contains(r.Error, "校验失败") {
		t.Fatalf("result %+v", r)
	}
	if readFile(t, p.Exe) != "#!/bin/sh\nexit 0\n" {
		t.Fatal("program replaced from a bad package")
	}
}

func TestHelperWaitsForProgramExit(t *testing.T) {
	old := exitTimeout
	exitTimeout = 300 * time.Millisecond
	defer func() { exitTimeout = old }()
	p, work := portablePlan(t, map[string]string{"mdbmc.exe": "new"})
	p.PID = os.Getpid() // still running
	r := runPlan(t, p, work)
	if r.Success || r.Error != errStillRunning.Error() {
		t.Fatalf("result %+v", r)
	}
}
