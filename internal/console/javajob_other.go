//go:build !windows

package console

import "os/exec"

// startTied starts a console's Java process. Console windows only exist on
// Windows; elsewhere (tests) the process is not tied to this one.
func startTied(cmd *exec.Cmd) error { return cmd.Start() }
