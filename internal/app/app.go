// Package app is the service layer bound to the Wails frontend. Every bound
// method checks the caller's role before touching the store or a BMC, and
// records security-relevant actions in the audit log.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/console"
	"github.com/12312bg/mdbmc/internal/store"
)

// Version is set at build time with -ldflags "-X .../internal/app.Version=...".
var Version = "0.1.0"

// Secret purposes bound into ciphertexts (see vault.Box.SealString).
const (
	purposeDevicePassword     = "device-password"
	purposeCredentialPassword = "credential-password"
)

// Emitter delivers events to the frontend.
type Emitter func(name string, data any)

// App holds shared state for all bound services.
type App struct {
	ctx     context.Context
	dataDir string
	store   *store.Store
	auth    *auth.Manager
	emit    Emitter
	launch  console.Launcher
	// closeConsoles closes the console windows (on logout).
	closeConsoles func()

	settingsMu sync.RWMutex
	settings   Settings

	sem      chan struct{} // global cap on concurrent BMC operations
	devMu    sync.Mutex
	devSems  map[int64]chan struct{}
	poller   *poller
	scanner  *scanner
	services []any
}

// New opens the database in dataDir and prepares the services.
func New(dataDir string, launcher console.Launcher) (*App, error) {
	st, err := store.Open(filepath.Join(dataDir, "mdbmc.db"))
	if err != nil {
		return nil, err
	}
	a := &App{
		ctx:     context.Background(),
		dataDir: dataDir,
		store:   st,
		auth:    auth.NewManager(),
		emit:    func(string, any) {},
		launch:  launcher,
		devSems: map[int64]chan struct{}{},

		closeConsoles: console.CloseAll,
	}
	if err := a.loadSettings(); err != nil {
		st.Close()
		return nil, err
	}
	a.poller = newPoller(a)
	a.scanner = &scanner{a: a}
	a.services = []any{
		&AuthService{a: a},
		&AdminService{a: a},
		&DeviceService{a: a},
		&CredentialService{a: a},
		&BMCService{a: a},
		&DiscoveryService{a: a},
	}
	return a, nil
}

// Bindings returns the objects exposed to the frontend.
func (a *App) Bindings() []any { return a.services }

// Startup is called by Wails with the runtime context.
func (a *App) Startup(ctx context.Context, emit Emitter) {
	a.ctx = ctx
	if emit != nil {
		a.emit = emit
	}
}

// Shutdown stops background work and closes the database.
func (a *App) Shutdown() {
	a.poller.stop()
	a.scanner.cancel()
	a.auth.Clear()
	a.store.Close()
}

func (a *App) bg() context.Context { return a.ctx }

// audit records an action by the current session's user.
func (a *App) audit(s *auth.Session, action, target string, err error, detail string) {
	e := &store.AuditEntry{Action: action, Target: target, Success: err == nil, Detail: detail}
	if s != nil {
		e.UserID, e.Username = s.UserID, s.Username
	}
	if err != nil {
		if detail != "" {
			e.Detail = detail + "；" + err.Error()
		} else {
			e.Detail = err.Error()
		}
	}
	if werr := a.store.AddAudit(context.Background(), e); werr != nil {
		log.Printf("audit: %v", werr)
	}
}

// acquire limits concurrent BMC work globally and per device (BMCs only
// allow a handful of sessions).
func (a *App) acquire(ctx context.Context, deviceID int64) (func(), error) {
	a.devMu.Lock()
	ds, ok := a.devSems[deviceID]
	if !ok {
		ds = make(chan struct{}, 2)
		a.devSems[deviceID] = ds
	}
	a.devMu.Unlock()
	select {
	case ds <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	a.settingsMu.RLock()
	sem := a.sem
	a.settingsMu.RUnlock()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		<-ds
		return nil, ctx.Err()
	}
	return func() { <-sem; <-ds }, nil
}

// target builds the connection parameters for a device, decrypting its
// password with the session's vault key.
func (a *App) target(ctx context.Context, s *auth.Session, d *store.Device) (bmc.Target, error) {
	set := a.Settings()
	t := bmc.Target{
		Host: d.Host, IPMIPort: d.IPMIPort, HTTPSPort: d.HTTPSPort,
		Protocol: bmc.Protocol(d.Protocol), HasIPMI: d.HasIPMI, HasRedfish: d.HasRedfish,
		TLSFingerprint: d.TLSFingerprint, Vendor: d.Vendor,
		IPMITimeout:    time.Duration(set.IPMITimeoutSec) * time.Second,
		RedfishTimeout: time.Duration(set.RedfishTimeoutSec) * time.Second,
	}
	user, pass, err := a.credentialsFor(ctx, s, d)
	if err != nil {
		return t, err
	}
	t.Username, t.Password = user, pass
	return t, nil
}

func (a *App) credentialsFor(ctx context.Context, s *auth.Session, d *store.Device) (string, string, error) {
	if d.CredentialID != 0 {
		c, err := a.store.CredentialByID(ctx, d.CredentialID)
		if err != nil {
			return "", "", fmt.Errorf("读取凭据失败: %w", err)
		}
		pass, err := s.Box().OpenString(c.PasswordEnc, purposeCredentialPassword)
		if err != nil {
			return "", "", errors.New("无法解密凭据密码")
		}
		return c.Username, pass, nil
	}
	if len(d.PasswordEnc) == 0 {
		return d.Username, "", nil
	}
	pass, err := s.Box().OpenString(d.PasswordEnc, purposeDevicePassword)
	if err != nil {
		return "", "", errors.New("无法解密设备密码")
	}
	return d.Username, pass, nil
}

// withClient runs fn with a BMC client for device id under the concurrency
// limits, then pins a first-seen TLS certificate. The timeout starts once a
// concurrency slot is free, so waiting in the queue does not eat into it.
func (a *App) withClient(ctx context.Context, s *auth.Session, id int64, timeout time.Duration,
	fn func(context.Context, *store.Device, *bmc.Client) error) (*store.Device, error) {
	if a.auth.Current() != s {
		return nil, auth.ErrNotLoggedIn
	}
	d, err := a.store.DeviceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	release, err := a.acquire(ctx, id)
	if err != nil {
		return d, err
	}
	defer release()
	// The user may have logged out while this operation was queued.
	if a.auth.Current() != s {
		return d, auth.ErrNotLoggedIn
	}
	t, err := a.target(ctx, s, d)
	if err != nil {
		return d, err
	}
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := bmc.New(t)
	err = fn(opCtx, d, c)
	c.Close()
	a.pinIfFirst(d, c.ObservedFingerprint)
	return d, err
}

func (a *App) pinIfFirst(d *store.Device, observed string) {
	if d.TLSFingerprint == "" && observed != "" {
		if ok, err := a.store.PinTLSFingerprint(context.Background(), d.ID, d.Host, d.HTTPSPort, observed); err == nil && ok {
			d.TLSFingerprint = observed
		}
	}
}

func deviceLabel(d *store.Device) string {
	if d == nil {
		return ""
	}
	if d.Name != "" && d.Name != d.Host {
		return fmt.Sprintf("%s (%s)", d.Name, d.Host)
	}
	return d.Host
}
