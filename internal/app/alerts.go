package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/store"
)

// Alert kinds and severities.
const (
	alertOffline = "offline" // BMC not answering on the network
	alertHealth  = "health"  // health rollup warning or critical
	alertPower   = "power"   // powered off without a power action from this program

	severityCritical = "critical"
	severityWarning  = "warning"
	severityOK       = "ok" // recovered
)

const (
	// A BMC is offline after this many failed checks in a row.
	offlineAfter = 2
	// A power-off within this time of a power action sent from this program
	// is expected.
	powerActionGrace = 10 * time.Minute
	probeTimeout     = 5 * time.Second
	firstProbe       = 15 * time.Second
)

// alertWatcher turns device status changes into alerts. The poller feeds it
// while someone is logged in; otherwise it probes the BMCs itself, which
// needs no credentials but only tells whether they are online.
type alertWatcher struct {
	a *App

	mu      sync.Mutex
	state   map[int64]*watchState
	powerOp map[int64]time.Time // last power action sent from this program

	hookMu   sync.Mutex
	lastHook string // outcome of the last webhook delivery

	stop context.CancelFunc
}

// watchState is what was last alerted for a device.
type watchState struct {
	misses   int    // failed checks in a row
	offline  bool   // offline alert raised
	health   string // warning or critical alert raised, "" when healthy
	power    string // last known power state (on/off)
	powerOff bool   // unexpected power-off alert raised
}

func newAlertWatcher(a *App) *alertWatcher {
	return &alertWatcher{a: a, state: map[int64]*watchState{}, powerOp: map[int64]time.Time{}}
}

// stateFor returns the device's state, starting from its last stored status
// so that a restart does not alert again about known problems.
func (w *alertWatcher) stateFor(d *store.Device) *watchState {
	st := w.state[d.ID]
	if st == nil {
		st = &watchState{}
		if d.CheckedAt > 0 && !d.Reachable {
			st.offline, st.misses = true, offlineAfter
		}
		if d.Health == string(bmc.HealthWarning) || d.Health == string(bmc.HealthCritical) {
			st.health = d.Health
		}
		if d.PowerState == string(bmc.PowerOn) || d.PowerState == string(bmc.PowerOff) {
			st.power = d.PowerState
		}
		w.state[d.ID] = st
	}
	return st
}

// notePowerAction records power actions sent from this program, whose
// power-offs are expected.
func (w *alertWatcher) notePowerAction(ids []int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, id := range ids {
		w.powerOp[id] = time.Now()
	}
}

// observe takes one check of device d: whether its BMC answered on the
// network, and the power state and health when they could be read ("" when
// not).
func (w *alertWatcher) observe(d *store.Device, online bool, power, health string) {
	cfg := w.a.alertConfig()
	var out []*store.Alert
	add := func(enabled bool, kind, severity, title, detail string) {
		if enabled {
			out = append(out, &store.Alert{DeviceID: d.ID, DeviceName: d.Name, Host: d.Host,
				Kind: kind, Severity: severity, Title: title, Detail: detail})
		}
	}

	w.mu.Lock()
	st := w.stateFor(d)
	if online {
		st.misses = 0
		if st.offline {
			st.offline = false
			add(cfg.Offline, alertOffline, severityOK, "BMC 已恢复连接", "")
		}
	} else {
		st.misses++
		if st.misses >= offlineAfter && !st.offline {
			st.offline = true
			add(cfg.Offline, alertOffline, severityCritical, "BMC 无法连接", fmt.Sprintf("连续 %d 次检查都没有响应。", offlineAfter))
		}
	}

	switch health {
	case string(bmc.HealthWarning), string(bmc.HealthCritical):
		// Alert when a problem appears or gets worse.
		if st.health == "" || (st.health == string(bmc.HealthWarning) && health == string(bmc.HealthCritical)) {
			add(cfg.Health, alertHealth, health, map[string]string{"warning": "健康状态：警告", "critical": "健康状态：严重"}[health],
				"BMC 报告的健康汇总状态异常，请查看传感器和事件日志。")
		}
		st.health = health
	case string(bmc.HealthOK):
		if st.health != "" {
			st.health = ""
			add(cfg.Health, alertHealth, severityOK, "健康状态已恢复正常", "")
		}
	}

	switch power {
	case string(bmc.PowerOff):
		if st.power == string(bmc.PowerOn) && !st.powerOff && time.Since(w.powerOp[d.ID]) > powerActionGrace {
			st.powerOff = true
			add(cfg.PowerLoss, alertPower, severityCritical, "意外断电", "设备从开机变为关机，期间没有人在本软件中执行电源操作。")
		}
		st.power = power
	case string(bmc.PowerOn):
		if st.powerOff {
			st.powerOff = false
			add(cfg.PowerLoss, alertPower, severityOK, "已重新开机", "")
		}
		st.power = power
	}
	w.mu.Unlock()

	for _, al := range out {
		w.raise(cfg, d, al)
	}
}

// raise stores an alert, shows it in the inbox and sends it to the webhook.
func (w *alertWatcher) raise(cfg AlertConfig, d *store.Device, al *store.Alert) {
	ctx := context.Background()
	if err := w.a.store.AddAlert(ctx, al); err != nil {
		log.Printf("alert: save: %v", err)
		return
	}
	if err := w.a.store.PruneAlerts(ctx, cfg.HistoryMax); err != nil {
		log.Printf("alert: prune: %v", err)
	}
	log.Printf("alert: %s (%s): %s", al.DeviceName, al.Host, al.Title)
	w.a.emit("alert:new", alertDTO(al, false))
	if cfg.WebhookURL != "" {
		go w.deliver(cfg, newWebhookPayload("alert", al, d))
	}
}

// webhookPayload is the JSON posted to the webhook.
type webhookPayload struct {
	Event     string        `json:"event"` // "alert", or "test" from the settings page
	ID        int64         `json:"id"`
	Time      string        `json:"time"` // RFC 3339
	Kind      string        `json:"kind"`
	Severity  string        `json:"severity"`
	Recovered bool          `json:"recovered"`
	Title     string        `json:"title"`
	Detail    string        `json:"detail"`
	Device    webhookDevice `json:"device"`
	// Text is the alert in one line, ready for chat tools.
	Text string `json:"text"`
}

type webhookDevice struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Host   string `json:"host"`
	Vendor string `json:"vendor"`
	Model  string `json:"model"`
}

var severityLabel = map[string]string{severityCritical: "严重", severityWarning: "警告", severityOK: "恢复"}

func newWebhookPayload(event string, al *store.Alert, d *store.Device) webhookPayload {
	p := webhookPayload{
		Event: event, ID: al.ID, Time: time.Unix(al.TS, 0).Format(time.RFC3339), Kind: al.Kind,
		Severity: al.Severity, Recovered: al.Severity == severityOK, Title: al.Title, Detail: al.Detail,
		Device: webhookDevice{ID: al.DeviceID, Name: al.DeviceName, Host: al.Host},
	}
	if d != nil {
		p.Device.Vendor, p.Device.Model = bmc.VendorName(d.Vendor), d.Model
	}
	p.Text = fmt.Sprintf("[%s] %s：%s", severityLabel[al.Severity], deviceLabel(&store.Device{Name: al.DeviceName, Host: al.Host}), al.Title)
	return p
}

// webhookDelays are the waits before each delivery attempt.
var webhookDelays = []time.Duration{0, 5 * time.Second, 30 * time.Second}

func (w *alertWatcher) deliver(cfg AlertConfig, p webhookPayload) {
	var err error
	for _, delay := range webhookDelays {
		time.Sleep(delay)
		if err = postWebhook(cfg.WebhookURL, cfg.WebhookSecret, p); err == nil {
			break
		}
	}
	at := time.Now().Format("2006-01-02 15:04:05")
	w.hookMu.Lock()
	if err != nil {
		log.Printf("alert: webhook: %v", err)
		w.lastHook = at + " 推送失败：" + err.Error()
	} else {
		w.lastHook = at + " 推送成功"
	}
	w.hookMu.Unlock()
}

func (w *alertWatcher) lastWebhook() string {
	w.hookMu.Lock()
	defer w.hookMu.Unlock()
	return w.lastHook
}

// postWebhook posts p as JSON. With a secret, X-Mdbmc-Signature carries
// "sha256=" and the hex HMAC-SHA256 of "<X-Mdbmc-Timestamp>.<body>".
func postWebhook(url, secret string, p webhookPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "mdbmc/"+Version)
	if secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(ts + "."))
		mac.Write(body)
		req.Header.Set("X-Mdbmc-Timestamp", ts)
		req.Header.Set("X-Mdbmc-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return nil
}

// probe tells whether d's BMC answers on the network, without credentials.
func (w *alertWatcher) probe(ctx context.Context, d *store.Device) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	ipmi := d.HasIPMI || d.Protocol != string(bmc.ProtocolRedfish)
	https := d.HasRedfish || d.Protocol != string(bmc.ProtocolIPMI)
	return bmc.Probe(ctx, d.Host, d.IPMIPort, d.HTTPSPort, ipmi, https)
}

// start runs the checks for when nobody is logged in until Shutdown.
func (w *alertWatcher) start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.stop = cancel
	go w.probeLoop(ctx)
}

func (w *alertWatcher) probeLoop(ctx context.Context) {
	timer := time.NewTimer(firstProbe)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		interval := time.Duration(w.a.Settings().PollIntervalSec) * time.Second
		if interval > 0 && w.a.auth.Current() == nil && w.a.alertConfig().Offline {
			w.probeAll(ctx)
		}
		if interval <= 0 {
			interval = time.Minute // status checks are off; look at the setting again later
		}
		timer.Reset(interval)
	}
}

func (w *alertWatcher) probeAll(ctx context.Context) {
	devs, err := w.a.store.ListDevices(ctx)
	if err != nil {
		log.Printf("alert: list devices: %v", err)
		return
	}
	sem := make(chan struct{}, max(w.a.Settings().MaxConcurrency, 1))
	var wg sync.WaitGroup
	for _, d := range devs {
		wg.Add(1)
		go func(d *store.Device) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			online := w.probe(ctx, d)
			// Someone logged in meanwhile: the poller has taken over.
			if ctx.Err() == nil && w.a.auth.Current() == nil {
				w.observe(d, online, "", "")
			}
		}(d)
	}
	wg.Wait()
}
