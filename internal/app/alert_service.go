package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/store"
)

// AlertService serves the alert inbox to every user and the alert settings
// to administrators.
type AlertService struct{ a *App }

// AlertConfig is the alert settings; administrators only.
type AlertConfig struct {
	Offline       bool   `json:"offline"`    // BMC offline / back online
	Health        bool   `json:"health"`     // health warning or critical / back to normal
	PowerLoss     bool   `json:"powerLoss"`  // unexpected power-off / back on
	HistoryMax    int    `json:"historyMax"` // alerts kept in the inbox
	WebhookURL    string `json:"webhookUrl"`
	WebhookSecret string `json:"webhookSecret"`
	// LastWebhook describes the last delivery (read-only).
	LastWebhook string `json:"lastWebhook"`
}

// AlertDTO is one inbox entry.
type AlertDTO struct {
	ID         int64  `json:"id"`
	TS         int64  `json:"ts"`
	DeviceID   int64  `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Host       string `json:"host"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Unread     bool   `json:"unread"`
}

// AlertQuery filters the inbox.
type AlertQuery struct {
	Severity string `json:"severity"`
	Keyword  string `json:"keyword"`
	Page     int    `json:"page"`
	Size     int    `json:"size"`
}

// AlertPage is a page of the inbox.
type AlertPage struct {
	Items  []AlertDTO `json:"items"`
	Total  int        `json:"total"`
	Unread int        `json:"unread"`
}

// AlertInbox is the current user's inbox state.
type AlertInbox struct {
	Unread int  `json:"unread"`
	Popup  bool `json:"popup"` // new alerts pop up for this user
}

const (
	keyAlertOffline       = "alert_offline"
	keyAlertHealth        = "alert_health"
	keyAlertPower         = "alert_power"
	keyAlertHistoryMax    = "alert_history_max"
	keyAlertWebhookURL    = "alert_webhook_url"
	keyAlertWebhookSecret = "alert_webhook_secret"

	minAlertHistory = 100
	maxAlertHistory = 100000
)

var defaultAlertConfig = AlertConfig{Offline: true, Health: true, PowerLoss: true, HistoryMax: 1000}

func alertDTO(a *store.Alert, unread bool) AlertDTO {
	return AlertDTO{
		ID: a.ID, TS: a.TS, DeviceID: a.DeviceID, DeviceName: a.DeviceName, Host: a.Host,
		Kind: a.Kind, Severity: a.Severity, Title: a.Title, Detail: a.Detail, Unread: unread,
	}
}

func (a *App) loadAlertConfig() error {
	kv, err := a.store.Settings(context.Background())
	if err != nil {
		return err
	}
	c := defaultAlertConfig
	flag := func(key string, p *bool) {
		if v, ok := kv[key]; ok {
			*p = v == "1"
		}
	}
	flag(keyAlertOffline, &c.Offline)
	flag(keyAlertHealth, &c.Health)
	flag(keyAlertPower, &c.PowerLoss)
	if n, err := strconv.Atoi(kv[keyAlertHistoryMax]); err == nil && n >= minAlertHistory && n <= maxAlertHistory {
		c.HistoryMax = n
	}
	c.WebhookURL, c.WebhookSecret = kv[keyAlertWebhookURL], kv[keyAlertWebhookSecret]
	a.alertMu.Lock()
	a.alertCfg = c
	a.alertMu.Unlock()
	return nil
}

func (a *App) alertConfig() AlertConfig {
	a.alertMu.Lock()
	defer a.alertMu.Unlock()
	return a.alertCfg
}

func validateWebhookURL(s string) error {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("Webhook 地址需为 http:// 或 https:// 开头的完整地址")
	}
	return nil
}

// Inbox returns the current user's unread count and pop-up preference.
func (s *AlertService) Inbox() (*AlertInbox, error) {
	sess, err := s.a.auth.Require()
	if err != nil {
		return nil, err
	}
	ctx := s.a.bg()
	popup, readID, err := s.a.store.AlertPrefs(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	_, unread, err := s.a.store.AlertCounts(ctx, readID)
	if err != nil {
		return nil, err
	}
	return &AlertInbox{Unread: unread, Popup: popup}, nil
}

// List returns a page of the inbox, newest first.
func (s *AlertService) List(q AlertQuery) (*AlertPage, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return nil, err
	}
	if q.Size <= 0 || q.Size > 200 {
		q.Size = 50
	}
	if q.Page < 1 {
		q.Page = 1
	}
	ctx := s.a.bg()
	_, readID, err := s.a.store.AlertPrefs(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.a.store.ListAlerts(ctx, store.AlertQuery{
		Severity: q.Severity, Keyword: strings.TrimSpace(q.Keyword), Offset: (q.Page - 1) * q.Size, Limit: q.Size,
	})
	if err != nil {
		return nil, err
	}
	_, unread, err := s.a.store.AlertCounts(ctx, readID)
	if err != nil {
		return nil, err
	}
	page := &AlertPage{Items: make([]AlertDTO, 0, len(rows)), Total: total, Unread: unread}
	for _, r := range rows {
		page.Items = append(page.Items, alertDTO(r, r.ID > readID))
	}
	return page, nil
}

// MarkAllRead marks every alert as read for the current user.
func (s *AlertService) MarkAllRead() error {
	sess, err := s.a.auth.Require()
	if err != nil {
		return err
	}
	maxID, _, err := s.a.store.AlertCounts(s.a.bg(), 0)
	if err != nil {
		return err
	}
	return s.a.store.SetAlertReadID(s.a.bg(), sess.UserID, maxID)
}

// SetPopup turns pop-ups for new alerts on or off for the current user.
func (s *AlertService) SetPopup(on bool) error {
	sess, err := s.a.auth.Require()
	if err != nil {
		return err
	}
	return s.a.store.SetAlertPopup(s.a.bg(), sess.UserID, on)
}

// GetConfig returns the alert settings.
func (s *AlertService) GetConfig() (*AlertConfig, error) {
	if _, err := s.a.auth.Require(auth.PermSettingsManage); err != nil {
		return nil, err
	}
	c := s.a.alertConfig()
	c.LastWebhook = s.a.alerts.lastWebhook()
	return &c, nil
}

// SaveConfig validates and stores the alert settings.
func (s *AlertService) SaveConfig(in AlertConfig) error {
	sess, err := s.a.auth.Require(auth.PermSettingsManage)
	if err != nil {
		return err
	}
	if in.HistoryMax < minAlertHistory || in.HistoryMax > maxAlertHistory {
		return fmt.Errorf("历史告警条数需在 %d 到 %d 之间", minAlertHistory, maxAlertHistory)
	}
	in.WebhookURL, in.WebhookSecret = strings.TrimSpace(in.WebhookURL), strings.TrimSpace(in.WebhookSecret)
	if err := validateWebhookURL(in.WebhookURL); err != nil {
		return err
	}
	b := map[bool]string{true: "1", false: "0"}
	ctx := s.a.bg()
	err = s.a.store.SaveSettings(ctx, map[string]string{
		keyAlertOffline: b[in.Offline], keyAlertHealth: b[in.Health], keyAlertPower: b[in.PowerLoss],
		keyAlertHistoryMax: strconv.Itoa(in.HistoryMax), keyAlertWebhookURL: in.WebhookURL, keyAlertWebhookSecret: in.WebhookSecret,
	})
	var kinds []string
	for _, k := range []struct {
		on   bool
		name string
	}{{in.Offline, "离线"}, {in.Health, "健康"}, {in.PowerLoss, "意外断电"}} {
		if k.on {
			kinds = append(kinds, k.name)
		}
	}
	detail := fmt.Sprintf("告警：%s；保留 %d 条；Webhook：%s", strings.Join(kinds, "、"), in.HistoryMax,
		map[bool]string{true: "未设置", false: "已设置"}[in.WebhookURL == ""])
	s.a.audit(sess, "alert.config", "", err, detail)
	if err != nil {
		return err
	}
	if err := s.a.loadAlertConfig(); err != nil {
		return err
	}
	return s.a.store.PruneAlerts(ctx, in.HistoryMax)
}

// TestWebhook posts a test message with the given (not yet saved) settings.
func (s *AlertService) TestWebhook(in AlertConfig) error {
	if _, err := s.a.auth.Require(auth.PermSettingsManage); err != nil {
		return err
	}
	in.WebhookURL = strings.TrimSpace(in.WebhookURL)
	if in.WebhookURL == "" {
		return errors.New("请先填写 Webhook 地址")
	}
	if err := validateWebhookURL(in.WebhookURL); err != nil {
		return err
	}
	al := &store.Alert{TS: time.Now().Unix(), DeviceName: "测试设备", Host: "192.0.2.1", Kind: alertHealth,
		Severity: severityWarning, Title: "测试消息", Detail: "这是明点服务器管理系统发送的测试消息。"}
	if err := postWebhook(in.WebhookURL, strings.TrimSpace(in.WebhookSecret), newWebhookPayload("test", al, nil)); err != nil {
		return fmt.Errorf("发送失败：%w", err)
	}
	return nil
}
