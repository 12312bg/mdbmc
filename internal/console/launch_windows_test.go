//go:build windows

package console

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
// profileDir is a temporary WebView2 profile. WebView2's browser processes
// can outlive the window for a moment and keep writing to the profile, so
// removing it is retried, and a profile still in use is only logged.
func profileDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mdbmc-profile-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for i := 0; ; i++ {
			err := os.RemoveAll(dir)
			if err == nil {
				return
			}
			if i == 50 {
				t.Logf("profile not removed: %v", err)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
	return dir
}

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
	// A Java console like AMI's JViewer: the jar is built by the Java subtest.
	var probeJar []byte
	var probeOut string
	mux.HandleFunc("GET /Java/jviewer.jnlp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-java-jnlp-file")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><jnlp codebase="https://%s/Java"><information><title>JViewer</title></information>
<resources><jar href="release/probe.jar"/><property name="probe.out" value="%s"/></resources>
<application-desc><argument>-hostname</argument><argument>%s</argument><argument>-kvmtoken</argument><argument>t0ken</argument></application-desc></jnlp>`,
			r.Host, html.EscapeString(probeOut), html.EscapeString(r.URL.Query().Get("EXTRNIP")))
	})
	mux.HandleFunc("GET /Java/release/probe.jar", func(w http.ResponseWriter, _ *http.Request) { w.Write(probeJar) })
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

	openAt := func(t *testing.T, fingerprint, path string) (*windowProc, chan string) {
		t.Helper()
		events := make(chan string, 256)
		p, err := startWindow(Request{
			Title:           "MDBMC smoke test",
			Spec:            bmc.LaunchSpec{URL: srv.URL + path, AutoLogin: true, Username: "ADMIN", Password: `p"w&1`},
			Profile:         profileDir(t),
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

	open := func(t *testing.T, fingerprint string) (*windowProc, chan string) {
		t.Helper()
		return openAt(t, fingerprint, "/")
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

	t.Run("CloseAll closes the windows (logout)", func(t *testing.T) {
		p, _ := open(t, pinned)
		CloseAll()
		select {
		case <-p.done:
		case <-time.After(15 * time.Second):
			t.Fatal("the window is still open 15s after CloseAll")
		}
	})

	t.Run("window closes when the main program exits", func(t *testing.T) {
		// The helper plays the main program: it opens a window and exits.
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		helper := exec.Command(exe, parentHelper)
		helper.Env = append(os.Environ(), "MDBMC_TEST_URL="+srv.URL+"/", "MDBMC_TEST_FP="+pinned, "MDBMC_TEST_PROFILE="+profileDir(t))
		out, err := helper.Output()
		if err != nil {
			t.Fatalf("helper: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			t.Fatalf("helper output %q", out)
		}
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			return // already gone
		}
		defer windows.CloseHandle(h)
		if ev, _ := windows.WaitForSingleObject(h, 15000); ev != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(h, 1)
			t.Fatal("the console window process outlived the main program")
		}
	})

	t.Run("Java console from a JNLP, closed with its window", func(t *testing.T) {
		jdk := os.Getenv("JAVA_HOME_8_X64")
		if os.Getenv(javaHomeEnv) == "" || jdk == "" {
			t.Skip("needs the bundled Java runtime (" + javaHomeEnv + ") and a JDK 8 (JAVA_HOME_8_X64) for the probe")
		}
		dir := t.TempDir()
		src := filepath.Join(dir, "Probe.java")
		if err := os.WriteFile(src, []byte(probeSource), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(filepath.Join(jdk, "bin", "javac.exe"), "-d", dir, src).CombinedOutput(); err != nil {
			t.Fatalf("javac: %v\n%s", err, out)
		}
		class, err := os.ReadFile(filepath.Join(dir, "Probe.class"))
		if err != nil {
			t.Fatal(err)
		}
		probeJar = makeJar(t, "META-INF/MANIFEST.MF", "Manifest-Version: 1.0\r\nMain-Class: Probe\r\n", "Probe.class", string(class))
		probeOut = filepath.Join(dir, "probe.out")

		p, events := openAt(t, pinned, "/Java/jviewer.jnlp?EXTRNIP=127.0.0.1&JNLPSTR=JViewer")
		var lines []string
		for deadline := time.Now().Add(90 * time.Second); lines == nil; {
			select {
			case ev := <-events:
				t.Logf("event: %s", ev)
			case <-p.done:
				t.Fatal("window process exited")
			case <-time.After(300 * time.Millisecond):
			}
			if b, err := os.ReadFile(probeOut); err == nil {
				lines = strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r", "")), "\n")
			} else if time.Now().After(deadline) {
				t.Fatal("the Java console did not start within 90s")
			}
		}
		t.Logf("probe: %q", lines)
		if got := strings.Join(lines[1:], " "); got != "-hostname 127.0.0.1 -kvmtoken t0ken" {
			t.Fatalf("Java console got arguments %q", got)
		}
		pid, err := strconv.Atoi(strings.Split(lines[0], "@")[0])
		if err != nil {
			t.Fatalf("probe output %q", lines)
		}
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			t.Fatalf("Java process %d: %v", pid, err)
		}
		defer windows.CloseHandle(h)
		CloseAll()
		if ev, _ := windows.WaitForSingleObject(h, 15000); ev != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(h, 1)
			t.Fatal("the Java console outlived its window")
		}
	})

	t.Run("other certificate is refused", func(t *testing.T) {
		_, events := open(t, strings.Repeat("00", 32))
		refused, shown := false, false
		deadline := time.After(20 * time.Second)
		for {
			select {
			case ev := <-events:
				t.Logf("event: %s", ev)
				switch {
				case strings.HasPrefix(ev, "loaded"):
					t.Fatalf("BMC page loaded although its certificate is not pinned: %s", ev)
				case strings.HasPrefix(ev, "proxy-error") && strings.Contains(ev, "不一致"):
					refused = true
				case strings.HasPrefix(ev, "status-page"):
					shown = true // the proxy's error page, not a blank window
				}
			case got := <-logins:
				t.Fatalf("credentials sent to a server with an unpinned certificate: %q", got)
			case <-deadline:
				if !refused || !shown {
					t.Fatalf("certificate mismatch reported=%v, error page shown=%v", refused, shown)
				}
				return
			}
		}
	})
}

// probeSource stands in for a BMC's Java console: it records its process
// id and arguments, then runs until it is killed.
const probeSource = `public class Probe {
  public static void main(String[] args) throws Exception {
    java.io.File out = new java.io.File(System.getProperty("probe.out"));
    java.io.File tmp = new java.io.File(out.getPath() + ".tmp");
    java.io.PrintWriter w = new java.io.PrintWriter(tmp, "UTF-8");
    w.println(java.lang.management.ManagementFactory.getRuntimeMXBean().getName());
    for (String a : args) w.println(a);
    w.close();
    tmp.renameTo(out);
    Thread.sleep(600000);
  }
}
`

// parentHelper makes the test binary open a console window, print the
// window process's PID and exit, like the main program being closed.
const parentHelper = "--console-test-parent"

func init() {
	testHelpers[parentHelper] = func() {
		p, err := startWindow(Request{
			Title:           "MDBMC parent exit test",
			Spec:            bmc.LaunchSpec{URL: os.Getenv("MDBMC_TEST_URL")},
			Profile:         os.Getenv("MDBMC_TEST_PROFILE"),
			CertFingerprint: os.Getenv("MDBMC_TEST_FP"),
		}, 60*time.Second, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(p.cmd.Process.Pid)
		os.Exit(0)
	}
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
