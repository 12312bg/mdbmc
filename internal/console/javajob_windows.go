//go:build windows

package console

import (
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A job object that kills its processes when this process exits: a Java
// console closes with the window that started it (window closed, logout,
// main program exit).
var (
	jobOnce sync.Once
	job     windows.Handle
	jobErr  error
)

func consoleJob() (windows.Handle, error) {
	jobOnce.Do(func() {
		job, jobErr = windows.CreateJobObject(nil, nil)
		if jobErr != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		_, jobErr = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	})
	return job, jobErr
}

// startTied starts cmd inside the console's job object.
func startTied(cmd *exec.Cmd) error {
	j, err := consoleJob()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(j, h)
		windows.CloseHandle(h)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	return nil
}
