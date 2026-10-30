package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/console"
	"github.com/12312bg/mdbmc/internal/store"
)

// BMCService runs operations against BMCs.
type BMCService struct{ a *App }

var actionLabels = map[bmc.PowerAction]string{
	bmc.ActionOn: "开机", bmc.ActionOff: "强制关机", bmc.ActionSoft: "正常关机",
	bmc.ActionReset: "硬重启", bmc.ActionCycle: "电源循环",
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Power runs a power action on several devices concurrently. Per-device
// outcomes are also pushed as "batch:item" events while the batch runs.
func (s *BMCService) Power(ids []int64, action string) (*BatchResult, error) {
	sess, err := s.a.auth.Require(auth.PermPowerControl)
	if err != nil {
		return nil, err
	}
	act := bmc.PowerAction(action)
	if !act.Valid() {
		return nil, fmt.Errorf("未知电源操作 %q", action)
	}
	if len(ids) == 0 {
		return nil, errors.New("请选择设备")
	}
	// The power-offs that follow are expected, not alerts.
	s.a.alerts.notePowerAction(ids)
	res := &BatchResult{BatchID: newID(), Total: len(ids)}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			ctx := s.a.bg()
			var via bmc.Protocol
			d, err := s.a.withClient(ctx, sess, id, 60*time.Second, func(ctx context.Context, _ *store.Device, c *bmc.Client) error {
				var err error
				via, err = c.Power(ctx, act)
				return err
			})
			item := BatchItem{BatchID: res.BatchID, DeviceID: id, OK: err == nil, Via: string(via)}
			if d != nil {
				item.Name = d.Name
			}
			if err != nil {
				item.Error = err.Error()
			}
			s.a.audit(sess, "power."+action, deviceLabel(d), err, actionLabels[act])
			s.a.emit("batch:item", item)
			mu.Lock()
			res.Items = append(res.Items, item)
			if item.OK {
				res.Succeeded++
			} else {
				res.Failed++
			}
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	sort.Slice(res.Items, func(i, j int) bool { return res.Items[i].DeviceID < res.Items[j].DeviceID })
	// Power state changes take a few seconds to show up on the BMC.
	go func() {
		time.Sleep(6 * time.Second)
		s.a.poller.pollNow(sess, ids)
	}()
	return res, nil
}

// IdentifyLED turns the chassis identify LED on (indefinitely) or off.
func (s *BMCService) IdentifyLED(id int64, on bool) error {
	sess, err := s.a.auth.Require(auth.PermPowerControl)
	if err != nil {
		return err
	}
	ctx := s.a.bg()
	d, err := s.a.withClient(ctx, sess, id, 30*time.Second, func(ctx context.Context, _ *store.Device, c *bmc.Client) error {
		if on {
			return c.IdentifyLED(ctx, 0, true)
		}
		return c.IdentifyLED(ctx, 0, false)
	})
	state := "关闭"
	if on {
		state = "点亮"
	}
	s.a.audit(sess, "power.identify", deviceLabel(d), err, "定位灯"+state)
	return err
}

// Sensors reads a device's sensors.
func (s *BMCService) Sensors(id int64) (*SensorsResult, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return nil, err
	}
	ctx := s.a.bg()
	res := &SensorsResult{}
	_, err = s.a.withClient(ctx, sess, id, 120*time.Second, func(ctx context.Context, _ *store.Device, c *bmc.Client) error {
		sensors, via, err := c.Sensors(ctx)
		res.Sensors, res.Via = sensors, string(via)
		return err
	})
	if err != nil {
		return nil, err
	}
	if res.Sensors == nil {
		res.Sensors = []bmc.Sensor{}
	}
	res.FetchedAt = time.Now().Unix()
	return res, nil
}

// SEL reads a device's event log.
func (s *BMCService) SEL(id int64) (*SELResult, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return nil, err
	}
	ctx := s.a.bg()
	res := &SELResult{}
	_, err = s.a.withClient(ctx, sess, id, 120*time.Second, func(ctx context.Context, _ *store.Device, c *bmc.Client) error {
		entries, via, err := c.SEL(ctx)
		res.Entries, res.Via = entries, string(via)
		return err
	})
	if err != nil {
		return nil, err
	}
	if res.Entries == nil {
		res.Entries = []bmc.SELEntry{}
	}
	res.FetchedAt = time.Now().Unix()
	return res, nil
}

// ClearSEL clears a device's event log.
func (s *BMCService) ClearSEL(id int64) error {
	sess, err := s.a.auth.Require(auth.PermSELClear)
	if err != nil {
		return err
	}
	ctx := s.a.bg()
	d, err := s.a.withClient(ctx, sess, id, 60*time.Second, func(ctx context.Context, _ *store.Device, c *bmc.Client) error {
		_, err := c.ClearSEL(ctx)
		return err
	})
	s.a.audit(sess, "sel.clear", deviceLabel(d), err, "")
	return err
}

// Inventory returns the cached inventory, collecting it when refresh is set
// or nothing is cached yet.
func (s *BMCService) Inventory(id int64, refresh bool) (*InventoryResult, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return nil, err
	}
	if !refresh {
		if data, ts, err := s.a.store.Inventory(s.a.bg(), id); err == nil {
			var inv bmc.Inventory
			if json.Unmarshal([]byte(data), &inv) == nil {
				return &InventoryResult{Inventory: &inv, UpdatedAt: ts, Cached: true}, nil
			}
		}
	}
	ctx := s.a.bg()
	var inv *bmc.Inventory
	_, err = s.a.withClient(ctx, sess, id, 180*time.Second, func(ctx context.Context, _ *store.Device, c *bmc.Client) error {
		var err error
		inv, err = c.Inventory(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(inv)
	_ = s.a.store.SaveInventory(s.a.bg(), id, string(b))
	return &InventoryResult{Inventory: inv, UpdatedAt: time.Now().Unix()}, nil
}

// OpenWeb opens the BMC web UI in a console window. Users without the
// auto-login permission get the bare login page.
func (s *BMCService) OpenWeb(id int64) (*ConsoleResult, error) {
	sess, err := s.a.auth.Require(auth.PermWebOpen)
	if err != nil {
		return nil, err
	}
	auto := sess.Can(auth.PermWebAutoLogin)
	ctx, cancel := context.WithTimeout(s.a.bg(), 30*time.Second)
	defer cancel()
	d, err := s.a.store.DeviceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	spec := &bmc.LaunchSpec{URL: bmc.New(bmc.Target{Host: d.Host, HTTPSPort: d.HTTPSPort}).WebURL(), AutoLogin: auto}
	if auto {
		if spec.Username, spec.Password, err = s.a.credentialsFor(ctx, sess, d); err != nil {
			return nil, err
		}
	}
	err = s.a.openConsole(ctx, d, spec, "Web 管理")
	detail := "仅打开登录页"
	if auto {
		detail = "自动登录"
	}
	s.a.audit(sess, "console.web", deviceLabel(d), err, detail)
	if err != nil {
		return nil, err
	}
	return &ConsoleResult{URL: spec.URL, AutoLogin: auto}, nil
}

// OpenKVM opens the remote KVM console.
func (s *BMCService) OpenKVM(id int64) (*ConsoleResult, error) {
	sess, err := s.a.auth.Require(auth.PermKVM)
	if err != nil {
		return nil, err
	}
	ctx := s.a.bg()
	var spec *bmc.LaunchSpec
	var user, pass string
	d, err := s.a.withClient(ctx, sess, id, 45*time.Second, func(ctx context.Context, d *store.Device, c *bmc.Client) error {
		var err error
		if spec, err = c.KVMLaunch(ctx); err != nil {
			return err
		}
		user, pass, err = s.a.credentialsFor(ctx, sess, d)
		return err
	})
	if err == nil && spec.AutoLogin {
		spec.Username, spec.Password = user, pass
	}
	if err == nil {
		err = s.a.openConsole(ctx, d, spec, "远程控制台")
	}
	s.a.audit(sess, "console.kvm", deviceLabel(d), err, "")
	if err != nil {
		return nil, err
	}
	return &ConsoleResult{URL: spec.URL, AutoLogin: spec.AutoLogin, Hint: spec.Hint}, nil
}

// openConsole verifies the BMC certificate against the pin, then starts a
// console window whose browser accepts only that certificate.
func (a *App) openConsole(ctx context.Context, d *store.Device, spec *bmc.LaunchSpec, kind string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cert, err := bmc.ProbeCertificate(ctx, d.Host, d.HTTPSPort, d.TLSFingerprint)
	if err != nil {
		var mm *bmc.FingerprintMismatchError
		if errors.As(err, &mm) {
			return err
		}
		return fmt.Errorf("无法连接 BMC Web 服务: %w", err)
	}
	a.pinIfFirst(d, cert.Fingerprint)
	return a.launch(console.Request{
		Title:           fmt.Sprintf("%s - %s", d.Name, kind),
		Spec:            *spec,
		Profile:         filepath.Join(a.dataDir, "console", strconv.FormatInt(d.ID, 10)),
		CertFingerprint: cert.Fingerprint,
	})
}

// Overview summarizes all devices for the dashboard.
func (s *BMCService) Overview() (*Overview, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return nil, err
	}
	ctx := s.a.bg()
	devs, err := s.a.store.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	dtos, err := s.a.deviceDTOs(ctx, devs)
	if err != nil {
		return nil, err
	}
	ov := &Overview{Total: len(dtos), Problems: []DeviceDTO{}, Recent: []AuditEntry{}}
	vendors := map[string]int{}
	groups := map[string]int{}
	for _, d := range dtos {
		if d.Reachable {
			ov.Reachable++
		} else if d.CheckedAt > 0 {
			ov.Unreachable++
		}
		switch d.PowerState {
		case "on":
			ov.PoweredOn++
		case "off":
			ov.PoweredOff++
		}
		switch d.Health {
		case "ok":
			ov.HealthOK++
		case "warning":
			ov.Warning++
		case "critical":
			ov.Critical++
		default:
			ov.Unknown++
		}
		v := d.Vendor
		if v == "" {
			v = "unknown"
		}
		vendors[v]++
		g := d.GroupName
		if g == "" {
			g = "未分组"
		}
		groups[g]++
		if (!d.Reachable && d.CheckedAt > 0) || d.Health == "warning" || d.Health == "critical" {
			ov.Problems = append(ov.Problems, d)
		}
	}
	for k, n := range vendors {
		label := bmc.VendorName(k)
		if k == "unknown" {
			label = "未识别"
		}
		ov.ByVendor = append(ov.ByVendor, CountItem{Key: k, Label: label, Count: n})
	}
	for k, n := range groups {
		ov.ByGroup = append(ov.ByGroup, CountItem{Key: k, Label: k, Count: n})
	}
	byCount := func(items []CountItem) func(i, j int) bool {
		return func(i, j int) bool {
			if items[i].Count != items[j].Count {
				return items[i].Count > items[j].Count
			}
			return items[i].Label < items[j].Label
		}
	}
	sort.Slice(ov.ByVendor, byCount(ov.ByVendor))
	sort.Slice(ov.ByGroup, byCount(ov.ByGroup))
	sort.Slice(ov.Problems, func(i, j int) bool { return problemRank(ov.Problems[i]) < problemRank(ov.Problems[j]) })
	if len(ov.Problems) > 20 {
		ov.Problems = ov.Problems[:20]
	}
	if sess.Can(auth.PermAuditView) {
		if rows, _, err := s.a.store.ListAudit(ctx, store.AuditQuery{Limit: 8}); err == nil {
			ov.Recent = auditDTOs(rows)
		}
	}
	ov.LastPoll, ov.Polling = s.a.poller.state()
	return ov, nil
}

func problemRank(d DeviceDTO) int {
	switch {
	case d.Health == "critical":
		return 0
	case !d.Reachable:
		return 1
	}
	return 2
}
