//go:build windows

package update

import (
	"slices"
	"testing"
)

func TestProcessesIn(t *testing.T) {
	dir := t.TempDir()
	startCopy(t, dir, "mdbmc.exe")
	startCopy(t, dir, "javaw.exe")
	if got := processesIn(dir); !slices.Equal(got, []string{"javaw.exe", "mdbmc.exe"}) {
		t.Fatalf("processesIn = %v", got)
	}
	if got := processesIn(t.TempDir()); len(got) != 0 {
		t.Fatalf("empty folder: %v", got)
	}
}
