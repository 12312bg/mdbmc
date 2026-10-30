//go:build windows

// Package proc starts processes that outlive this one and waits for
// processes to exit; the update helper and the program's own restart use it.
package proc

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// WaitExit waits for process pid to exit.
func WaitExit(pid int, timeout time.Duration) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil // already gone
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
	if err != nil {
		return err
	}
	if ev != windows.WAIT_OBJECT_0 {
		return errors.New("timeout")
	}
	return nil
}

// StartDetached starts a process that outlives this one, outside any job
// this process belongs to when the job allows that.
func StartDetached(exe string, args []string, dir string) error {
	flags := uint32(windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP)
	start := func(extra uint32) error {
		cmd := exec.Command(exe, args...)
		cmd.Dir = dir
		cmd.Env = cleanEnv(os.Environ())
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags | extra}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	if err := start(windows.CREATE_BREAKAWAY_FROM_JOB); err == nil {
		return nil
	}
	return start(0)
}

// cleanEnv drops the WEBVIEW2_* variables the WebView2 loader sets to empty
// strings in this process; a new program instance must not inherit them.
func cleanEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(strings.ToUpper(kv), "WEBVIEW2_") {
			out = append(out, kv)
		}
	}
	return out
}
