//go:build windows

package update

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProcessesIn(t *testing.T) {
	dir := t.TempDir()
	cmd, _ := startCopy(t, dir, "mdbmc.exe")
	startCopy(t, dir, "javaw.exe")
	if got := processesIn(dir); !slices.Equal(got, []string{"javaw.exe", "mdbmc.exe"}) {
		img, err := imagePath(uint32(cmd.Process.Pid))
		t.Fatalf("processesIn(%s) = %v; mdbmc.exe runs as %q (%v), folder compared as %q", dir, got, img, err, normPath(dir))
	}
	if got := processesIn(t.TempDir()); len(got) != 0 {
		t.Fatalf("empty folder: %v", got)
	}
}

// GitHub's Windows runners keep temporary files under a short (8.3) path
// such as C:\Users\RUNNER~1.
func TestNormPath(t *testing.T) {
	long := filepath.Join(t.TempDir(), "Long Folder Name")
	if err := os.Mkdir(long, 0o755); err != nil {
		t.Fatal(err)
	}
	in, _ := windows.UTF16PtrFromString(long)
	buf := make([]uint16, windows.MAX_PATH)
	n, err := windows.GetShortPathName(in, &buf[0], uint32(len(buf)))
	short := windows.UTF16ToString(buf[:n])
	if err != nil || !strings.Contains(short, "~") {
		t.Skipf("no short names on this volume (%q, %v)", short, err)
	}
	if normPath(short) != normPath(long) || normPath(strings.ToUpper(long)) != normPath(long) {
		t.Fatalf("normPath(%q) = %q, normPath(%q) = %q", short, normPath(short), long, normPath(long))
	}
}
