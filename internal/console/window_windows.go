//go:build windows

package console

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"syscall"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func messageBox(title, text string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	windows.MessageBox(0, m, t, windows.MB_OK|windows.MB_ICONERROR)
}

// RunWindow is the entry point of a console window process.
func RunWindow() {
	var req Request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&req); err != nil {
		messageBox("MDBMC", "控制台启动参数无效: "+err.Error())
		os.Exit(2)
	}
	script, err := initScript(req.Spec)
	if err != nil {
		messageBox("MDBMC", err.Error())
		os.Exit(2)
	}
	// BMC certificates are self-signed; the main process verified the pinned
	// fingerprint just before starting this window. The flag only applies to
	// this process's private browser profile.
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--ignore-certificate-errors")
	if req.Profile != "" {
		_ = os.MkdirAll(req.Profile, 0o700)
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
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
		messageBox("MDBMC", "无法创建 WebView2 窗口。请确认已安装 Microsoft Edge WebView2 Runtime。")
		os.Exit(1)
	}
	defer w.Destroy()
	w.Init(script)
	w.Navigate(req.Spec.URL)
	w.Run()
}
