package bmc

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Client runs operations against one BMC, choosing IPMI or Redfish per
// operation and falling back to the other protocol when the first fails.
// A Client is cheap; create one per operation and Close it afterwards.
type Client struct {
	t  Target
	ip *ipmiConn
	rf *rfConn

	// ObservedFingerprint is the TLS fingerprint seen by the last Redfish call.
	ObservedFingerprint string
}

// New returns a client for t.
func New(t Target) *Client {
	if t.IPMIPort == 0 {
		t.IPMIPort = 623
	}
	if t.HTTPSPort == 0 {
		t.HTTPSPort = 443
	}
	if !t.Protocol.Valid() {
		t.Protocol = ProtocolAuto
	}
	return &Client{t: t}
}

// Close releases open sessions.
func (c *Client) Close() {
	if c.ip != nil {
		c.ip.close()
		c.ip = nil
	}
	if c.rf != nil {
		c.ObservedFingerprint = c.rf.pin.Observed()
		c.rf.close()
		c.rf = nil
	}
}

func (c *Client) allowed(p Protocol) bool {
	switch c.t.Protocol {
	case ProtocolIPMI:
		return p == ProtocolIPMI
	case ProtocolRedfish:
		return p == ProtocolRedfish
	}
	if !c.t.HasIPMI && !c.t.HasRedfish {
		return true // never identified: try everything
	}
	return (p == ProtocolIPMI && c.t.HasIPMI) || (p == ProtocolRedfish && c.t.HasRedfish)
}

// order filters the preferred protocol order by mode and capabilities.
func (c *Client) order(pref ...Protocol) []Protocol {
	var out []Protocol
	for _, p := range pref {
		if c.allowed(p) {
			out = append(out, p)
		}
	}
	return out
}

func (c *Client) ipmi(ctx context.Context) (*ipmiConn, error) {
	if c.ip != nil {
		return c.ip, nil
	}
	ip, err := dialIPMI(ctx, c.t)
	if err != nil {
		return nil, err
	}
	c.ip = ip
	return ip, nil
}

func (c *Client) redfish(ctx context.Context) (*rfConn, error) {
	if c.rf != nil {
		return c.rf, nil
	}
	rf, err := dialRedfish(ctx, c.t)
	if err != nil {
		return nil, err
	}
	c.rf = rf
	return rf, nil
}

// try runs fn for each protocol in order until one succeeds.
func (c *Client) try(ctx context.Context, order []Protocol, ipmiFn func(*ipmiConn) error, rfFn func(*rfConn) error) (Protocol, error) {
	if len(order) == 0 {
		return "", ErrNotSupported
	}
	var errs []error
	for _, p := range order {
		if err := ctx.Err(); err != nil {
			errs = append(errs, friendly(p, err))
			break
		}
		var err error
		switch p {
		case ProtocolIPMI:
			if ipmiFn == nil {
				continue
			}
			var ip *ipmiConn
			if ip, err = c.ipmi(ctx); err == nil {
				err = ipmiFn(ip)
			}
		case ProtocolRedfish:
			if rfFn == nil {
				continue
			}
			var rf *rfConn
			if rf, err = c.redfish(ctx); err == nil {
				err = rfFn(rf)
			}
			if c.rf != nil {
				c.ObservedFingerprint = c.rf.pin.Observed()
			}
		}
		if err == nil {
			return p, nil
		}
		var mm *FingerprintMismatchError
		if errors.As(err, &mm) {
			return p, err
		}
		errs = append(errs, friendly(p, err))
	}
	if len(errs) == 0 {
		return "", ErrNotSupported
	}
	return "", joinErrors(errs)
}

// Identify detects protocols, vendor and identity. It tries both protocols
// regardless of the configured mode so the capabilities can be refreshed.
func (c *Client) Identify(ctx context.Context) (*Identity, error) {
	id := &Identity{PowerState: PowerUnknown, Health: HealthUnknown}
	var errs []error
	var fruProduct string

	if c.t.Protocol != ProtocolRedfish {
		if ip, err := c.ipmi(ctx); err == nil {
			id.HasIPMI = true
			if info, err := ip.deviceInfo(ctx); err == nil {
				id.ManufacturerID = info.manufacturerID
				id.IPMIVersion = info.ipmiVersion
				id.BMCFirmware = info.firmware
			} else {
				id.Warnings = append(id.Warnings, "IPMI Get Device ID 失败: "+err.Error())
			}
			if ps, h, err := ip.status(ctx); err == nil {
				id.PowerState, id.Health = ps, h
			}
			if mfg, product, err := ip.baseboardFRU(ctx); err == nil {
				id.fruNames, fruProduct = mfg, product
			}
		} else {
			errs = append(errs, friendly(ProtocolIPMI, err))
		}
	}

	if c.t.Protocol != ProtocolIPMI {
		if rf, err := c.redfish(ctx); err == nil {
			ipmiFW, ipmiHealth, ipmiPower := id.BMCFirmware, id.Health, id.PowerState
			id.Warnings = append(id.Warnings, rf.identity(id)...)
			if id.BMCFirmware == "" {
				id.BMCFirmware = ipmiFW
			}
			if id.PowerState == PowerUnknown {
				id.PowerState = ipmiPower
			}
			if id.Health == HealthUnknown {
				id.Health = ipmiHealth
			}
			id.TLSFingerprint = rf.pin.Observed()
			c.ObservedFingerprint = id.TLSFingerprint
		} else {
			var mm *FingerprintMismatchError
			if errors.As(err, &mm) {
				return nil, err
			}
			errs = append(errs, friendly(ProtocolRedfish, err))
		}
	}

	if !id.HasIPMI && !id.HasRedfish {
		return nil, joinErrors(errs)
	}
	for _, m := range id.fruNames {
		if m = strings.TrimSpace(m); id.Manufacturer == "" && m != "" {
			id.Manufacturer = m
		}
	}
	if id.Model == "" {
		id.Model = fruProduct
	}
	if id.Vendor == "" {
		id.Vendor = DetectVendor(id.ManufacturerID, append([]string{id.Manufacturer}, id.fruNames...)...)
	}
	id.VendorName = VendorName(id.Vendor)
	for _, e := range errs {
		id.Warnings = append(id.Warnings, e.Error())
	}
	return id, nil
}

// Status polls power and health with the cheapest available call.
func (c *Client) Status(ctx context.Context) Status {
	st := Status{Power: PowerUnknown, Health: HealthUnknown}
	// Redfish reports a real health rollup; IPMI only chassis fault bits.
	via, err := c.try(ctx, c.order(ProtocolRedfish, ProtocolIPMI),
		func(ip *ipmiConn) (err error) {
			st.Power, st.Health, err = ip.status(ctx)
			return
		},
		func(rf *rfConn) (err error) {
			st.Power, st.Health, err = rf.status()
			return
		})
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Reachable, st.Via = true, via
	return st
}

// Power performs a power action.
func (c *Client) Power(ctx context.Context, a PowerAction) (Protocol, error) {
	if !a.Valid() {
		return "", fmt.Errorf("未知电源操作 %q", a)
	}
	var errs []error
	for _, p := range c.order(ProtocolIPMI, ProtocolRedfish) {
		var sent bool
		var err error
		switch p {
		case ProtocolIPMI:
			var ip *ipmiConn
			if ip, err = c.ipmi(ctx); err == nil {
				sent, err = true, ip.power(ctx, a)
			}
		case ProtocolRedfish:
			var rf *rfConn
			if rf, err = c.redfish(ctx); err == nil {
				sent, err = true, rf.power(a)
			}
			if c.rf != nil {
				c.ObservedFingerprint = c.rf.pin.Observed()
			}
		}
		if err == nil {
			return p, nil
		}
		var mm *FingerprintMismatchError
		if errors.As(err, &mm) {
			return p, err
		}
		errs = append(errs, friendly(p, err))
		// A reset or power cycle whose reply was lost may already have
		// happened; repeating it over the other protocol could hit the host
		// twice. Only fall back when the command never reached the BMC.
		if sent && !a.Idempotent() {
			break
		}
	}
	if len(errs) == 0 {
		return "", ErrNotSupported
	}
	return "", joinErrors(errs)
}

// Identify LED: seconds 0 turns it off; force keeps it on indefinitely.
func (c *Client) IdentifyLED(ctx context.Context, seconds uint8, force bool) error {
	_, err := c.try(ctx, c.order(ProtocolIPMI),
		func(ip *ipmiConn) error { return ip.identify(ctx, seconds, force) }, nil)
	return err
}

// Sensors reads all sensors.
func (c *Client) Sensors(ctx context.Context) ([]Sensor, Protocol, error) {
	var out []Sensor
	via, err := c.try(ctx, c.order(ProtocolIPMI, ProtocolRedfish),
		func(ip *ipmiConn) (err error) { out, err = ip.sensors(ctx); return },
		func(rf *rfConn) (err error) { out, err = rf.sensors(); return })
	slices.SortStableFunc(out, func(a, b Sensor) int {
		if a.Category != b.Category {
			return strings.Compare(categoryRank(a.Category), categoryRank(b.Category))
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, via, err
}

func categoryRank(c string) string {
	switch c {
	case "temperature":
		return "1"
	case "fan":
		return "2"
	case "voltage":
		return "3"
	case "current":
		return "4"
	case "power":
		return "5"
	}
	return "9"
}

// SEL reads the system event log, newest first.
func (c *Client) SEL(ctx context.Context) ([]SELEntry, Protocol, error) {
	var out []SELEntry
	via, err := c.try(ctx, c.order(ProtocolIPMI, ProtocolRedfish),
		func(ip *ipmiConn) (err error) { out, err = ip.sel(ctx); return },
		func(rf *rfConn) (err error) { out, err = rf.sel(); return })
	slices.Reverse(out)
	return out, via, err
}

// ClearSEL clears the system event log.
func (c *Client) ClearSEL(ctx context.Context) (Protocol, error) {
	return c.try(ctx, c.order(ProtocolIPMI, ProtocolRedfish),
		func(ip *ipmiConn) error { return ip.clearSEL(ctx) },
		func(rf *rfConn) error { return rf.clearSEL() })
}

// Inventory collects hardware inventory. Redfish provides the detail; IPMI
// FRU data is added when IPMI is available, and is all there is otherwise.
func (c *Client) Inventory(ctx context.Context) (*Inventory, error) {
	inv := &Inventory{CollectedAt: time.Now().Unix()}
	var errs []error
	if c.allowed(ProtocolRedfish) {
		if rf, err := c.redfish(ctx); err == nil {
			rf.inventory(inv)
			inv.Source = ProtocolRedfish
		} else {
			var mm *FingerprintMismatchError
			if errors.As(err, &mm) {
				return nil, err
			}
			errs = append(errs, friendly(ProtocolRedfish, err))
		}
	}
	rfErrs := len(errs)
	if c.allowed(ProtocolIPMI) {
		if ip, err := c.ipmi(ctx); err == nil {
			if fru, err := ip.fru(ctx); err == nil {
				inv.FRU = fru
				if inv.Source == "" {
					inv.Source = ProtocolIPMI
				}
			} else {
				errs = append(errs, friendly(ProtocolIPMI, err))
			}
		} else {
			errs = append(errs, friendly(ProtocolIPMI, err))
		}
	}
	if inv.Source == "" {
		if err := joinErrors(errs); err != nil {
			return nil, err
		}
		return nil, ErrNotSupported
	}
	// The failed protocol's sections say why they are empty.
	for i, e := range errs {
		if i < rfErrs {
			for _, s := range redfishSections {
				inv.fail(s, e.Error())
			}
		} else {
			inv.fail("fru", e.Error())
		}
	}
	return inv, nil
}

// WebURL is the BMC web UI address.
func (c *Client) WebURL() string {
	return baseURL(c.t.Host, c.t.HTTPSPort) + "/"
}

// KVMLaunch works out how to open the remote console for the vendor.
func (c *Client) KVMLaunch(ctx context.Context) (*LaunchSpec, error) {
	base := baseURL(c.t.Host, c.t.HTTPSPort)
	switch c.t.Vendor {
	case VendorSupermicro:
		return c.supermicroKVM(ctx, base)
	case VendorInspur:
		// AMI MegaRAC firmware with the Java console (JViewer): after the
		// login the window opens its JNLP, which the console window runs
		// with the bundled Java runtime.
		return &LaunchSpec{
			URL:           base + "/",
			AutoLogin:     true,
			AfterLoginURL: base + "/Java/jviewer.jnlp?EXTRNIP=" + url.QueryEscape(c.t.Host) + "&JNLPSTR=JViewer",
			Hint:          "登录 BMC 后会自动启动 Java 远程控制台，首次启动需要从 BMC 下载程序",
		}, nil
	}
	hint := VendorName(c.t.Vendor) + " 的一键 KVM 尚未适配"
	if c.t.Vendor == "" || c.t.Vendor == VendorGeneric {
		hint = "未识别出该设备的品牌，可在设备详情中点“重新识别”"
	}
	return &LaunchSpec{
		URL:       base + "/",
		AutoLogin: true,
		Hint:      "已打开 BMC Web 界面，请在页面中启动远程控制台，Java 版控制台会直接打开（" + hint + "）",
	}, nil
}
