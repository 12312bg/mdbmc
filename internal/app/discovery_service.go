package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/store"
)

var scanIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{4,40}$`)

// DiscoveryService scans the network for BMCs and adds them.
type DiscoveryService struct{ a *App }

type scanner struct {
	a      *App
	mu     sync.Mutex
	id     string
	stopFn context.CancelFunc
}

func (s *scanner) cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopFn != nil {
		s.stopFn()
		s.stopFn = nil
	}
}

// StartScan begins an asynchronous scan. Progress arrives as "scan:progress"
// events and each BMC found as a "scan:found" event.
func (d *DiscoveryService) StartScan(req ScanRequest) (*ScanStarted, error) {
	sess, err := d.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return nil, err
	}
	hosts, err := bmc.ExpandTargets(req.Targets, bmc.MaxScanHosts)
	if err != nil {
		return nil, err
	}
	if req.IPMIPort == 0 {
		req.IPMIPort = 623
	}
	if req.HTTPSPort == 0 {
		req.HTTPSPort = 443
	}
	managed, err := d.a.store.ManagedHosts(d.a.bg())
	if err != nil {
		return nil, err
	}
	sc := d.a.scanner
	sc.cancel()
	ctx, cancel := context.WithCancel(d.a.bg())
	id := req.ScanID
	if !scanIDRe.MatchString(id) {
		id = newID()
	}
	sc.mu.Lock()
	sc.id, sc.stopFn = id, cancel
	sc.mu.Unlock()
	d.a.audit(sess, "scan.start", strings.TrimSpace(req.Targets), nil, fmt.Sprintf("%d 个地址", len(hosts)))

	go func() {
		var mu sync.Mutex
		found := 0
		lastEmit := time.Time{}
		progress := func(done, total int, finished bool) {
			mu.Lock()
			defer mu.Unlock()
			if !finished && time.Since(lastEmit) < 150*time.Millisecond && done != total {
				return
			}
			lastEmit = time.Now()
			d.a.emit("scan:progress", ScanProgress{ScanID: id, Done: done, Total: total, Found: found,
				Finished: finished, Cancelled: finished && ctx.Err() != nil})
		}
		set := d.a.Settings()
		bmc.Scan(ctx, hosts, bmc.ScanOptions{
			IPMIPort: req.IPMIPort, HTTPSPort: req.HTTPSPort, Concurrency: set.ScanConcurrency,
		}, func(r bmc.ScanResult) {
			mu.Lock()
			found++
			mu.Unlock()
			d.a.emit("scan:found", ScanFound{
				ScanID: id, Host: r.Host, IPMI: r.IPMI, IPMIVersion: r.IPMIVersion, Redfish: r.Redfish,
				RedfishVersion: r.RedfishVersion, Vendor: r.Vendor, VendorName: r.VendorName, Product: r.Product,
				TLSFingerprint: r.TLSFingerprint, ManagedID: managed[strings.ToLower(r.Host)],
			})
		}, func(done, total int) { progress(done, total, false) })
		mu.Lock()
		n := found
		mu.Unlock()
		progress(len(hosts), len(hosts), true)
		d.a.audit(sess, "scan.finish", strings.TrimSpace(req.Targets), nil, fmt.Sprintf("发现 %d 台 BMC", n))
		sc.mu.Lock()
		if sc.id == id {
			sc.stopFn = nil
		}
		sc.mu.Unlock()
		cancel()
	}()
	return &ScanStarted{ScanID: id, Total: len(hosts)}, nil
}

// CancelScan stops the running scan.
func (d *DiscoveryService) CancelScan() error {
	if _, err := d.a.auth.Require(auth.PermDeviceManage); err != nil {
		return err
	}
	d.a.scanner.cancel()
	return nil
}

// AddDiscovered adds selected scan results as devices and identifies them.
func (d *DiscoveryService) AddDiscovered(req AddDiscoveredRequest) (*AddDiscoveredResult, error) {
	sess, err := d.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return nil, err
	}
	if len(req.Hosts) == 0 {
		return nil, errors.New("请选择要添加的设备")
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.CredentialID == 0 && req.Username == "" {
		return nil, errors.New("请选择凭据或填写 BMC 用户名")
	}
	if req.IPMIPort == 0 {
		req.IPMIPort = 623
	}
	if req.HTTPSPort == 0 {
		req.HTTPSPort = 443
	}
	ctx := d.a.bg()
	groups, err := d.a.groupsByName(ctx)
	if err != nil {
		return nil, err
	}
	res := &AddDiscoveredResult{Skipped: []string{}}
	var added []int64
	var pwEnc []byte
	if req.CredentialID == 0 {
		pwEnc = sess.Box().SealString(req.Password, purposeDevicePassword)
	}
	for _, h := range req.Hosts {
		name := strings.TrimSpace(h.Name)
		if name == "" {
			name = h.Host
		}
		groupID := req.GroupID
		if strings.TrimSpace(h.Group) != "" {
			if groupID, err = groups.id(ctx, h.Group); err != nil {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s：%v", h.Host, err))
				continue
			}
		}
		dev := &store.Device{
			Name: name, Host: h.Host, IPMIPort: req.IPMIPort, HTTPSPort: req.HTTPSPort, Protocol: string(bmc.ProtocolAuto),
			GroupID: groupID, CredentialID: req.CredentialID, Username: req.Username, PasswordEnc: pwEnc,
			Vendor: h.Vendor, HasIPMI: h.IPMI, HasRedfish: h.Redfish, TLSFingerprint: h.TLSFingerprint,
		}
		if req.CredentialID != 0 {
			dev.Username, dev.PasswordEnc = "", nil
		}
		if err := d.a.store.CreateDevice(ctx, dev); err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s：%v", h.Host, err))
			continue
		}
		added = append(added, dev.ID)
	}
	res.Added = len(added)
	d.a.audit(sess, "device.create", fmt.Sprintf("扫描添加 %d 台", res.Added), nil, strings.Join(res.Skipped, "；"))
	d.a.identify(sess, added)
	return res, nil
}
