package app

import (
	"context"
	"fmt"
	"strconv"
)

// Settings are application-wide options editable by administrators.
type Settings struct {
	PollIntervalSec   int `json:"pollIntervalSec"`   // status refresh interval, 0 disables
	MaxConcurrency    int `json:"maxConcurrency"`    // concurrent BMC operations
	IPMITimeoutSec    int `json:"ipmiTimeoutSec"`    // per IPMI request
	RedfishTimeoutSec int `json:"redfishTimeoutSec"` // per Redfish request
	IdleLockMinutes   int `json:"idleLockMinutes"`   // lock the UI after inactivity, 0 disables
	ScanConcurrency   int `json:"scanConcurrency"`   // hosts probed in parallel during discovery
}

var defaultSettings = Settings{
	PollIntervalSec:   120,
	MaxConcurrency:    32,
	IPMITimeoutSec:    5,
	RedfishTimeoutSec: 20,
	IdleLockMinutes:   30,
	ScanConcurrency:   64,
}

type settingField struct {
	key      string
	ptr      func(*Settings) *int
	min, max int
	label    string
}

var settingFields = []settingField{
	{"poll_interval_sec", func(s *Settings) *int { return &s.PollIntervalSec }, 0, 86400, "状态刷新间隔"},
	{"max_concurrency", func(s *Settings) *int { return &s.MaxConcurrency }, 1, 256, "最大并发数"},
	{"ipmi_timeout_sec", func(s *Settings) *int { return &s.IPMITimeoutSec }, 1, 60, "IPMI 超时"},
	{"redfish_timeout_sec", func(s *Settings) *int { return &s.RedfishTimeoutSec }, 3, 300, "Redfish 超时"},
	{"idle_lock_minutes", func(s *Settings) *int { return &s.IdleLockMinutes }, 0, 1440, "空闲锁定时间"},
	{"scan_concurrency", func(s *Settings) *int { return &s.ScanConcurrency }, 1, 512, "扫描并发数"},
}

func (a *App) loadSettings() error {
	kv, err := a.store.Settings(context.Background())
	if err != nil {
		return err
	}
	s := defaultSettings
	for _, f := range settingFields {
		if v, ok := kv[f.key]; ok {
			if n, err := strconv.Atoi(v); err == nil && n >= f.min && n <= f.max {
				*f.ptr(&s) = n
			}
		}
	}
	a.applySettings(s)
	return nil
}

func (a *App) applySettings(s Settings) {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.sem == nil || cap(a.sem) != s.MaxConcurrency {
		// In-flight operations keep the old semaphore until they finish.
		a.sem = make(chan struct{}, s.MaxConcurrency)
	}
	a.settings = s
}

// Settings returns the current settings.
func (a *App) Settings() Settings {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.settings
}

func validateSettings(s Settings) error {
	for _, f := range settingFields {
		v := *f.ptr(&s)
		if v < f.min || v > f.max {
			return fmt.Errorf("%s 需在 %d 到 %d 之间", f.label, f.min, f.max)
		}
	}
	if s.PollIntervalSec != 0 && s.PollIntervalSec < 15 {
		return fmt.Errorf("状态刷新间隔不能小于 15 秒（填 0 表示关闭自动刷新）")
	}
	return nil
}

func (a *App) saveSettings(ctx context.Context, s Settings) error {
	if err := validateSettings(s); err != nil {
		return err
	}
	kv := map[string]string{}
	for _, f := range settingFields {
		kv[f.key] = strconv.Itoa(*f.ptr(&s))
	}
	if err := a.store.SaveSettings(ctx, kv); err != nil {
		return err
	}
	a.applySettings(s)
	a.poller.reschedule()
	return nil
}
