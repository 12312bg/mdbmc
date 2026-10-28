// Package console opens BMC web and KVM pages in separate WebView2 windows.
//
// Each window runs in a child process (the same executable started with
// the --console flag) so that it can use its own browser profile and accept
// the BMC's self-signed certificate without affecting the main window. The
// launch request, including the BMC password for auto-login, is passed over
// the child's stdin and never appears on the command line.
package console

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/12312bg/mdbmc/internal/bmc"
)

// Flag is the command-line switch that starts a console window process.
const Flag = "--console"

// Request describes one console window.
type Request struct {
	Title string         `json:"title"`
	Spec  bmc.LaunchSpec `json:"spec"`
	// Profile is the WebView2 user data folder, one per device so cookies
	// and BMC sessions stay separate.
	Profile string `json:"profile"`
	// SPKIHash is the public key of the pinned BMC certificate (see
	// bmc.CertInfo). The window accepts this self-signed key and nothing else.
	SPKIHash string `json:"spkiHash"`
}

// browserArgs returns the WebView2 switches that make the window trust the
// pinned BMC key. Without a key hash it falls back to accepting any
// certificate, relying on the probe made just before the launch.
func browserArgs(spkiHash string) string {
	if spkiHash == "" {
		return "--ignore-certificate-errors"
	}
	return "--ignore-certificate-errors-spki-list=" + spkiHash
}

// Launcher opens a console window.
type Launcher func(Request) error

// ProcessLauncher starts a child process of the running executable.
func ProcessLauncher(req Request) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, Flag)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	hideConsole(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动控制台窗口失败: %w", err)
	}
	err = json.NewEncoder(stdin).Encode(req)
	stdin.Close()
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
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
