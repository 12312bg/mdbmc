package bmc

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/bougou/go-ipmi/pkg/command/chassis"
	"github.com/bougou/go-ipmi/pkg/types"
)

// ipmiConn is an open RMCP+ session.
type ipmiConn struct {
	c *client.Client
}

func dialIPMI(ctx context.Context, t Target) (*ipmiConn, error) {
	timeout := t.IPMITimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	connect := func(priv types.PrivilegeLevel) (*client.Client, error) {
		c, err := client.NewClient(t.Host, t.IPMIPort, t.Username, t.Password)
		if err != nil {
			return nil, err
		}
		c.WithInterface(client.InterfaceLanplus)
		c.WithTimeout(timeout).WithRetry(1)
		c.WithMaxPrivilegeLevel(priv)
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
		return c, nil
	}
	c, err := connect(types.PrivilegeLevelAdministrator)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "privilege") {
		// The BMC account may only hold the Operator role.
		c, err = connect(types.PrivilegeLevelOperator)
	}
	if err != nil {
		return nil, err
	}
	return &ipmiConn{c: c}, nil
}

func (i *ipmiConn) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = i.c.Close(ctx)
}

type ipmiDeviceInfo struct {
	manufacturerID uint32
	productID      uint16
	firmware       string
	ipmiVersion    string
}

func (i *ipmiConn) deviceInfo(ctx context.Context) (*ipmiDeviceInfo, error) {
	res, err := i.c.GetDeviceID(ctx)
	if err != nil {
		return nil, err
	}
	return &ipmiDeviceInfo{
		manufacturerID: res.ManufacturerID,
		productID:      res.ProductID,
		firmware:       res.FirmwareVersionStr(),
		ipmiVersion:    fmt.Sprintf("%d.%d", res.MajorIPMIVersion, res.MinorIPMIVersion),
	}, nil
}

func (i *ipmiConn) status(ctx context.Context) (PowerState, Health, error) {
	res, err := i.c.GetChassisStatus(ctx)
	if err != nil {
		return PowerUnknown, HealthUnknown, err
	}
	power := PowerOff
	if res.PowerIsOn {
		power = PowerOn
	}
	health := HealthOK
	switch {
	case res.PowerFault, res.PowerControlFault, res.PowerOverload:
		health = HealthCritical
	case res.CollingFanFault, res.DriveFault:
		health = HealthWarning
	}
	return power, health, nil
}

func (i *ipmiConn) power(ctx context.Context, a PowerAction) error {
	var ctl chassis.ChassisControl
	switch a {
	case ActionOn:
		ctl = chassis.ChassisControlPowerUp
	case ActionOff:
		ctl = chassis.ChassisControlPowerDown
	case ActionSoft:
		ctl = chassis.ChassisControlSoftShutdown
	case ActionReset:
		ctl = chassis.ChassisControlHardReset
	case ActionCycle:
		ctl = chassis.ChassisControlPowerCycle
	default:
		return fmt.Errorf("未知电源操作 %q", a)
	}
	if !a.Idempotent() {
		// A retransmitted reset could reset the host a second time.
		i.c.WithRetry(0)
	}
	_, err := i.c.ChassisControl(ctx, ctl)
	return err
}

func (i *ipmiConn) identify(ctx context.Context, seconds uint8, force bool) error {
	_, err := i.c.ChassisIdentify(ctx, seconds, force)
	return err
}

func (i *ipmiConn) sensors(ctx context.Context) ([]Sensor, error) {
	var out []Sensor
	for r := range i.c.GetSensorsStream(ctx) {
		if r.Err != nil {
			if len(out) == 0 {
				return nil, r.Err
			}
			// Keep what was read before the failing record.
			break
		}
		if r.Ok != nil {
			out = append(out, convertIPMISensor(r.Ok))
		}
	}
	return out, nil
}

func convertIPMISensor(s *types.Sensor) Sensor {
	out := Sensor{
		Name:     s.Name,
		Type:     s.SensorType.String(),
		Category: ipmiCategory(s.SensorType),
		Source:   ProtocolIPMI,
	}
	switch {
	case s.NotPresent:
		out.Status = "absent"
		return out
	case s.ScanningDisabled || !s.IsReadingValid():
		out.Status = "unknown"
		return out
	}
	if s.IsThreshold() {
		v := round(s.Value)
		out.Reading = &v
		out.Unit = ipmiUnit(s)
		out.Status = thresholdStatus(string(s.Threshold.ThresholdStatus))
		th := func(tt types.SensorThresholdType, val float64) *float64 {
			if !s.IsThresholdReadable(tt) {
				return nil
			}
			r := round(val)
			return &r
		}
		out.LowerCritical = th(types.SensorThresholdType_LCR, s.Threshold.LCR)
		out.LowerWarning = th(types.SensorThresholdType_LNC, s.Threshold.LNC)
		out.UpperWarning = th(types.SensorThresholdType_UNC, s.Threshold.UNC)
		out.UpperCritical = th(types.SensorThresholdType_UCR, s.Threshold.UCR)
		return out
	}
	var texts []string
	for _, t := range s.DiscreteActiveEventsString() {
		if t != "" {
			texts = append(texts, t)
		}
	}
	out.Text = strings.Join(texts, ", ")
	out.Status = discreteStatus(out.Text)
	return out
}

// discreteStatus rates a discrete sensor by the names of its asserted
// states ("a, b, c"), keeping the most severe one.
func discreteStatus(text string) string {
	worst := "ok"
	for _, state := range strings.Split(strings.ToLower(text), ",") {
		switch rateState(strings.TrimSpace(state)) {
		case "critical":
			return "critical"
		case "warning":
			worst = "warning"
		}
	}
	return worst
}

func rateState(s string) string {
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(s, w) {
				return true
			}
		}
		return false
	}
	switch {
	case s == "":
		return "ok"
	// Degraded but still running: checked first so "Predictive Failure" and
	// "Redundancy Lost" are not caught by "fail"/"lost" below.
	case has("predictive", "redundancy", "degraded", "warning"):
		return "warning"
	case has("fail", "fault", "lost", "critical", "uncorrectable", "non-recoverable"):
		return "critical"
	}
	return "ok"
}

func thresholdStatus(st string) string {
	switch strings.ToLower(st) {
	case "ok":
		return "ok"
	case "lnc", "unc":
		return "warning"
	case "lcr", "ucr", "lnr", "unr":
		return "critical"
	}
	return "unknown"
}

func ipmiCategory(t types.SensorType) string {
	switch t {
	case types.SensorTypeTemperature:
		return "temperature"
	case types.SensorTypeFan:
		return "fan"
	case types.SensorTypeVoltage:
		return "voltage"
	case types.SensorTypeCurrent:
		return "current"
	case types.SensorTypePowerSupply, types.SensorTypePowerUnit:
		return "power"
	}
	return "other"
}

func ipmiUnit(s *types.Sensor) string {
	u := strings.TrimSpace(s.SensorUnit.String())
	switch strings.ToLower(u) {
	case "degrees c":
		return "°C"
	case "degrees f":
		return "°F"
	case "volts":
		return "V"
	case "amps":
		return "A"
	case "watts":
		return "W"
	case "rpm":
		return "RPM"
	case "unspecified":
		return ""
	}
	return u
}

func round(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func (i *ipmiConn) sel(ctx context.Context) ([]SELEntry, error) {
	if info, err := i.c.GetSELInfo(ctx); err == nil && info.Entries == 0 {
		return nil, nil
	}
	// Sensor names come from the SDR repository; it is optional.
	sdrMap, _ := i.c.GetSDRsMap(ctx)
	var out []SELEntry
	for r := range i.c.GetSELEntriesStream(ctx, 0) {
		if r.Err != nil {
			if len(out) == 0 {
				return nil, r.Err
			}
			break
		}
		if r.Ok != nil {
			out = append(out, convertSEL(r.Ok, sdrMap))
		}
	}
	return out, nil
}

func convertSEL(sel *types.SEL, sdrMap types.SDRMapBySensorNumber) SELEntry {
	e := SELEntry{ID: fmt.Sprintf("%04X", sel.RecordID), Source: ProtocolIPMI, Severity: "info"}
	switch {
	case sel.Standard != nil:
		s := sel.Standard
		if !s.Timestamp.IsZero() && s.Timestamp.Unix() > 0 {
			e.Time = s.Timestamp.Unix()
		}
		e.SensorType = s.SensorType.String()
		e.Message = s.EventString()
		if s.EventDir {
			e.Direction = "deassert"
		} else {
			e.Direction = "assert"
		}
		e.Severity = selSeverity(string(s.EventSeverity()))
		if m, ok := sdrMap[s.GeneratorID]; ok {
			if sdr, ok := m[s.SensorNumber]; ok && sdr != nil {
				e.Sensor = sdr.SensorName()
			}
		}
		if e.Sensor == "" {
			e.Sensor = fmt.Sprintf("#%02X", uint8(s.SensorNumber))
		}
	case sel.OEMTimestamped != nil:
		if ts := sel.OEMTimestamped.Timestamp; !ts.IsZero() {
			e.Time = ts.Unix()
		}
		e.SensorType = "OEM"
		e.Message = fmt.Sprintf("OEM 记录 (类型 %s)", sel.RecordType.String())
	default:
		e.SensorType = "OEM"
		e.Message = fmt.Sprintf("OEM 记录 (类型 %s)", sel.RecordType.String())
	}
	return e
}

func selSeverity(s string) string {
	switch strings.ToLower(s) {
	case "ok":
		return "ok"
	case "warning", "degraded", "non-fatal":
		return "warning"
	case "critical":
		return "critical"
	}
	return "info"
}

func (i *ipmiConn) clearSEL(ctx context.Context) error {
	res, err := i.c.ReserveSEL(ctx)
	if err != nil {
		return err
	}
	_, err = i.c.ClearSEL(ctx, res.ReservationID)
	return err
}

// baseboardFRU reads the manufacturer and product names of FRU device 0,
// the baseboard.
func (i *ipmiConn) baseboardFRU(ctx context.Context) (mfg []string, product string, err error) {
	f, err := i.c.GetFRU(ctx, 0, "baseboard")
	if err != nil {
		return nil, "", err
	}
	if a := f.ProductInfoArea; a != nil {
		mfg = append(mfg, types.FRUFieldString(a.ManufacturerTypeLength, a.Manufacturer))
		product = types.FRUFieldString(a.NameTypeLength, a.Name)
	}
	if a := f.BoardInfoArea; a != nil {
		mfg = append(mfg, types.FRUFieldString(a.ManufacturerTypeLength, a.Manufacturer))
		if product == "" {
			product = types.FRUFieldString(a.ProductNameTypeLength, a.ProductName)
		}
	}
	return mfg, strings.TrimSpace(product), nil
}

func (i *ipmiConn) fru(ctx context.Context) ([]FRUInfo, error) {
	frus, err := i.c.GetFRUs(ctx)
	if err != nil {
		return nil, err
	}
	var out []FRUInfo
	for _, f := range frus {
		if f == nil || !f.Present() {
			continue
		}
		info := FRUInfo{Device: fmt.Sprintf("%s (ID %d)", f.DeviceName, f.DeviceID)}
		if a := f.ChassisInfoArea; a != nil {
			info.ChassisPart = types.FRUFieldString(a.PartNumberTypeLength, a.PartNumber)
			info.ChassisSerial = types.FRUFieldString(a.SerialNumberTypeLength, a.SerialNumber)
		}
		if a := f.BoardInfoArea; a != nil {
			info.BoardMfg = types.FRUFieldString(a.ManufacturerTypeLength, a.Manufacturer)
			info.BoardProduct = types.FRUFieldString(a.ProductNameTypeLength, a.ProductName)
			info.BoardSerial = types.FRUFieldString(a.SerialNumberTypeLength, a.SerialNumber)
			info.BoardPart = types.FRUFieldString(a.PartNumberTypeLength, a.PartNumber)
		}
		if a := f.ProductInfoArea; a != nil {
			info.ProductMfg = types.FRUFieldString(a.ManufacturerTypeLength, a.Manufacturer)
			info.ProductName = types.FRUFieldString(a.NameTypeLength, a.Name)
			info.ProductPart = types.FRUFieldString(a.PartModelTypeLength, a.PartModel)
			info.ProductVersion = types.FRUFieldString(a.VersionTypeLength, a.Version)
			info.ProductSerial = types.FRUFieldString(a.SerialNumberTypeLength, a.SerialNumber)
		}
		out = append(out, info)
	}
	return out, nil
}
