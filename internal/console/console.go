// Package console opens BMC web and KVM pages in separate WebView2 windows.
//
// Each window runs in a child process (the same executable started with
// the --console flag) so that it can use its own browser profile and accept
// the BMC's pinned self-signed certificate without affecting the main window. The
// launch request, including the BMC password for auto-login, is passed over
// the child's stdin and never appears on the command line. The child reports
// back on stdout once its window is on screen, and on stderr if it fails.
package console

import (
	"bufio"
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/bmc"
)

// Flag is the command-line switch that starts a console window process.
const Flag = "--console"

// Lines the child writes to stdout.
const (
	readyLine   = "ready"
	eventPrefix = "event "
)

// readyTimeout bounds how long a launch waits for the window to appear.
// WebView2 can take a few seconds on its very first start.
const readyTimeout = 20 * time.Second

// Request describes one console window.
type Request struct {
	Title string         `json:"title"`
	Spec  bmc.LaunchSpec `json:"spec"`
	// Profile is the WebView2 user data folder, one per device so cookies
	// and BMC sessions stay separate.
	Profile string `json:"profile"`
	// CertFingerprint is the hex SHA-256 of the pinned BMC certificate. The
	// window accepts this self-signed certificate and refuses every other
	// certificate error.
	CertFingerprint string `json:"certFingerprint"`
}

// Launcher opens a console window.
type Launcher func(Request) error

// ProcessLauncher starts a console window process and returns once its
// window is visible, or with the child's error if it could not start.
func ProcessLauncher(req Request) error {
	_, err := startWindow(req, readyTimeout)
	return err
}

// windowProc is a running console window process.
type windowProc struct {
	cmd *exec.Cmd
	// events receives "name\tdetail" lines reported by the page script.
	events chan string
	done   chan struct{}
}

func (p *windowProc) kill() {
	_ = p.cmd.Process.Kill()
	<-p.done
}

// childEnv drops the WEBVIEW2_* variables. The main window's WebView2 loader
// sets several of them to empty strings to block overrides; the stock loader
// in the child treats a present-but-empty variable (for example the browser
// folder) as configured and then fails to create the browser.
func childEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "WEBVIEW2_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func startWindow(req Request, timeout time.Duration) (*windowProc, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// The child is a GUI program: it must not be started hidden, since its
	// first ShowWindow call would then use the hidden state we passed.
	cmd := exec.Command(exe, Flag)
	cmd.Env = childEnv(os.Environ())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tailBuffer{max: 32 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动控制台窗口失败: %w", err)
	}
	err = json.NewEncoder(stdin).Encode(req)
	stdin.Close()
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("向控制台窗口传递参数失败: %w", err)
	}

	p := &windowProc{cmd: cmd, events: make(chan string, 64), done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		defer close(p.done)
		gotReady := false
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case !gotReady && line == readyLine:
				gotReady = true
				ready <- nil
			case strings.HasPrefix(line, eventPrefix):
				select {
				case p.events <- strings.TrimPrefix(line, eventPrefix):
				default:
				}
			}
		}
		waitErr := cmd.Wait()
		msg := strings.TrimSpace(stderr.String())
		if !gotReady {
			if msg == "" && waitErr != nil {
				msg = waitErr.Error()
			}
			if msg == "" {
				msg = "窗口进程意外退出"
			}
			ready <- fmt.Errorf("控制台窗口启动失败：%s", msg)
			return
		}
		if msg != "" {
			log.Printf("console window %q: %s", req.Title, msg)
		}
	}()

	select {
	case err := <-ready:
		if err != nil {
			<-p.done
			return nil, err
		}
		return p, nil
	case <-time.After(timeout):
		log.Printf("console window %q has not reported ready after %s", req.Title, timeout)
		return p, nil
	}
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Write(p)
	if over := b.buf.Len() - b.max; over > 0 {
		b.buf.Next(over)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

//go:embed autologin.js
var autologinJS string

// initScript renders the script injected into every page of the window.
func initScript(spec bmc.LaunchSpec) (string, error) {
	u, err := url.Parse(spec.URL)
	if err != nil || u.Host == "" {
		return "", errors.New("无效的控制台地址")
	}
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	cfg := map[string]any{
		// location.host is lower case; host names are case-insensitive.
		"host":      strings.ToLower(u.Host),
		"autologin": spec.AutoLogin && spec.Username != "",
		"user":      spec.Username,
		"pass":      spec.Password,
		"after":     spec.AfterLoginURL,
		"nonce":     hex.EncodeToString(nonce),
		"hint":      spec.Hint,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return strings.Replace(autologinJS, "__MDBMC_CONFIG__", string(b), 1), nil
}
