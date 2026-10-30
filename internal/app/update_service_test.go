package app

import (
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/update"
)

// releaseServer publishes version as the latest release, signed with priv.
func releaseServer(t *testing.T, priv ed25519.PrivateKey, version string) *httptest.Server {
	t.Helper()
	pkg := filepath.Join(t.TempDir(), "mdbmc-amd64-installer.exe")
	must(t, os.WriteFile(pkg, []byte("installer"), 0o600))
	manifest, err := update.BuildManifest(version, "v"+version, "## 新功能\n- 在线更新", time.Now(), map[update.Mode]string{update.ModeInstaller: pkg})
	must(t, err)
	sig := update.Sign(manifest, priv)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest/download/" + update.ManifestName:
			w.Write(manifest)
		case "/releases/latest/download/" + update.SignatureName:
			w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateServicePermissionsAndCheck(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	must(t, err)
	t.Setenv("MDBMC_UPDATE_PUBLIC_KEY", update.EncodeKey(pub))
	h := newHarness(t)
	upd := &UpdateService{a: h.app}
	srv := releaseServer(t, priv, "99.0.0")

	if _, err := upd.Status(); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Fatalf("status before login: %v", err)
	}
	_, err = h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	must(t, h.admin.CreateUser(UserInput{Username: "ro", Role: "viewer", Password: "Viewer@123"}))

	// Configuration is validated and normalized.
	for _, bad := range []UpdateConfig{{CheckHours: -1}, {CheckHours: 24, Source: "ftp://x"}, {CheckHours: 24, Proxy: "ftp://p:1"}} {
		if err := upd.SaveConfig(bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	must(t, upd.SaveConfig(UpdateConfig{CheckHours: 6, Source: srv.URL + "/releases/", Proxy: "10.0.0.1:3128"}))
	cfg, err := upd.GetConfig()
	must(t, err)
	if cfg.CheckHours != 6 || cfg.Source != srv.URL+"/releases" || cfg.Proxy != "http://10.0.0.1:3128" {
		t.Fatalf("config %+v", cfg)
	}
	if c, _ := h.app.loadUpdateConfig(); c.CheckHours != 6 || c.Proxy != "http://10.0.0.1:3128" {
		t.Fatalf("stored config %+v", c)
	}
	must(t, upd.SaveConfig(UpdateConfig{CheckHours: 6, Source: srv.URL + "/releases"}))

	// Every user sees a new version; only administrators act on it.
	h.authS.Logout()
	_, err = h.authS.Login("ro", "Viewer@123")
	must(t, err)
	must(t, h.authS.ChangePassword("Viewer@123", "Viewer@456"))
	st, err := upd.Check()
	must(t, err)
	if st.State != "available" || st.Latest == nil || st.Latest.Version != "99.0.0" || !strings.Contains(st.Latest.Notes, "在线更新") {
		t.Fatalf("status %+v", st)
	}
	if st.CurrentVersion != Version {
		t.Fatalf("current version %q", st.CurrentVersion)
	}
	for name, call := range map[string]func() error{
		"download": upd.Download,
		"cancel":   upd.CancelDownload,
		"install":  upd.Install,
		"config":   func() error { _, err := upd.GetConfig(); return err },
		"save":     func() error { return upd.SaveConfig(UpdateConfig{CheckHours: 1}) },
	} {
		if err := call(); !errors.Is(err, auth.ErrForbidden) {
			t.Errorf("viewer %s: %v", name, err)
		}
	}

	// This test binary cannot install anything.
	h.authS.Logout()
	_, err = h.authS.Login("admin", "Admin@12345")
	must(t, err)
	if st, _ := upd.Status(); st.CanInstall || st.Mode != string(update.ModeUnsupported) {
		t.Fatalf("status %+v", st)
	}
	if err := upd.Download(); err == nil {
		t.Fatal("download on an unsupported system")
	}
	if err := upd.Install(); err == nil {
		t.Fatal("install on an unsupported system")
	}
}

func TestUpdateResultIsReportedOnce(t *testing.T) {
	h := newHarness(t)
	upd := &UpdateService{a: h.app}
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)

	// The helper of a failed update left its result for the next start.
	must(t, os.WriteFile(filepath.Join(h.app.updateDir, "result.json"),
		[]byte(`{"from":"0.1.0","version":"0.2.0","success":false,"error":"没有获得管理员授权","time":1,"user":"admin","userId":1}`), 0o600))
	must(t, os.MkdirAll(filepath.Join(h.app.updateDir, "0.0.9"), 0o700))
	must(t, h.app.initUpdater())

	st, err := upd.Status()
	must(t, err)
	if r := st.LastResult; r == nil || r.Success || r.Version != "0.2.0" || r.Error != "没有获得管理员授权" {
		t.Fatalf("result %+v", st.LastResult)
	}
	if _, err := os.Stat(filepath.Join(h.app.updateDir, "0.0.9")); !os.IsNotExist(err) {
		t.Fatal("download of an older version kept")
	}
	page, err := h.admin.ListAudit(AuditQuery{Keyword: "update.finish"})
	must(t, err)
	if page.Total != 1 || page.Items[0].Success || page.Items[0].Username != "admin" || !strings.Contains(page.Items[0].Detail, "管理员授权") {
		t.Fatalf("audit %+v", page.Items)
	}
	must(t, upd.DismissResult())
	if st, _ := upd.Status(); st.LastResult != nil {
		t.Fatal("result shown again")
	}
}
