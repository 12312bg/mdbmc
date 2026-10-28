//go:build windows

package console

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// fail reports an error to the parent (which shows it in the main window)
// and exits.
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// RunWindow is the entry point of a console window process.
func RunWindow() {
	// The window, the WebView2 controller and the message loop must all
	// live on one OS thread.
	runtime.LockOSThread()
	defer func() {
		if r := recover(); r != nil {
			fail("控制台窗口崩溃: %v\n%s", r, debug.Stack())
		}
	}()

	var req Request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&req); err != nil {
		fail("控制台启动参数无效: %v", err)
	}
	script, err := initScript(req.Spec)
	if err != nil {
		fail("%v", err)
	}
	// The parent already drops inherited WEBVIEW2_* variables; clear them
	// here too in case the process was started some other way.
	for _, k := range []string{
		"WEBVIEW2_BROWSER_EXECUTABLE_FOLDER", "WEBVIEW2_USER_DATA_FOLDER",
		"WEBVIEW2_RELEASE_CHANNEL_PREFERENCE", "WEBVIEW2_PIPE_FOR_SCRIPT_DEBUGGER",
	} {
		_ = os.Unsetenv(k)
	}
	// BMC certificates are self-signed. Chromium only honours the SPKI list
	// together with a user data folder, which every console window has.
	_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", browserArgs(req.SPKIHash))
	if req.Profile != "" {
		_ = os.MkdirAll(req.Profile, 0o700)
	}

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  req.Profile,
		WindowOptions: webview2.WindowOptions{
			Title:  req.Title,
			Width:  1366,
			Height: 900,
			Center: true,
		},
	})
	if w == nil {
		fail("无法创建 WebView2 窗口。请确认已安装 Microsoft Edge WebView2 Runtime。")
	}
	defer w.Destroy()

	// The page script reports progress (page loaded, login submitted); the
	// parent logs it and the Windows smoke test waits for it.
	_ = w.Bind("__mdbmcEvent", func(name, detail string) {
		fmt.Printf("%s%s\t%s\n", eventPrefix, name, detail)
	})
	w.Init(script)
	w.Navigate(req.Spec.URL)

	hwnd := uintptr(w.Window())
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
		fail("控制台窗口已创建但不可见")
	}
	fmt.Println(readyLine)
	w.Run()
}
