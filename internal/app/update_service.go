package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/store"
	"github.com/12312bg/mdbmc/internal/update"
)

// Settings keys of the update configuration.
const (
	keyUpdateCheckHours = "update_check_hours"
	keyUpdateSource     = "update_source"
	keyUpdateProxy      = "update_proxy"
)

const (
	defaultUpdateCheckHours = 24
	maxUpdateCheckHours     = 720
	// The first automatic check waits a little so it does not slow startup.
	firstUpdateCheck = 20 * time.Second
	updateCheckRetry = time.Hour // after a failed automatic check
	updateCheckLimit = 60 * time.Second
)

// UpdateService checks for, downloads and installs new versions. Every
// logged-in user sees whether a new version exists; downloading,
// installing and the configuration need the system.update permission.
type UpdateService struct{ a *App }

// initUpdater prepares the updater and picks up the outcome of an update
// installed before this start.
func (a *App) initUpdater() error {
	a.updateDir = filepath.Join(a.dataDir, "updates")
	if err := os.MkdirAll(a.updateDir, 0o700); err != nil {
		return err
	}
	mode := update.ModeUnsupported
	if exe, err := os.Executable(); err == nil {
		mode = update.DetectMode(exe)
	}
	keys, err := update.Keys()
	if err != nil {
		log.Printf("update: %v", err)
	}
	a.updater = update.New(update.Options{
		Current: Version, Mode: mode, Dir: a.updateDir, Keys: keys,
		OnChange: func() { a.emit("update:state", a.updateStatus()) },
	})
	cfg, err := a.loadUpdateConfig()
	if err != nil {
		return err
	}
	a.updateCfg = cfg
	a.updater.SetConfig(update.Config{Source: cfg.Source, Proxy: cfg.Proxy})
	a.updateKick = make(chan struct{}, 1)

	if r := update.TakeResult(a.updateDir); r != nil {
		a.updateResult = &UpdateResult{From: r.From, Version: r.Version, Success: r.Success, Error: r.Error, Time: r.Time}
		e := &store.AuditEntry{UserID: r.UserID, Username: r.User, Action: "update.finish", Target: "v" + r.Version,
			Success: r.Success, Detail: fmt.Sprintf("从 %s 更新到 %s", r.From, r.Version)}
		if !r.Success {
			e.Detail += "；" + r.Error
		}
		if err := a.store.AddAudit(context.Background(), e); err != nil {
			log.Printf("audit: %v", err)
		}
	}
	a.updater.PruneInstalled()
	return nil
}

func (a *App) loadUpdateConfig() (UpdateConfig, error) {
	kv, err := a.store.Settings(context.Background())
	if err != nil {
		return UpdateConfig{}, err
	}
	c := UpdateConfig{CheckHours: defaultUpdateCheckHours}
	if v, ok := kv[keyUpdateCheckHours]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= maxUpdateCheckHours {
			c.CheckHours = n
		}
	}
	if s, err := update.NormalizeSource(kv[keyUpdateSource]); err == nil {
		c.Source = s
	}
	if p, err := update.NormalizeProxy(kv[keyUpdateProxy]); err == nil {
		c.Proxy = p
	}
	return c, nil
}

func (a *App) updateConfig() UpdateConfig {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.updateCfg
}

func (a *App) updateStatus() UpdateStatus {
	st := a.updater.Status()
	mode := a.updater.Mode()
	out := UpdateStatus{
		CurrentVersion: Version, State: string(st.State), Mode: string(mode),
		CanInstall: mode != update.ModeUnsupported && a.quit != nil,
		Downloaded: st.Downloaded, Total: st.Total, Speed: st.Speed, Error: st.Error,
	}
	if !st.CheckedAt.IsZero() {
		out.CheckedAt = st.CheckedAt.Unix()
	}
	if m := st.Latest; m != nil {
		out.Latest = &UpdateRelease{
			Version: m.Version, Tag: m.Tag, PublishedAt: m.PublishedAt.Unix(), Notes: m.Notes,
			Size: m.Assets[mode].Size, URL: update.ReleaseURL(a.updater.Config().Source, m.Tag),
		}
	}
	a.updateMu.Lock()
	out.LastResult = a.updateResult
	a.updateMu.Unlock()
	return out
}

// startUpdateChecks runs the automatic checks until Shutdown. A build
// that cannot verify updates only checks when asked to.
func (a *App) startUpdateChecks() {
	if !a.updater.CanVerify() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.updateStop = cancel
	go a.updateLoop(ctx)
}

// nextUpdateCheck is how long until the next automatic check, or -1 when
// they are off.
func (a *App) nextUpdateCheck() time.Duration {
	hours := a.updateConfig().CheckHours
	if hours == 0 {
		return -1
	}
	last := a.updater.Status().CheckedAt
	if last.IsZero() {
		return firstUpdateCheck
	}
	return max(time.Until(last.Add(time.Duration(hours)*time.Hour)), 0)
}

func (a *App) updateLoop(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	schedule := func(d time.Duration) {
		if d < 0 {
			timer.Stop()
		} else {
			timer.Reset(d)
		}
	}
	schedule(a.nextUpdateCheck())
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-a.updateKick:
			schedule(a.nextUpdateCheck())
		case <-timer.C:
			hours := a.updateConfig().CheckHours
			if hours == 0 {
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, updateCheckLimit)
			err := a.updater.Check(cctx)
			cancel()
			next := time.Duration(hours) * time.Hour
			if err != nil {
				log.Printf("update: %v", err)
				next = min(next, updateCheckRetry)
			}
			schedule(next)
		}
	}
}

// Status returns the update state.
func (s *UpdateService) Status() (*UpdateStatus, error) {
	if _, err := s.a.auth.Require(); err != nil {
		return nil, err
	}
	st := s.a.updateStatus()
	return &st, nil
}

// Check looks for a new version now.
func (s *UpdateService) Check() (*UpdateStatus, error) {
	if _, err := s.a.auth.Require(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.a.bg(), updateCheckLimit)
	defer cancel()
	if err := s.a.updater.Check(ctx); err != nil {
		return nil, err
	}
	st := s.a.updateStatus()
	return &st, nil
}

// Download starts downloading the new version; progress arrives as
// update:state events.
func (s *UpdateService) Download() error {
	if _, err := s.a.auth.Require(auth.PermSystemUpdate); err != nil {
		return err
	}
	if s.a.updater.Mode() == update.ModeUnsupported {
		return errors.New("当前系统不支持在线安装，请到发布页面下载")
	}
	return s.a.updater.Download()
}

// CancelDownload stops the download; the next one resumes it.
func (s *UpdateService) CancelDownload() error {
	if _, err := s.a.auth.Require(auth.PermSystemUpdate); err != nil {
		return err
	}
	s.a.updater.Cancel()
	return nil
}

// Install hands the downloaded package to the update helper and exits the
// program, closing the console windows. The helper installs it and starts
// the program again.
func (s *UpdateService) Install() error {
	sess, err := s.a.auth.Require(auth.PermSystemUpdate)
	if err != nil {
		return err
	}
	if s.a.quit == nil || s.a.updater.Mode() == update.ModeUnsupported {
		return errors.New("当前运行方式不支持在线安装更新")
	}
	pkg, m, asset, err := s.a.updater.Prepared()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	mode := s.a.updater.Mode()
	err = update.Launch(s.a.updateDir, update.Plan{
		PID: os.Getpid(), Exe: exe, Mode: mode, Package: pkg, Asset: asset,
		From: Version, Version: m.Version, User: sess.Username, UserID: sess.UserID,
		Log: filepath.Join(s.a.dataDir, "mdbmc.log"),
	})
	how := map[update.Mode]string{update.ModeInstaller: "安装程序", update.ModePortable: "替换程序文件"}[mode]
	s.a.audit(sess, "update.install", "v"+m.Version, err, fmt.Sprintf("从 %s 更新，方式：%s", Version, how))
	if err != nil {
		return err
	}
	s.a.updater.SetInstalling()
	go func() {
		// Let this call return to the window before it closes.
		time.Sleep(800 * time.Millisecond)
		s.a.closeConsoles()
		s.a.quit()
	}()
	return nil
}

// DismissResult hides the outcome of the last update once it was shown.
func (s *UpdateService) DismissResult() error {
	if _, err := s.a.auth.Require(); err != nil {
		return err
	}
	s.a.updateMu.Lock()
	s.a.updateResult = nil
	s.a.updateMu.Unlock()
	return nil
}

// GetConfig returns the update configuration.
func (s *UpdateService) GetConfig() (*UpdateConfig, error) {
	if _, err := s.a.auth.Require(auth.PermSystemUpdate); err != nil {
		return nil, err
	}
	c := s.a.updateConfig()
	c.DefaultSource = update.DefaultSource
	return &c, nil
}

// SaveConfig validates and stores the update configuration.
func (s *UpdateService) SaveConfig(in UpdateConfig) error {
	sess, err := s.a.auth.Require(auth.PermSystemUpdate)
	if err != nil {
		return err
	}
	if in.CheckHours < 0 || in.CheckHours > maxUpdateCheckHours {
		return fmt.Errorf("自动检查间隔需在 0 到 %d 小时之间", maxUpdateCheckHours)
	}
	src, err := update.NormalizeSource(in.Source)
	if err != nil {
		return err
	}
	proxy, err := update.NormalizeProxy(in.Proxy)
	if err != nil {
		return err
	}
	err = s.a.store.SaveSettings(s.a.bg(), map[string]string{
		keyUpdateCheckHours: strconv.Itoa(in.CheckHours), keyUpdateSource: src, keyUpdateProxy: proxy,
	})
	detail := fmt.Sprintf("自动检查间隔 %d 小时，更新源：%s，代理：%s", in.CheckHours,
		map[bool]string{true: "默认（GitHub）", false: src}[src == ""],
		map[bool]string{true: "系统代理", false: "已设置"}[proxy == ""])
	s.a.audit(sess, "update.config", "", err, detail)
	if err != nil {
		return err
	}
	s.a.updateMu.Lock()
	s.a.updateCfg = UpdateConfig{CheckHours: in.CheckHours, Source: src, Proxy: proxy}
	s.a.updateMu.Unlock()
	s.a.updater.SetConfig(update.Config{Source: src, Proxy: proxy})
	select {
	case s.a.updateKick <- struct{}{}:
	default:
	}
	return nil
}
