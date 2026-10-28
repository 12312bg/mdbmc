//go:build windows

package console

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
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
)

const (
	wsOverlappedWindow = 0x00CF0000
	swShow             = 5
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmActivate         = 0x0006
	idcArrow           = 32512
	smCxScreen         = 0
	smCyScreen         = 1
	colorWindow        = 5
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

// browser is used by the window procedure; there is one window per process.
var browser *edge.Chromium

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
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

	var req Request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&req); err != nil {
		fail("控制台启动参数无效: %v", err)
	}
	script, err := initScript(req.Spec)
	if err != nil {
		fail("%v", err)
	}
	if req.Profile != "" {
		_ = os.MkdirAll(req.Profile, 0o700)
	}

	hwnd := createWindow(req.Title, 1366, 900)
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
		if ev, ok := strings.CutPrefix(message, "mdbmc:"); ok {
			fmt.Printf("%s%s\n", eventPrefix, ev)
		}
	}
	browser = c
	if !c.Embed(hwnd) {
		fail("无法创建 WebView2 窗口。请确认已安装 Microsoft Edge WebView2 Runtime。")
	}
	c.Resize()
	installCertPinning(c, req.CertFingerprint)
	c.Init(script)
	c.Navigate(req.Spec.URL)

	_, _, _ = procSetForeground.Call(hwnd)
	c.Focus()
	if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
		fail("控制台窗口已创建但不可见")
	}
	fmt.Println(readyLine)

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
