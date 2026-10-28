//go:build !windows

package console

import (
	"fmt"
	"os"
)

// RunWindow is only implemented on Windows, the target platform. Development
// builds on other systems open console pages in the default browser instead
// (see main.go).
func RunWindow() {
	fmt.Fprintln(os.Stderr, "console windows are only supported on Windows")
	os.Exit(1)
}
