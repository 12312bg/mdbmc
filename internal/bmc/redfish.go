package bmc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

// rfConn is an authenticated Redfish client bound to one operation context.
type rfConn struct {
	api    *gofish.APIClient
	svc    *gofish.Service
	pin    *pinner
	system *schemas.ComputerSystem
}

func dialRedfish(ctx context.Context, t Target) (*rfConn, error) {
	timeout := t.RedfishTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	p := &pinner{expected: t.TLSFingerprint}
	api, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:          baseURL(t.Host, t.HTTPSPort),
		Username:          t.Username,
		Password:          t.Password,
		HTTPClient:        newHTTPClient(p, timeout),
		BasicAuth:         true, // avoids exhausting the BMC's small session table
		NoModifyTransport: true,
	})
	if err != nil {
		return nil, p.mismatchOr(err)
	}
	r := &rfConn{api: api, svc: api.Service, pin: p}
	// The service root is usually readable without credentials, so fetch the
	// systems collection to make sure the credentials are actually accepted.
	systems, err := r.svc.Systems()
	if err != nil {
		api.Logout()
		return nil, p.mismatchOr(err)
	}
	if len(systems) > 0 {
		r.system = systems[0]
	}
	return r, nil
}

func (r *rfConn) close() {
	r.api.Logout()
}

func (r *rfConn) firstSystem() (*schemas.ComputerSystem, error) {
	if r.system != nil {
		return r.system, nil
	}
	systems, err := r.svc.Systems()
	if err != nil {
		return nil, err
	}
	if len(systems) == 0 {
		return nil, fmt.Errorf("Redfish 服务中没有 ComputerSystem 资源")
	}
	r.system = systems[0]
	return r.system, nil
}

func (r *rfConn) firstManager() (*schemas.Manager, error) {
	managers, err := r.svc.Managers()
	if err != nil {
		return nil, err
	}
	if len(managers) == 0 {
		return nil, fmt.Errorf("Redfish 服务中没有 Manager 资源")
	}
	return managers[0], nil
}

func rfPower(ps schemas.PowerState) PowerState {
	switch ps {
	case schemas.OnPowerState, schemas.PoweringOffPowerState:
		return PowerOn
	case schemas.OffPowerState, schemas.PoweringOnPowerState:
		return PowerOff
	}
	return PowerUnknown
}

func rfHealth(st schemas.Status) Health {
	h := st.HealthRollup
	if h == "" {
		h = st.Health
	}
	switch h {
	case schemas.OKHealth:
		return HealthOK
	case schemas.WarningHealth:
		return HealthWarning
	case schemas.CriticalHealth:
		return HealthCritical
	}
	return HealthUnknown
}

func rfHealthStr(st schemas.Status) string {
	if st.State == "Absent" {
		return "absent"
	}
	return string(rfHealth(st))
}

func (r *rfConn) status() (PowerState, Health, error) {
	sys, err := r.firstSystem()
	if err != nil {
		return PowerUnknown, HealthUnknown, err
	}
	return rfPower(sys.PowerState), rfHealth(sys.Status), nil
}

func (r *rfConn) power(a PowerAction) error {
	sys, err := r.firstSystem()
	if err != nil {
		return err
	}
	var rt schemas.ResetType
	switch a {
	case ActionOn:
		rt = schemas.OnResetType
	case ActionOff:
		rt = schemas.ForceOffResetType
	case ActionSoft:
		rt = schemas.GracefulShutdownResetType
	case ActionReset:
		rt = schemas.ForceRestartResetType
	case ActionCycle:
		rt = schemas.PowerCycleResetType
	default:
		return fmt.Errorf("未知电源操作 %q", a)
	}
	_, err = sys.Reset(rt)
	return err
}

// serviceRootInfo is the unauthenticated part of /redfish/v1.
type serviceRootInfo struct {
	RedfishVersion string
	Vendor         string
	Product        string
	UUID           string
	OemKeys        []string
}

func parseServiceRoot(b []byte) (*serviceRootInfo, error) {
	var raw struct {
		RedfishVersion string
		Vendor         string
		Product        string
		UUID           string
		Oem            map[string]json.RawMessage
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw.RedfishVersion == "" {
		return nil, fmt.Errorf("不是 Redfish 服务根")
	}
	info := &serviceRootInfo{RedfishVersion: raw.RedfishVersion, Vendor: raw.Vendor, Product: raw.Product, UUID: raw.UUID}
	for k := range raw.Oem {
		info.OemKeys = append(info.OemKeys, k)
	}
	slices.Sort(info.OemKeys)
	return info, nil
}

// probeServiceRoot reads /redfish/v1/ without credentials.
func probeServiceRoot(ctx context.Context, host string, port int, timeout time.Duration) (*serviceRootInfo, string, error) {
	p := &pinner{}
	hc := newHTTPClient(p, timeout)
	defer hc.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL(host, port)+"/redfish/v1/", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, p.Observed(), err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, p.Observed(), err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, p.Observed(), fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	info, err := parseServiceRoot(body)
	return info, p.Observed(), err
}

func (r *rfConn) identity(id *Identity) []string {
	var warns []string
	id.HasRedfish = true
	id.RedfishVersion = r.svc.RedfishVersion
	if sys, err := r.firstSystem(); err == nil {
		id.Manufacturer = strings.TrimSpace(sys.Manufacturer)
		id.Model = strings.TrimSpace(sys.Model)
		id.Serial = strings.TrimSpace(sys.SerialNumber)
		id.BIOSVersion = strings.TrimSpace(sys.BiosVersion)
		id.PowerState = rfPower(sys.PowerState)
		id.Health = rfHealth(sys.Status)
	} else {
		warns = append(warns, "读取 Redfish 系统信息失败: "+err.Error())
	}
	if m, err := r.firstManager(); err == nil {
		id.BMCFirmware = strings.TrimSpace(m.FirmwareVersion)
		if id.Manufacturer == "" {
			id.Manufacturer = strings.TrimSpace(m.Manufacturer)
		}
	} else {
		warns = append(warns, "读取 Redfish BMC 信息失败: "+err.Error())
	}
	var oemKeys []string
	if len(r.svc.OEM) > 0 {
		var oem map[string]json.RawMessage
		if json.Unmarshal(r.svc.OEM, &oem) == nil {
			for k := range oem {
				oemKeys = append(oemKeys, k)
			}
		}
	}
	names := append([]string{id.Manufacturer, r.svc.Vendor, r.svc.Product}, oemKeys...)
	names = append(names, id.fruNames...)
	id.Vendor = DetectVendor(id.ManufacturerID, names...)
	return warns
}

func f64(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := round(*p)
	return &v
}

func f32(p *float32) *float64 {
	if p == nil {
		return nil
	}
	v := round(float64(*p))
	return &v
}

func iptr(p *int) *float64 {
	if p == nil {
		return nil
	}
	v := float64(*p)
	return &v
}

func sensorStatus(st schemas.Status) string {
	if st.State == "Absent" {
		return "absent"
	}
	switch st.Health {
	case schemas.OKHealth:
		return "ok"
	case schemas.WarningHealth:
		return "warning"
	case schemas.CriticalHealth:
		return "critical"
	}
	return "unknown"
}

func (r *rfConn) sensors() ([]Sensor, error) {
	chassis, err := r.svc.Chassis()
	if err != nil {
		return nil, err
	}
	var out []Sensor
	var lastErr error
	for _, ch := range chassis {
		if th, err := ch.Thermal(); err == nil && th != nil {
			for _, t := range th.Temperatures {
				if t.Status.State == "Absent" {
					continue
				}
				out = append(out, Sensor{
					Name: t.Name, Category: "temperature", Type: string(t.PhysicalContext),
					Reading: f64(t.ReadingCelsius), Unit: "°C", Status: sensorStatus(t.Status),
					LowerCritical: f64(t.LowerThresholdCritical), LowerWarning: f64(t.LowerThresholdNonCritical),
					UpperWarning: f64(t.UpperThresholdNonCritical), UpperCritical: f64(t.UpperThresholdCritical),
					Source: ProtocolRedfish,
				})
			}
			for _, f := range th.Fans {
				if f.Status.State == "Absent" {
					continue
				}
				name := f.Name
				if name == "" {
					name = f.FanName
				}
				unit := string(f.ReadingUnits)
				if unit == "" || unit == "RPM" {
					unit = "RPM"
				} else if unit == "Percent" {
					unit = "%"
				}
				out = append(out, Sensor{
					Name: name, Category: "fan", Type: "Fan", Reading: iptr(f.Reading), Unit: unit,
					Status:        sensorStatus(f.Status),
					LowerCritical: iptr(f.LowerThresholdCritical), LowerWarning: iptr(f.LowerThresholdNonCritical),
					UpperWarning: iptr(f.UpperThresholdNonCritical), UpperCritical: iptr(f.UpperThresholdCritical),
					Source: ProtocolRedfish,
				})
			}
		} else if err != nil {
			lastErr = err
		}
		if pw, err := ch.Power(); err == nil && pw != nil {
			for _, v := range pw.Voltages {
				if v.Status.State == "Absent" {
					continue
				}
				out = append(out, Sensor{
					Name: v.Name, Category: "voltage", Type: string(v.PhysicalContext), Reading: f32(v.ReadingVolts),
					Unit: "V", Status: sensorStatus(v.Status),
					LowerCritical: f32(v.LowerThresholdCritical), LowerWarning: f32(v.LowerThresholdNonCritical),
					UpperWarning: f32(v.UpperThresholdNonCritical), UpperCritical: f32(v.UpperThresholdCritical),
					Source: ProtocolRedfish,
				})
			}
			for _, pc := range pw.PowerControl {
				if pc.PowerConsumedWatts == nil {
					continue
				}
				name := pc.Name
				if name == "" {
					name = "System Power"
				}
				out = append(out, Sensor{
					Name: name, Category: "power", Type: "PowerControl", Reading: f32(pc.PowerConsumedWatts),
					Unit: "W", Status: sensorStatus(pc.Status), Source: ProtocolRedfish,
				})
			}
			for _, ps := range pw.PowerSupplies {
				st := sensorStatus(ps.Status)
				if st == "absent" {
					continue
				}
				s := Sensor{Name: ps.Name, Category: "power", Type: "PowerSupply", Status: st, Source: ProtocolRedfish}
				if ps.PowerInputWatts != nil {
					s.Reading, s.Unit = f32(ps.PowerInputWatts), "W"
				} else if ps.LastPowerOutputWatts != nil {
					s.Reading, s.Unit = f32(ps.LastPowerOutputWatts), "W"
				}
				out = append(out, s)
			}
		} else if err != nil {
			lastErr = err
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

// eventLogPreference orders log service ids by how likely they hold the
// hardware event log.
var eventLogPreference = []string{"sel", "log1", "iel", "iml", "eventlog", "event", "bmc", "system"}

func (r *rfConn) eventLog() (*schemas.LogService, error) {
	var all []*schemas.LogService
	if m, err := r.firstManager(); err == nil {
		if ls, err := m.LogServices(); err == nil {
			all = append(all, ls...)
		}
	}
	if sys, err := r.firstSystem(); err == nil {
		if ls, err := sys.LogServices(); err == nil {
			all = append(all, ls...)
		}
	}
	if len(all) == 0 {
		return nil, ErrNotSupported
	}
	for _, pref := range eventLogPreference {
		for _, l := range all {
			if strings.EqualFold(l.ID, pref) || strings.Contains(strings.ToLower(l.Name), pref) {
				return l, nil
			}
		}
	}
	return all[0], nil
}

func (r *rfConn) sel() ([]SELEntry, error) {
	log, err := r.eventLog()
	if err != nil {
		return nil, err
	}
	entries, err := log.Entries()
	if err != nil {
		return nil, err
	}
	out := make([]SELEntry, 0, len(entries))
	for _, e := range entries {
		ts := e.Created
		if ts == "" {
			ts = e.EventTimestamp
		}
		var unix int64
		if t, err := time.Parse(time.RFC3339, ts); err == nil && t.Year() > 1971 {
			unix = t.Unix()
		}
		sev := "info"
		switch strings.ToLower(string(e.Severity)) {
		case "ok":
			sev = "ok"
		case "warning":
			sev = "warning"
		case "critical":
			sev = "critical"
		}
		sensor := e.Name
		if e.SensorNumber != nil && sensor == "" {
			sensor = fmt.Sprintf("#%02X", *e.SensorNumber)
		}
		out = append(out, SELEntry{
			ID: e.ID, Time: unix, Sensor: sensor, SensorType: string(e.SensorType),
			Message: strings.TrimSpace(e.Message), Severity: sev, Source: ProtocolRedfish,
		})
	}
	return out, nil
}

func (r *rfConn) clearSEL() error {
	log, err := r.eventLog()
	if err != nil {
		return err
	}
	_, err = log.ClearLog("")
	return err
}

func uintVal(p *uint) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func intVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func (r *rfConn) inventory(inv *Inventory) {
	warn := func(what string, err error) {
		inv.Warnings = append(inv.Warnings, fmt.Sprintf("%s: %v", what, err))
	}
	sys, err := r.firstSystem()
	if err != nil {
		warn("系统信息", err)
	} else {
		inv.System = SystemInfo{
			Manufacturer: sys.Manufacturer, Model: sys.Model, Serial: sys.SerialNumber, SKU: sys.SKU,
			PartNumber: sys.PartNumber, UUID: sys.UUID, HostName: sys.HostName, BIOSVersion: sys.BiosVersion,
			PowerState: string(rfPower(sys.PowerState)), Health: string(rfHealth(sys.Status)),
			ProcessorModel: sys.ProcessorSummary.Model, ProcessorCount: uintVal(sys.ProcessorSummary.Count),
			CoreCount: uintVal(sys.ProcessorSummary.CoreCount), ThreadCount: uintVal(sys.ProcessorSummary.LogicalProcessorCount),
		}
		if sys.MemorySummary.TotalSystemMemoryGiB != nil {
			inv.System.MemoryGiB = math.Round(*sys.MemorySummary.TotalSystemMemoryGiB*10) / 10
		}
		if procs, err := sys.Processors(); err == nil {
			for _, p := range procs {
				if p.Status.State == "Absent" || (p.Model == "" && p.TotalCores == nil) {
					continue
				}
				inv.Processors = append(inv.Processors, Processor{
					Socket: firstNonEmpty(p.Socket, p.Name, p.ID), Model: strings.TrimSpace(p.Model), Manufacturer: p.Manufacturer,
					Cores: intVal(p.TotalCores), Threads: intVal(p.TotalThreads), MaxSpeedMHz: intVal(p.MaxSpeedMHz),
					Health: rfHealthStr(p.Status),
				})
			}
		} else {
			warn("处理器", err)
		}
		if mems, err := sys.Memory(); err == nil {
			for _, m := range mems {
				if m.Status.State == "Absent" || intVal(m.CapacityMiB) == 0 {
					continue
				}
				inv.Memory = append(inv.Memory, MemoryDIMM{
					Slot: firstNonEmpty(m.DeviceLocator, m.Name, m.ID), CapacityMiB: intVal(m.CapacityMiB),
					Type: string(m.MemoryDeviceType), SpeedMHz: intVal(m.OperatingSpeedMhz), Manufacturer: m.Manufacturer,
					PartNumber: strings.TrimSpace(m.PartNumber), Serial: m.SerialNumber, Health: rfHealthStr(m.Status),
				})
			}
		} else {
			warn("内存", err)
		}
		if storages, err := sys.Storage(); err == nil {
			for _, st := range storages {
				drives, err := st.Drives()
				if err != nil {
					warn("磁盘", err)
					continue
				}
				for _, d := range drives {
					if d.Status.State == "Absent" {
						continue
					}
					inv.Drives = append(inv.Drives, Drive{
						Name: firstNonEmpty(d.Name, d.ID), Model: strings.TrimSpace(d.Model), Serial: strings.TrimSpace(d.SerialNumber),
						CapacityBytes: int64(intVal(d.CapacityBytes)), MediaType: string(d.MediaType), Protocol: string(d.Protocol),
						Firmware: d.Revision, Health: rfHealthStr(d.Status),
					})
				}
			}
		} else {
			warn("存储", err)
		}
		if nics, err := sys.EthernetInterfaces(); err == nil {
			for _, n := range nics {
				nic := NIC{
					Name: firstNonEmpty(n.Name, n.ID), MACAddress: firstNonEmpty(n.MACAddress, n.PermanentMACAddress),
					SpeedMbps: intVal(n.SpeedMbps), LinkStatus: string(n.LinkStatus), Health: rfHealthStr(n.Status),
				}
				for _, a := range n.IPv4Addresses {
					if a.Address != "" && a.Address != "0.0.0.0" {
						nic.IPv4 = append(nic.IPv4, a.Address)
					}
				}
				inv.NICs = append(inv.NICs, nic)
			}
		} else {
			warn("网卡", err)
		}
	}

	if m, err := r.firstManager(); err == nil {
		inv.Manager.Model = m.Model
		inv.Manager.FirmwareVersion = m.FirmwareVersion
		if eths, err := m.EthernetInterfaces(); err == nil {
			for _, e := range eths {
				if inv.Manager.MACAddress == "" {
					inv.Manager.MACAddress = e.MACAddress
				}
				for _, a := range e.IPv4Addresses {
					if inv.Manager.IPv4 == "" && a.Address != "" && a.Address != "0.0.0.0" {
						inv.Manager.IPv4 = a.Address
					}
				}
			}
		}
	} else {
		warn("BMC 信息", err)
	}

	if chassis, err := r.svc.Chassis(); err == nil {
		for _, ch := range chassis {
			pw, err := ch.Power()
			if err != nil || pw == nil {
				continue
			}
			for _, ps := range pw.PowerSupplies {
				if ps.Status.State == "Absent" {
					continue
				}
				p := PowerSupply{
					Name: firstNonEmpty(ps.Name, ps.MemberID), Model: strings.TrimSpace(ps.Model), Serial: strings.TrimSpace(ps.SerialNumber),
					Firmware: ps.FirmwareVersion, Health: rfHealthStr(ps.Status),
				}
				if ps.PowerCapacityWatts != nil {
					p.CapacityWatts = float64(*ps.PowerCapacityWatts)
				}
				inv.PowerSupplies = append(inv.PowerSupplies, p)
			}
		}
	}

	if us, err := r.svc.UpdateService(); err == nil && us != nil {
		if fws, err := us.FirmwareInventory(); err == nil {
			for _, fw := range fws {
				if fw.Version == "" {
					continue
				}
				inv.Firmware = append(inv.Firmware, Firmware{Name: firstNonEmpty(fw.Name, fw.ID), Version: fw.Version, Updateable: fw.Updateable})
			}
		}
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// getJSON GETs a resource path and decodes it into v.
func (r *rfConn) getJSON(path string, v any) error {
	resp, err := r.api.Get(path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}
