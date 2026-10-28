//go:build windows

package console

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/12312bg/mdbmc/internal/bmc"
)

// The pages' background lets the test check that they are really painted.
const (
	pageColor = "#1565c0"
	loginPage = `<!doctype html><html><body style="margin:0;background:` + pageColor + `">
<form action="/cgi/login.cgi" method="post">
<input type="text" name="name"><input type="password" name="pwd">
<button type="submit">Login</button></form></body></html>`
)

// TestConsoleWindowSmoke opens real WebView2 windows against a local BMC-like
// HTTPS server that, like old BMC firmware, only speaks TLS 1.0 with RSA key
// exchange. WebView2 refuses that server, so the page can only appear through
// the window's local proxy. It needs a desktop session and the WebView2
// runtime, so it only runs with MDBMC_UI_TEST=1 (set in CI).
func TestConsoleWindowSmoke(t *testing.T) {
	if os.Getenv("MDBMC_UI_TEST") == "" {
		t.Skip("set MDBMC_UI_TEST=1 to open a real WebView2 window")
	}
	logins := make(chan string, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, loginPage)
	})
	mux.HandleFunc("POST /cgi/login.cgi", func(w http.ResponseWriter, r *http.Request) {
		logins <- r.FormValue("name") + ":" + r.FormValue("pwd")
		fmt.Fprint(w, `<html><body style="background:`+pageColor+`">welcome</body></html>`)
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS10,
		MaxVersion:   tls.VersionTLS10,
		CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA},
	}
	srv.StartTLS()
	defer srv.Close()
	sum := sha256.Sum256(srv.Certificate().Raw)
	pinned := hex.EncodeToString(sum[:])

	open := func(t *testing.T, fingerprint string) (*windowProc, chan string) {
		t.Helper()
		events := make(chan string, 256)
		p, err := startWindow(Request{
			Title:           "MDBMC smoke test",
			Spec:            bmc.LaunchSpec{URL: srv.URL + "/", AutoLogin: true, Username: "ADMIN", Password: `p"w&1`},
			Profile:         t.TempDir(),
			CertFingerprint: fingerprint,
		}, 60*time.Second, func(ev string) {
			select {
			case events <- ev:
			default:
			}
		})
		if err != nil {
			t.Fatalf("window did not start: %v", err)
		}
		t.Cleanup(p.kill)
		return p, events
	}

	t.Run("legacy TLS BMC through the proxy, auto-login", func(t *testing.T) {
		p, events := open(t, pinned)
		var hwnd uintptr
		deadline := time.After(90 * time.Second)
		for {
			select {
			case ev := <-events:
				t.Logf("event: %s", ev)
				name, detail, _ := strings.Cut(ev, "\t")
				switch name {
				case "window":
					n, _ := strconv.ParseUint(detail, 10, 64)
					hwnd = uintptr(n)
				case "loaded":
					logPaint(t, hwnd)
				}
			case got := <-logins:
				if got != `ADMIN:p"w&1` {
					t.Fatalf("login form submitted %q", got)
				}
				return
			case <-p.done:
				t.Fatal("window process exited before logging in")
			case <-deadline:
				t.Fatal("no login within 90s: page not loaded through the proxy, or auto-login failed")
			}
		}
	})

	t.Run("other certificate is refused", func(t *testing.T) {
		_, events := open(t, strings.Repeat("00", 32))
		refused := false
		deadline := time.After(20 * time.Second)
		for {
			select {
			case ev := <-events:
				t.Logf("event: %s", ev)
				if strings.HasPrefix(ev, "loaded") {
					t.Fatalf("BMC page loaded although its certificate is not pinned: %s", ev)
				}
				if strings.HasPrefix(ev, "proxy-error") && strings.Contains(ev, "不一致") {
					refused = true
				}
			case got := <-logins:
				t.Fatalf("credentials sent to a server with an unpinned certificate: %q", got)
			case <-deadline:
				if !refused {
					t.Fatal("the proxy did not report the certificate mismatch")
				}
				return
			}
		}
	})
}

var (
	gdi32dll                   = windows.NewLazySystemDLL("gdi32.dll")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procGetClientRect          = user32.NewProc("GetClientRect")
	procPrintWindow            = user32.NewProc("PrintWindow")
	procCreateCompatibleDC     = gdi32dll.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32dll.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32dll.NewProc("SelectObject")
	procGetPixel               = gdi32dll.NewProc("GetPixel")
	procDeleteObject           = gdi32dll.NewProc("DeleteObject")
	procDeleteDC               = gdi32dll.NewProc("DeleteDC")
)

// logPaint captures the window and logs the colour in the middle of its
// client area, to tell a painted page from a blank window. It only logs:
// capturing a window on a CI desktop is not reliable enough to fail on.
func logPaint(t *testing.T, hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	time.Sleep(1500 * time.Millisecond) // let the page paint
	var rc struct{ Left, Top, Right, Bottom int32 }
	_, _, _ = procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right-rc.Left, rc.Bottom-rc.Top
	if w <= 0 || h <= 0 {
		t.Logf("paint check: empty client area %dx%d", w, h)
		return
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(mem, bmp)
	defer procSelectObject.Call(mem, old)
	const pwClientOnly, pwRenderFullContent = 1, 2
	ok, _, _ := procPrintWindow.Call(hwnd, mem, pwClientOnly|pwRenderFullContent)
	c, _, _ := procGetPixel.Call(mem, uintptr(w/2), uintptr(h/2))
	got := fmt.Sprintf("#%02x%02x%02x", c&0xff, c>>8&0xff, c>>16&0xff)
	t.Logf("paint check: PrintWindow=%d, %dx%d, centre pixel %s (page background %s, match=%v)", ok, w, h, got, pageColor, got == pageColor)
}
