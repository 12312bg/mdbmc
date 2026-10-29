//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	hwnd       windows.Handle
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    windows.Handle
	idList     uintptr
	class      *uint16
	keyClass   windows.Handle
	hotKey     uint32
	icon       windows.Handle
	process    windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
	swShowNormal          = 1
	installerTimeout      = 15 * time.Minute
)

// runInstaller runs the NSIS installer silently into dir and waits for it.
// The installer asks for administrator rights itself (UAC).
func runInstaller(pkg, dir string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(pkg)
	if err != nil {
		return err
	}
	// NSIS wants /D last and unquoted, even when the path has spaces.
	params, err := windows.UTF16PtrFromString("/S /D=" + dir)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		mask: seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		verb: verb, file: file, parameters: params, show: swShowNormal,
	}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return errors.New("没有获得管理员授权（UAC 已取消）")
		}
		return fmt.Errorf("启动安装程序失败: %w", callErr)
	}
	if info.process == 0 {
		return errors.New("启动安装程序失败")
	}
	defer windows.CloseHandle(info.process)
	ev, err := windows.WaitForSingleObject(info.process, uint32(installerTimeout.Milliseconds()))
	if err != nil {
		return err
	}
	if ev != windows.WAIT_OBJECT_0 {
		return errors.New("安装程序运行超时")
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("安装程序返回错误代码 %d", code)
	}
	return nil
}

// waitExit waits for process pid to exit.
func waitExit(pid int, timeout time.Duration) error {
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

// waitDirFree waits until no process runs a program from dir: the console
// windows (mdbmc.exe) and Java consoles (jre\bin\javaw.exe) exit shortly
// after the main program. Checking processes rather than opening the files
// works without write access to the folder (Program Files).
func waitDirFree(dir, _ string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		names := processesIn(dir)
		if len(names) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s 仍在运行，请关闭后重试", strings.Join(names, "、"))
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// processesIn returns the image names of the processes started from dir.
func processesIn(dir string) []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	prefix := strings.ToLower(longPath(filepath.Clean(dir))) + `\`
	self := uint32(os.Getpid())
	var names []string
	buf := make([]uint16, windows.MAX_LONG_PATH)
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID == self {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		n := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &n)
		windows.CloseHandle(h)
		if err == nil && strings.HasPrefix(strings.ToLower(windows.UTF16ToString(buf[:n])), prefix) {
			names = append(names, windows.UTF16ToString(e.ExeFile[:]))
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// longPath expands 8.3 short names (C:\Users\RUNNER~1) so paths compare.
func longPath(p string) string {
	in, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetLongPathName(in, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || n > uint32(len(buf)) {
		return p
	}
	return windows.UTF16ToString(buf[:n])
}

// startDetached starts a process that outlives this one, outside any job
// this process belongs to when the job allows that.
func startDetached(exe string, args []string, dir string) error {
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
