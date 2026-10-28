//go:build windows

package console

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/12312bg/mdbmc/internal/bmc"
)

const loginPage = `<!doctype html><html><body>
<form action="/cgi/login.cgi" method="post">
<input type="text" name="name"><input type="password" name="pwd">
<button type="submit">Login</button></form></body></html>`

// TestConsoleWindowSmoke opens a real WebView2 window against a local HTTPS
// server with a self-signed certificate. It needs a desktop session and the
// WebView2 runtime, so it only runs with MDBMC_UI_TEST=1 (set in CI).
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
		fmt.Fprint(w, "<html><body>welcome</body></html>")
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	sum := sha256.Sum256(srv.Certificate().Raw)
	pinned := hex.EncodeToString(sum[:])

	open := func(t *testing.T, fingerprint string) *windowProc {
		t.Helper()
		p, err := startWindow(Request{
			Title:           "MDBMC smoke test",
			Spec:            bmc.LaunchSpec{URL: srv.URL + "/", AutoLogin: true, Username: "ADMIN", Password: `p"w&1`},
			Profile:         t.TempDir(),
			CertFingerprint: fingerprint,
		}, 60*time.Second)
		if err != nil {
			t.Fatalf("window did not start: %v", err)
		}
		t.Cleanup(p.kill)
		return p
	}

	t.Run("pinned certificate, auto-login", func(t *testing.T) {
		p := open(t, pinned)
		deadline := time.After(90 * time.Second)
		for {
			select {
			case ev := <-p.events:
				t.Logf("event: %s", ev)
			case got := <-logins:
				if got != `ADMIN:p"w&1` {
					t.Fatalf("login form submitted %q", got)
				}
				return
			case <-p.done:
				t.Fatal("window process exited before logging in")
			case <-deadline:
				t.Fatal("no login within 90s: page not loaded (certificate refused?) or auto-login failed")
			}
		}
	})

	t.Run("other certificate is refused", func(t *testing.T) {
		wrong := strings.Repeat("00", 32)
		p := open(t, wrong)
		deadline := time.After(20 * time.Second)
		for {
			select {
			case ev := <-p.events:
				if strings.HasPrefix(ev, "loaded") {
					t.Fatalf("page loaded although its certificate is not pinned: %s", ev)
				}
			case got := <-logins:
				t.Fatalf("credentials sent to a server with an unpinned certificate: %q", got)
			case <-deadline:
				return
			}
		}
	})
}
