//go:build windows

package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procPostMessage      = user32.NewProc("PostMessageW")
)

const (
	wsOverlappedWindow = 0x00CF0000
	swShow             = 5
	wmDestroy          = 0x0002
	wmClose            = 0x0010
	wmSize             = 0x0005
	wmActivate         = 0x0006
	idcArrow           = 32512
	smCxScreen         = 0
	smCyScreen         = 1
	colorWindow        = 5
	// wmRunQueued runs work queued with post on the window's thread.
	wmRunQueued = 0x8000 + 1 // WM_APP + 1
	// appIconID is the icon resource Wails embeds in the executable.
	appIconID = 3
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

// The window and its browser; there is one window per process. They, and
// everything below that is not marked otherwise, are only used on the
// window's thread, where the WebView2 events arrive too.
var (
	mainHwnd uintptr
	browser  *edge.Chromium
	queued   []func()
	// closeHwnd is mainHwnd for the parent watcher's goroutine.
	closeHwnd atomic.Uintptr
)

// watchParent closes the window when the main program exits or crashes, so
// console windows (and their WebView2 processes) do not outlive it.
func watchParent() {
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(os.Getppid()))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			os.Exit(0) // the main program is already gone
		}
		return
	}
	go func() {
		_, _ = windows.WaitForSingleObject(parent, windows.INFINITE)
		closeAndExit()
	}()
}

// closeAndExit closes the window and ends the process. It may be called
// from any goroutine.
func closeAndExit() {
	if h := closeHwnd.Load(); h != 0 {
		_, _, _ = procPostMessage.Call(h, wmClose, 0, 0)
		time.Sleep(3 * time.Second) // the window normally closes first
	}
	os.Exit(0)
}

// post runs f from the message loop, outside the WebView2 event that is
// being handled.
func post(f func()) {
	queued = append(queued, f)
	_, _, _ = procPostMessage.Call(mainHwnd, wmRunQueued, 0, 0)
}

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmRunQueued:
		q := queued
		queued = nil
		for _, f := range q {
			f()
		}
		return 0
	case wmSize:
		if browser != nil {
			browser.Resize()
		}
	case wmActivate:
		// Embed pumps messages before the controller exists.
		if browser != nil && browser.GetController() != nil && wParam&0xFFFF != 0 {
			browser.Focus()
		}
	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}

// fail reports an error to the parent (which shows it in the main window)
// and exits.
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func createWindow(title string, width, height int32) uintptr {
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		fail("GetModuleHandle: %v", err)
	}
	className, _ := windows.UTF16PtrFromString("MDBMCConsoleWindow")
	icon, _, _ := procLoadIcon.Call(uintptr(instance), appIconID)
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{
		WndProc:    windows.NewCallback(wndProc),
		Instance:   instance,
		Icon:       windows.Handle(icon),
		IconSm:     windows.Handle(icon),
		Cursor:     windows.Handle(cursor),
		Background: colorWindow + 1,
		ClassName:  className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		fail("RegisterClassEx: %v", err)
	}
	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	width = min(width, int32(sw))
	height = min(height, int32(sh)-40)
	x, y := (int32(sw)-width)/2, (int32(sh)-height)/2
	titlePtr, _ := windows.UTF16PtrFromString(title)
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(titlePtr)),
		wsOverlappedWindow, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		fail("CreateWindowEx: %v", err)
	}
	return hwnd
}

// RunWindow is the entry point of a console window process.
func RunWindow() {
	// The window, the WebView2 controller and the message loop must all
	// live on one OS thread.
	runtime.LockOSThread()
	// go-webview2 logs progress through the log package; stderr is reserved
	// for errors shown to the user.
	log.SetOutput(io.Discard)
	defer func() {
		if r := recover(); r != nil {
			fail("控制台窗口崩溃: %v\n%s", r, debug.Stack())
		}
	}()

	watchParent()

	var req Request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&req); err != nil {
		fail("控制台启动参数无效: %v", err)
	}
	// The main program closes stdin to close the window (on logout).
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		closeAndExit()
	}()
	// The browser talks to the BMC through a local proxy; the page script
	// and the first navigation use the proxied addresses.
	px, err := startProxy(req.Spec.URL, req.CertFingerprint, reportEvent)
	if err != nil {
		fail("%v", err)
	}
	spec := req.Spec
	spec.URL = px.localURL(spec.URL)
	spec.AfterLoginURL = px.localURL(spec.AfterLoginURL)
	script, err := initScript(spec)
	if err != nil {
		fail("%v", err)
	}
	if req.Profile != "" {
		_ = os.MkdirAll(req.Profile, 0o700)
	}
	nav = navState{start: px.startURL(req.Spec.URL), target: px.target.Host}
	reportEvent("proxy", px.origin()+" -> "+px.target.String())

	hwnd := createWindow(req.Title, 1366, 900)
	mainHwnd = hwnd
	closeHwnd.Store(hwnd)
	_, _, _ = procShowWindow.Call(hwnd, swShow)
	_, _, _ = procUpdateWindow.Call(hwnd)

	c := edge.NewChromium()
	c.DataPath = req.Profile
	c.SetErrorCallback(func(err error) {
		if err != nil {
			fail("WebView2 错误: %v", err)
		}
	})
	c.MessageCallback = func(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		if message == openMessage {
			post(func() { browser.Navigate(nav.start) })
			return
		}
		if ev, ok := strings.CutPrefix(message, "mdbmc:"); ok {
			name, detail, _ := strings.Cut(ev, "\t")
			reportEvent(name, detail)
		}
	}
	c.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		ok, status := navigationResult(args)
		nav.completed(ok, status)
	}
	c.ProcessFailedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		kind, _ := args.GetProcessFailedKind()
		reportEvent("process-failed", fmt.Sprint(kind))
		switch kind {
		case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_BROWSER_PROCESS_EXITED:
			fail("WebView2 浏览器进程意外退出")
		case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_EXITED:
			nav.show(page{
				Tone:    "error",
				Title:   "页面意外退出",
				Message: "显示 BMC 页面的进程意外结束了，可以重新连接。",
				Target:  nav.target,
				Retry:   nav.start,
			})
		}
	}
	browser = c
	if !c.Embed(hwnd) {
		fail("无法创建 WebView2 窗口。请确认已安装 Microsoft Edge WebView2 Runtime。")
	}
	c.Resize()
	// The window accepts the proxy's certificate, and the BMC's pinned one
	// for pages that link to the BMC's own address directly.
	installCertPinning(c, []string{px.certFP, req.CertFingerprint}, nav.certDecision)
	c.Init(script)
	// A status page stays on screen until the BMC's first page arrives;
	// it asks for the BMC to be opened once it is shown (openMessage).
	c.NavigateToString(page{
		Tone:    "loading",
		Title:   "正在连接 BMC",
		Message: "正在打开管理页面。部分 BMC 响应较慢，可能需要十几秒。",
		Target:  nav.target,
	}.render())

	_, _, _ = procSetForeground.Call(hwnd)
	c.Focus()
	if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
		fail("控制台窗口已创建但不可见")
	}
	reportEvent("window", strconv.FormatUint(uint64(hwnd), 10))
	writeLine(readyLine)

	var m winMsg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// navState turns navigation failures into status pages, so the window never
// stays blank.
type navState struct {
	start  string // proxy start URL, behind the reconnect button
	target string // BMC address shown on status pages
	// certPending: a navigation failed on a certificate error and the
	// certificate event that decides it has not arrived yet.
	certPending  bool
	lastRejected time.Time
	rejected     string
}

var nav navState

func (n *navState) show(pg page) {
	html := pg.render()
	post(func() { browser.NavigateToString(html) })
}

func (n *navState) completed(ok bool, status int32) {
	if ok {
		n.certPending = false
		return
	}
	reportEvent("navigation-failed", webErrorName(status))
	switch {
	case status >= webErrorCertCommonName && status <= webErrorCertInvalid:
		// WebView2 reports the failed navigation before asking the
		// certificate handler; an allowed certificate resumes it.
		if time.Since(n.lastRejected) < 5*time.Second {
			n.showCertRejected()
		} else {
			n.certPending = true
		}
	case isNetworkError(status):
		n.show(page{
			Tone:    "error",
			Title:   "无法打开页面",
			Message: networkErrorMessage(status),
			Target:  n.target,
			Hints:   []string{"确认 BMC 在线且本机能访问其管理网络，然后重新连接。"},
			Detail:  "WebView2 WebErrorStatus: " + webErrorName(status),
			Retry:   n.start,
		})
	}
}

// certDecision is called by the certificate handler.
func (n *navState) certDecision(allowed bool, uri, fingerprint string) {
	// Query strings can carry one-time tokens; keep them out of the log.
	if u, err := url.Parse(uri); err == nil {
		u.RawQuery, u.Fragment = "", ""
		uri = u.String()
	}
	if allowed {
		reportEvent("certificate-allowed", uri)
		n.certPending = false
		return
	}
	reportEvent("certificate-rejected", uri+" "+fingerprint)
	n.lastRejected = time.Now()
	n.rejected = uri + "\n证书指纹 " + fingerprint
	if n.certPending {
		n.certPending = false
		n.showCertRejected()
	}
}

func (n *navState) showCertRejected() {
	n.show(page{
		Tone:    "warning",
		Title:   "页面证书未通过校验",
		Message: "该页面使用的 HTTPS 证书不是此 BMC 已固定的证书，为安全起见已拒绝打开。",
		Target:  n.target,
		Hints: []string{
			"如果 BMC 更换过证书，请在设备详情中点击“信任当前证书”，然后重新打开。",
		},
		Detail: n.rejected,
		Retry:  n.start,
	})
}

// COREWEBVIEW2_WEB_ERROR_STATUS values.
const (
	webErrorCertCommonName = 1
	webErrorCertInvalid    = 5
	webErrorTimeout        = 7
	webErrorCanceled       = 14
)

var webErrorNames = []string{
	"UNKNOWN", "CERTIFICATE_COMMON_NAME_IS_INCORRECT", "CERTIFICATE_EXPIRED",
	"CLIENT_CERTIFICATE_CONTAINS_ERRORS", "CERTIFICATE_REVOKED", "CERTIFICATE_IS_INVALID",
	"SERVER_UNREACHABLE", "TIMEOUT", "ERROR_HTTP_INVALID_SERVER_RESPONSE", "CONNECTION_ABORTED",
	"CONNECTION_RESET", "DISCONNECTED", "CANNOT_CONNECT", "HOST_NAME_NOT_RESOLVED",
	"OPERATION_CANCELED", "REDIRECT_FAILED", "UNEXPECTED_ERROR",
	"VALID_AUTHENTICATION_CREDENTIALS_REQUIRED", "VALID_PROXY_AUTHENTICATION_REQUIRED",
}

func webErrorName(status int32) string {
	if status >= 0 && int(status) < len(webErrorNames) {
		return fmt.Sprintf("%d %s", status, webErrorNames[status])
	}
	return strconv.Itoa(int(status))
}

// isNetworkError reports failures that leave no page from the server. HTTP
// error responses (UNKNOWN), cancelled navigations (including downloads)
// and authentication prompts keep what WebView2 shows.
func isNetworkError(status int32) bool {
	return status > webErrorCertInvalid && status <= 16 && status != webErrorCanceled
}

func networkErrorMessage(status int32) string {
	if status == webErrorTimeout {
		return "页面加载超时。"
	}
	return "浏览器无法连接到该页面的服务器。"
}
