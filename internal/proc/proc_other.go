//go:build !windows

// Package proc starts processes that outlive this one and waits for
// processes to exit; the update helper and the program's own restart use it.
package proc

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// WaitExit waits for process pid to exit.
func WaitExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		p, err := os.FindProcess(pid)
		if err != nil || p.Signal(syscall.Signal(0)) != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timeout")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// StartDetached starts a process that outlives this one.
func StartDetached(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
