package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmcsim"
	"github.com/12312bg/mdbmc/internal/store"
)

func alertTitles(t *testing.T, h *harness) []string {
	t.Helper()
	rows, _, err := h.app.store.ListAlerts(context.Background(), store.AlertQuery{Limit: 500})
	must(t, err)
	var out []string
	for i := len(rows) - 1; i >= 0; i-- {
		out = append(out, rows[i].Severity+" "+rows[i].Title)
	}
	return out
}

func wantAlerts(t *testing.T, h *harness, want ...string) {
	t.Helper()
	if got := alertTitles(t, h); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("alerts:\n got  %q\n want %q", got, want)
	}
}

func TestAlertTransitions(t *testing.T) {
	h := newHarness(t)
	w := h.app.alerts
	d := &store.Device{ID: 1, Name: "web01", Host: "10.0.0.5"}

	// Health: alerts when a problem appears or gets worse, and once when it clears.
	w.observe(d, true, "on", "ok")
	w.observe(d, true, "on", "warning")
	w.observe(d, true, "on", "warning")
	w.observe(d, true, "on", "critical")
	w.observe(d, true, "on", "warning")
	w.observe(d, true, "on", "unknown")
	w.observe(d, true, "on", "ok")
	wantAlerts(t, h, "warning 健康状态：警告", "critical 健康状态：严重", "ok 健康状态已恢复正常")

	// Offline after two misses in a row; one miss is not enough.
	must(t, h.app.store.PruneAlerts(context.Background(), 0))
	w.observe(d, false, "", "")
	w.observe(d, true, "", "")
	w.observe(d, false, "", "")
	w.observe(d, false, "", "")
	w.observe(d, false, "", "")
	w.observe(d, true, "", "")
	wantAlerts(t, h, "critical BMC 无法连接", "ok BMC 已恢复连接")

	// Power: off without a power action from here is unexpected.
	must(t, h.app.store.PruneAlerts(context.Background(), 0))
	w.observe(d, true, "off", "")
	w.observe(d, true, "off", "")
	w.observe(d, true, "on", "")
	w.notePowerAction([]int64{d.ID})
	w.observe(d, true, "off", "")
	w.observe(d, true, "on", "")
	wantAlerts(t, h, "critical 意外断电", "ok 已重新开机")
}

func TestAlertStateSurvivesRestart(t *testing.T) {
	h := newHarness(t)
	// Last stored status: already warning, unreachable and powered on.
	d := &store.Device{ID: 2, Name: "db01", Host: "10.0.0.6", Health: "warning", PowerState: "on", Reachable: true, CheckedAt: 1}
	h.app.alerts.observe(d, true, "on", "warning")
	wantAlerts(t, h)
	h.app.alerts.observe(d, true, "on", "ok")
	wantAlerts(t, h, "ok 健康状态已恢复正常")

	// Known offline before: no new offline alert, but the recovery.
	d2 := &store.Device{ID: 3, Name: "db02", Host: "10.0.0.7", CheckedAt: 1, Reachable: false}
	h.app.alerts.observe(d2, false, "", "")
	h.app.alerts.observe(d2, true, "", "")
	wantAlerts(t, h, "ok 健康状态已恢复正常", "ok BMC 已恢复连接")
}

func TestAlertSettingsAndInbox(t *testing.T) {
	h := newHarness(t)
	alerts := &AlertService{a: h.app}
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	must(t, h.admin.CreateUser(UserInput{Username: "ro", Role: "viewer", Password: "Viewer@123"}))

	for _, bad := range []AlertConfig{{HistoryMax: 10}, {HistoryMax: 1000, WebhookURL: "ftp://x"}} {
		if err := alerts.SaveConfig(bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	// Health alerts off, a short history.
	must(t, alerts.SaveConfig(AlertConfig{Offline: true, PowerLoss: true, HistoryMax: 100}))
	cfg, err := alerts.GetConfig()
	must(t, err)
	if cfg.Health || !cfg.Offline || cfg.HistoryMax != 100 {
		t.Fatalf("config %+v", cfg)
	}
	d := &store.Device{ID: 9, Name: "x", Host: "10.0.0.9"}
	h.app.alerts.observe(d, true, "on", "critical")
	wantAlerts(t, h)
	for i := 0; i < 70; i++ {
		h.app.alerts.observe(d, true, "on", "")
		h.app.alerts.observe(d, true, "off", "")
	}
	if n := len(alertTitles(t, h)); n != 100 {
		t.Fatalf("%d alerts kept, want 100", n)
	}

	// Each user has their own read marker and pop-up choice.
	in, err := alerts.Inbox()
	must(t, err)
	if in.Unread != 100 || !in.Popup {
		t.Fatalf("admin inbox %+v", in)
	}
	must(t, alerts.MarkAllRead())
	must(t, alerts.SetPopup(false))
	h.app.alerts.observe(d, true, "on", "")
	page, err := alerts.List(AlertQuery{Page: 1, Size: 5})
	must(t, err)
	if page.Unread != 1 || page.Total != 100 || !page.Items[0].Unread || page.Items[1].Unread || page.Items[0].Title != "已重新开机" {
		t.Fatalf("admin page %+v", page)
	}
	if in, _ := alerts.Inbox(); in.Popup {
		t.Fatal("pop-up preference not saved")
	}
	if page, _ := alerts.List(AlertQuery{Severity: "ok", Keyword: "重新开机"}); page.Total != 50 {
		t.Fatalf("filtered total %d", page.Total)
	}

	h.authS.Logout()
	_, err = h.authS.Login("ro", "Viewer@123")
	must(t, err)
	must(t, h.authS.ChangePassword("Viewer@123", "Viewer@456"))
	in, err = alerts.Inbox()
	must(t, err)
	if in.Unread != 100 || !in.Popup {
		t.Fatalf("viewer inbox %+v", in)
	}
	if _, err := alerts.List(AlertQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := alerts.GetConfig(); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer read alert settings: %v", err)
	}
	if err := alerts.SaveConfig(AlertConfig{HistoryMax: 100}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer saved alert settings: %v", err)
	}
}

func TestAlertWebhook(t *testing.T) {
	old := webhookDelays
	webhookDelays = []time.Duration{0, 10 * time.Millisecond}
	t.Cleanup(func() { webhookDelays = old })

	var mu sync.Mutex
	var got []webhookPayload
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte("s3cret"))
		mac.Write([]byte(r.Header.Get("X-Mdbmc-Timestamp") + "."))
		mac.Write(body)
		if r.Header.Get("X-Mdbmc-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if fail {
			fail = false
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		var p webhookPayload
		_ = json.Unmarshal(body, &p)
		got = append(got, p)
	}))
	t.Cleanup(srv.Close)

	h := newHarness(t)
	alerts := &AlertService{a: h.app}
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	cfg := AlertConfig{Offline: true, Health: true, PowerLoss: true, HistoryMax: 1000, WebhookURL: srv.URL, WebhookSecret: "s3cret"}
	if err := alerts.TestWebhook(AlertConfig{WebhookURL: srv.URL, WebhookSecret: "wrong"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("wrong secret: %v", err)
	}
	must(t, alerts.TestWebhook(cfg))
	must(t, alerts.SaveConfig(cfg))

	mu.Lock()
	fail = true // the first delivery is retried
	mu.Unlock()
	d := &store.Device{ID: 4, Name: "gpu01", Host: "10.0.0.8", Vendor: "supermicro", Model: "SYS-420GP"}
	h.app.alerts.observe(d, true, "on", "critical")
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("webhook calls: %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	test, alert := got[0], got[1]
	if test.Event != "test" || alert.Event != "alert" || alert.Severity != "critical" || alert.Recovered ||
		alert.Device.Name != "gpu01" || alert.Device.Vendor != "Supermicro" || alert.Text != "[严重] gpu01 (10.0.0.8)：健康状态：严重" {
		t.Fatalf("payloads %+v", got)
	}
	for !strings.Contains(h.app.alerts.lastWebhook(), "推送成功") {
		time.Sleep(10 * time.Millisecond)
	}
}

// With nobody logged in, BMCs are still probed for being online.
func TestAlertProbesWhileLoggedOut(t *testing.T) {
	sim, err := bmcsim.Start(bmcsim.Options{Index: 8})
	must(t, err)
	h := newHarness(t)
	_, err = h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	h.app.poller.stop()
	_, err = h.devices.Create(DeviceInput{Name: "node08", Host: "127.0.0.1", IPMIPort: sim.IPMIPort, HTTPSPort: sim.HTTPSPort, Username: "ADMIN", Password: "ADMIN"})
	must(t, err)
	h.authS.Logout()

	ctx := context.Background()
	h.app.alerts.probeAll(ctx)
	wantAlerts(t, h)
	sim.Close()
	h.app.alerts.probeAll(ctx)
	h.app.alerts.probeAll(ctx)
	wantAlerts(t, h, "critical BMC 无法连接")
}
