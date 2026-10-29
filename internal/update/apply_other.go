//go:build !windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func runInstaller(string, string) error {
	return errors.New("当前系统不支持运行安装程序")
}

// waitExit waits for process pid to exit.
func waitExit(pid int, timeout time.Duration) error {
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

// startDetached starts a process that outlives this one.
func startDetached(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// waitDirFree waits until the program and the Java runtime in dir can be
// opened for writing; a running executable cannot.
func waitDirFree(dir, exe string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for _, path := range []string{exe, filepath.Join(dir, "jre", "bin", "java")} {
		for {
			f, err := os.OpenFile(path, os.O_WRONLY, 0)
			if err == nil {
				f.Close()
				break
			}
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s 仍在运行，请关闭后重试", filepath.Base(path))
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
	return nil
}
