package app

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmcsim"
	"github.com/12312bg/mdbmc/internal/console"
	"github.com/12312bg/mdbmc/internal/vault"
)

type harness struct {
	t        *testing.T
	app      *App
	authS    *AuthService
	admin    *AdminService
	devices  *DeviceService
	creds    *CredentialService
	bmcS     *BMCService
	mu       sync.Mutex
	launched []console.Request
	events   []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	// Keep the KDF cheap in tests.
	old := vault.DefaultKDF
	vault.DefaultKDF = vault.KDFParams{Time: 1, MemoryKiB: 8 * 1024, Threads: 1}
	t.Cleanup(func() { vault.DefaultKDF = old })

	h := &harness{t: t}
	a, err := New(t.TempDir(), func(r console.Request) error {
		h.mu.Lock()
		h.launched = append(h.launched, r)
		h.mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.emit = func(name string, _ any) {
		h.mu.Lock()
		h.events = append(h.events, name)
		h.mu.Unlock()
	}
	t.Cleanup(a.Shutdown)
	h.app = a
	h.authS = &AuthService{a: a}
	h.admin = &AdminService{a: a}
	h.devices = &DeviceService{a: a}
	h.creds = &CredentialService{a: a}
	h.bmcS = &BMCService{a: a}
	return h
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetupLoginAndRoles(t *testing.T) {
	h := newHarness(t)
	st, err := h.authS.Status()
	must(t, err)
	if st.Initialized {
		t.Fatal("fresh database should not be initialized")
	}
	if _, err := h.authS.Setup("admin", "管理员", "short"); err == nil {
		t.Fatal("short password accepted")
	}
	me, err := h.authS.Setup("admin", "管理员", "Admin@12345")
	must(t, err)
	if me.Role != "admin" {
		t.Fatalf("setup role = %s", me.Role)
	}
	if _, err := h.authS.Setup("x", "", "Admin@12345"); err == nil {
		t.Fatal("second setup should fail")
	}

	must(t, h.admin.CreateUser(UserInput{Username: "op", Role: "operator", Password: "Operator@1"}))
	must(t, h.admin.CreateUser(UserInput{Username: "ro", Role: "viewer", Password: "Viewer@123"}))
	must(t, h.creds.Create(CredentialInput{Name: "默认", Username: "ADMIN", Password: "secret-pw"}))

	// The last admin cannot be demoted or deleted.
	users, err := h.admin.ListUsers()
	must(t, err)
	var adminID, roID int64
	for _, u := range users {
		switch u.Username {
		case "admin":
			adminID = u.ID
		case "ro":
			roID = u.ID
		}
	}
	if err := h.admin.UpdateUser(UserInput{ID: adminID, Role: "viewer"}); err == nil {
		t.Fatal("admin demoted itself")
	}

	h.authS.Logout()
	if _, err := h.devices.List(); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Fatalf("want not logged in, got %v", err)
	}
	if _, err := h.authS.Login("admin", "wrong-password"); err == nil {
		t.Fatal("wrong password accepted")
	}

	// A viewer shares the vault key (it can read sensors) but not the rights.
	me, err = h.authS.Login("RO", "Viewer@123")
	must(t, err)
	if !me.MustChangePassword {
		t.Fatal("new users must change their password")
	}
	if _, err := h.creds.List(); !errors.Is(err, auth.ErrMustChangePassword) {
		t.Fatalf("initial password must be changed first, got %v", err)
	}
	must(t, h.authS.ChangePassword("Viewer@123", "Viewer@456"))
	creds, err := h.creds.List()
	must(t, err)
	if _, err := h.creds.Reveal(creds[0].ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer revealed a password: %v", err)
	}
	if _, err := h.bmcS.Power([]int64{1}, "off"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer controlled power: %v", err)
	}
	if _, err := h.admin.ListUsers(); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer listed users: %v", err)
	}
	h.authS.Logout()

	// The operator can reveal passwords, decrypted with its own wrapped key.
	_, err = h.authS.Login("op", "Operator@1")
	must(t, err)
	must(t, h.authS.ChangePassword("Operator@1", "Operator@2"))
	pw, err := h.creds.Reveal(creds[0].ID)
	must(t, err)
	if pw != "secret-pw" {
		t.Fatalf("revealed %q", pw)
	}
	if _, err := h.admin.ListAudit(AuditQuery{}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("operator read audit: %v", err)
	}
	h.authS.Logout()

	// Admin resets the viewer's password; the viewer can still decrypt.
	_, err = h.authS.Login("admin", "Admin@12345")
	must(t, err)
	must(t, h.admin.ResetPassword(roID, "Reset@7890"))
	page, err := h.admin.ListAudit(AuditQuery{Keyword: "credential.reveal"})
	must(t, err)
	// Only the operator's successful reveal is logged; the viewer was refused
	// before reaching the vault.
	if page.Total != 1 || page.Items[0].Username != "op" {
		t.Fatalf("reveal audit rows = %+v", page.Items)
	}
	h.authS.Logout()
	_, err = h.authS.Login("ro", "Reset@7890")
	must(t, err)
	must(t, h.authS.ChangePassword("Reset@7890", "Viewer@789"))
	if _, err := h.creds.List(); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceLifecycleAgainstSimulator(t *testing.T) {
	sim, err := bmcsim.Start(bmcsim.Options{Index: 7})
	must(t, err)
	t.Cleanup(sim.Close)

	h := newHarness(t)
	_, err = h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	h.app.poller.stop() // drive polling explicitly

	g, err := h.devices.CreateGroup("机房 A")
	must(t, err)
	dev, err := h.devices.Create(DeviceInput{
		Name: "node07", Host: "127.0.0.1", IPMIPort: sim.IPMIPort, HTTPSPort: sim.HTTPSPort,
		GroupID: g.ID, Username: "ADMIN", Password: "ADMIN",
	})
	must(t, err)

	// Identification runs in the background.
	deadline := time.Now().Add(20 * time.Second)
	for {
		d, err := h.devices.Get(dev.ID)
		must(t, err)
		if d.IdentifiedAt > 0 {
			dev = d
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("device was not identified: %+v", d)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if dev.Vendor != "supermicro" || !dev.HasIPMI || !dev.HasRedfish || dev.Model != "SYS-120U-TNR" ||
		dev.TLSFingerprint != sim.Fingerprint || dev.GroupName != "机房 A" || dev.PowerState != "on" {
		t.Fatalf("identified device = %+v", dev)
	}

	res, err := h.bmcS.Power([]int64{dev.ID}, "off")
	must(t, err)
	if res.Succeeded != 1 || sim.PowerOn() {
		t.Fatalf("power off: %+v", res)
	}
	h.app.poller.pollNow(h.app.auth.Current(), []int64{dev.ID})
	d, _ := h.devices.Get(dev.ID)
	if d.PowerState != "off" || !d.Reachable {
		t.Fatalf("after poll: %+v", d)
	}

	sensors, err := h.bmcS.Sensors(dev.ID)
	must(t, err)
	if len(sensors.Sensors) < 10 || sensors.Via != "ipmi" {
		t.Fatalf("sensors: %d via %s", len(sensors.Sensors), sensors.Via)
	}
	sel, err := h.bmcS.SEL(dev.ID)
	must(t, err)
	if len(sel.Entries) != 6 {
		t.Fatalf("sel entries = %d", len(sel.Entries))
	}
	must(t, h.bmcS.ClearSEL(dev.ID))

	inv, err := h.bmcS.Inventory(dev.ID, false)
	must(t, err)
	if inv.Cached || len(inv.Inventory.Memory) != 8 {
		t.Fatalf("inventory: %+v", inv)
	}
	inv, err = h.bmcS.Inventory(dev.ID, false)
	must(t, err)
	if !inv.Cached {
		t.Fatal("second inventory read should come from cache")
	}

	kvm, err := h.bmcS.OpenKVM(dev.ID)
	must(t, err)
	if kvm.AutoLogin || !strings.Contains(kvm.URL, "token=") {
		t.Fatalf("kvm = %+v", kvm)
	}
	web, err := h.bmcS.OpenWeb(dev.ID)
	must(t, err)
	if !web.AutoLogin {
		t.Fatal("admin should get auto-login")
	}
	h.mu.Lock()
	n := len(h.launched)
	last := h.launched[n-1]
	h.mu.Unlock()
	if last.CertFingerprint != dev.TLSFingerprint || last.CertFingerprint == "" {
		t.Fatalf("console request must carry the pinned certificate, got %q", last.CertFingerprint)
	}
	if n != 2 || last.Spec.Password != "ADMIN" || !strings.HasSuffix(last.Profile, filepath.Join("console", strconv.FormatInt(dev.ID, 10))) {
		t.Fatalf("launched = %+v", h.launched)
	}

	pw, err := h.devices.RevealPassword(dev.ID)
	must(t, err)
	if pw != "ADMIN" {
		t.Fatalf("password = %q", pw)
	}

	// A viewer opens only the bare login page.
	must(t, h.admin.CreateUser(UserInput{Username: "ro", Role: "viewer", Password: "Viewer@123"}))
	h.authS.Logout()
	_, err = h.authS.Login("ro", "Viewer@123")
	must(t, err)
	must(t, h.authS.ChangePassword("Viewer@123", "Viewer@456"))
	h.app.poller.stop()
	web, err = h.bmcS.OpenWeb(dev.ID)
	must(t, err)
	h.mu.Lock()
	last = h.launched[len(h.launched)-1]
	h.mu.Unlock()
	if web.AutoLogin || last.Spec.Password != "" || last.Spec.AutoLogin {
		t.Fatalf("viewer console request leaked credentials: %+v", last.Spec)
	}
	if _, err := h.bmcS.OpenKVM(dev.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer opened KVM: %v", err)
	}
	ov, err := h.bmcS.Overview()
	must(t, err)
	if ov.Total != 1 || ov.PoweredOff != 1 {
		t.Fatalf("overview = %+v", ov)
	}
}
