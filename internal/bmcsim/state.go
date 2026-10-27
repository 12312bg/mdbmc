package bmcsim

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/bougou/go-ipmi/pkg/hal"
	"github.com/bougou/go-ipmi/pkg/handlers"
	"github.com/bougou/go-ipmi/pkg/types"
)

type simSensor struct {
	num   uint8
	name  string
	stype types.SensorType
	unit  types.SensorUnitType
	// Linear conversion: value = M * raw * 10^rexp.
	m    int16
	rexp int8
	// base and jitter of the simulated reading; offWhenPowered means the
	// reading drops to zero while the host is off (fans, CPU temps).
	base, jitter float64
	poweredOnly  bool
	// thresholds in real units; 0 = not readable
	lcr, lnc, unc, ucr float64
	// discrete sensors (sensor-specific); state is the active offset bit mask
	discrete bool
	state    uint16
}

type selRecord struct {
	id  uint16
	sel *types.SEL
}

type state struct {
	mu      sync.Mutex
	index   int
	hal     hal.HAL
	sensors []*simSensor
	sel     []selRecord
	nextSEL uint16
	resv    uint16
	erased  time.Time
	start   time.Time
}

func newState(index int, h hal.HAL) *state {
	st := &state{index: index, hal: h, nextSEL: 1, start: time.Now()}
	st.sensors = []*simSensor{
		{num: 0x01, name: "CPU1 Temp", stype: types.SensorTypeTemperature, unit: types.SensorUnitType_DegreesC, m: 1, base: 46, jitter: 3, poweredOnly: true, unc: 90, ucr: 95},
		{num: 0x02, name: "CPU2 Temp", stype: types.SensorTypeTemperature, unit: types.SensorUnitType_DegreesC, m: 1, base: 49, jitter: 3, poweredOnly: true, unc: 90, ucr: 95},
		{num: 0x03, name: "System Temp", stype: types.SensorTypeTemperature, unit: types.SensorUnitType_DegreesC, m: 1, base: 31, jitter: 1, unc: 80, ucr: 85},
		{num: 0x04, name: "Peripheral Temp", stype: types.SensorTypeTemperature, unit: types.SensorUnitType_DegreesC, m: 1, base: 39, jitter: 1, unc: 80, ucr: 85},
		{num: 0x05, name: "Inlet Temp", stype: types.SensorTypeTemperature, unit: types.SensorUnitType_DegreesC, m: 1, base: 24, jitter: 1, unc: 40, ucr: 45},
		{num: 0x41, name: "FAN1", stype: types.SensorTypeFan, unit: types.SensorUnitType_RPM, m: 100, base: 8400, jitter: 200, poweredOnly: true, lcr: 420, lnc: 700},
		{num: 0x42, name: "FAN2", stype: types.SensorTypeFan, unit: types.SensorUnitType_RPM, m: 100, base: 8200, jitter: 200, poweredOnly: true, lcr: 420, lnc: 700},
		{num: 0x43, name: "FAN3", stype: types.SensorTypeFan, unit: types.SensorUnitType_RPM, m: 100, base: 8300, jitter: 200, poweredOnly: true, lcr: 420, lnc: 700},
		{num: 0x44, name: "FAN4", stype: types.SensorTypeFan, unit: types.SensorUnitType_RPM, m: 100, base: 600, jitter: 0, poweredOnly: true, lcr: 420, lnc: 700},
		{num: 0x30, name: "12V", stype: types.SensorTypeVoltage, unit: types.SensorUnitType_Volts, m: 6, rexp: -2, base: 12.06, jitter: 0.06, lcr: 10.8, lnc: 11.22, unc: 12.78, ucr: 13.2},
		{num: 0x31, name: "5VCC", stype: types.SensorTypeVoltage, unit: types.SensorUnitType_Volts, m: 3, rexp: -2, base: 5.04, jitter: 0.03, lcr: 4.5, lnc: 4.68, unc: 5.31, ucr: 5.49},
		{num: 0x32, name: "3.3VCC", stype: types.SensorTypeVoltage, unit: types.SensorUnitType_Volts, m: 2, rexp: -2, base: 3.32, jitter: 0.02, lcr: 2.96, lnc: 3.06, unc: 3.54, ucr: 3.64},
		{num: 0x33, name: "VBAT", stype: types.SensorTypeVoltage, unit: types.SensorUnitType_Volts, m: 2, rexp: -2, base: 3.1, jitter: 0, lcr: 2.5, lnc: 2.7},
		{num: 0x50, name: "PS1 Input Power", stype: types.SensorTypePowerSupply, unit: types.SensorUnitType_Watts, m: 4, base: 256, jitter: 16, poweredOnly: true},
		{num: 0x60, name: "PS1 Status", stype: types.SensorTypePowerSupply, discrete: true, state: 1 << 0},      // presence detected
		{num: 0x61, name: "PS2 Status", stype: types.SensorTypePowerSupply, discrete: true, state: 1<<0 | 1<<1}, // presence + failure
		{num: 0x70, name: "Chassis Intru", stype: types.SensorTypePhysicalSecurity, discrete: true, state: 0},
	}
	now := time.Now()
	st.addSEL(now.Add(-72*time.Hour), types.SensorTypePowerUnit, 0xCA, false, types.EventReadingTypeSensorSpecific, types.EventData{EventData1: 0x00})
	st.addSEL(now.Add(-26*time.Hour), types.SensorTypeTemperature, 0x01, false, types.EventReadingTypeThreshold, types.EventData{EventData1: 0x57, EventData2: 91, EventData3: 90})
	st.addSEL(now.Add(-26*time.Hour+3*time.Minute), types.SensorTypeTemperature, 0x01, true, types.EventReadingTypeThreshold, types.EventData{EventData1: 0x57, EventData2: 88, EventData3: 90})
	st.addSEL(now.Add(-5*time.Hour), types.SensorTypePowerSupply, 0x61, false, types.EventReadingTypeSensorSpecific, types.EventData{EventData1: 0x01})
	st.addSEL(now.Add(-2*time.Hour), types.SensorTypeMemory, 0xD0, false, types.EventReadingTypeSensorSpecific, types.EventData{EventData1: 0x00})
	st.addSEL(now.Add(-35*time.Minute), types.SensorTypeFan, 0x44, false, types.EventReadingTypeThreshold, types.EventData{EventData1: 0x50, EventData2: 6, EventData3: 7})
	return st
}

func (st *state) addSEL(ts time.Time, stype types.SensorType, num uint8, deassert bool, ert types.EventReadingType, data types.EventData) {
	id := st.nextSEL
	st.nextSEL++
	st.sel = append(st.sel, selRecord{id: id, sel: &types.SEL{
		RecordID:   id,
		RecordType: types.SELRecordType(0x02),
		Standard: &types.SELStandard{
			Timestamp:        ts,
			GeneratorID:      0x0020,
			EvMRev:           0x04,
			SensorType:       stype,
			SensorNumber:     types.SensorNumber(num),
			EventDir:         types.EventDir(deassert),
			EventReadingType: ert,
			EventData:        data,
		},
	}})
}

func (st *state) powered() bool {
	on, _ := st.hal.Chassis().PowerState(context.Background())
	return on
}

// value returns the current real reading of a threshold sensor.
func (st *state) value(s *simSensor) float64 {
	if s.poweredOnly && !st.powered() {
		if s.stype == types.SensorTypeTemperature {
			return s.base * 0.55
		}
		return 0
	}
	if s.jitter == 0 {
		return s.base
	}
	return s.base + (rand.Float64()*2-1)*s.jitter
}

func (s *simSensor) factor() float64 { return float64(s.m) * math.Pow10(int(s.rexp)) }

func (s *simSensor) raw(v float64) uint8 {
	r := math.Round(v / s.factor())
	return uint8(math.Max(0, math.Min(255, r)))
}

func (st *state) seedStorage(ctx context.Context) error {
	store := st.hal.Storage()
	fru, err := types.PackFRU(types.FRUPackConfig{
		Chassis: &types.FRUPackChassis{Type: 0x17, PartNumber: "CSE-819UTS-R1K02P-T", Serial: fmt.Sprintf("C8190SIM%04d", st.index)},
		Board: &types.FRUPackBoard{MfgDate: time.Date(2023, 4, 12, 0, 0, 0, 0, time.UTC), Mfg: "Supermicro",
			Product: "X12DPU-6", Serial: fmt.Sprintf("WM23AS%06d", 100000+st.index), PartNumber: "X12DPU-6"},
		Product: &types.FRUPackProduct{Manufacturer: "Supermicro", Name: "SYS-120U-TNR", PartModel: "SYS-120U-TNR",
			Version: "0123456789", Serial: fmt.Sprintf("S5040SIM%04d", st.index)},
	})
	if err != nil {
		return err
	}
	if err := store.FRU().Write(ctx, 0, fru); err != nil {
		return err
	}
	sdr := store.SDR()
	if err := sdr.Write(ctx, 1, types.PackMCLocator(types.MCLocatorPackOpts{RecordID: 1})); err != nil {
		return err
	}
	for i, s := range st.sensors {
		id := uint16(i + 2)
		if err := sdr.Write(ctx, id, st.packSDR(s, id)); err != nil {
			return err
		}
	}
	return nil
}

func (st *state) packSDR(s *simSensor, id uint16) []byte {
	name := []byte(s.name)
	full := &types.SDRFull{
		GeneratorID:    0x0020,
		SensorNumber:   types.SensorNumber(s.num),
		SensorEntityID: types.EntityID(0x07),
		SensorInitialization: types.SensorInitialization{
			InitScanning: true, InitEvents: true, EventGenerationEnabled: true, SensorScanningEnabled: true,
		},
		SensorCapabilities: types.SensorCapabilities{AutoRearm: true},
		SensorType:         s.stype,
		IDStringTypeLength: types.TypeLength(0xC0 | byte(len(name))),
		IDStringBytes:      name,
	}
	if s.discrete {
		full.SensorEventReadingType = types.EventReadingTypeSensorSpecific
		full.SensorUnit = types.SensorUnit{AnalogDataFormat: types.SensorAnalogUnitFormat_NotAnalog}
	} else {
		full.SensorEventReadingType = types.EventReadingTypeThreshold
		full.SensorUnit = types.SensorUnit{AnalogDataFormat: types.SensorAnalogUnitFormat_Unsigned, BaseUnit: s.unit}
		full.SensorCapabilities.ThresholdAccess = types.SensorThresholdAccess_Readable
		full.ReadingFactors = types.ReadingFactors{M: s.m, R_Exp: s.rexp}
		full.SensorMaxReadingRaw = 0xff
	}
	return full.Pack(id)
}

func (st *state) sensorByNum(n uint8) *simSensor {
	for _, s := range st.sensors {
		if s.num == n {
			return s
		}
	}
	return nil
}

func (st *state) registerHandlers(reg *handlers.Registry) {
	reg.RegisterFunc(types.CommandGetSensorReading, st.handleSensorReading)
	reg.RegisterFunc(types.CommandGetSensorThresholds, st.handleSensorThresholds)
	reg.RegisterFunc(types.CommandGetSELInfo, st.handleSELInfo)
	reg.RegisterFunc(types.CommandReserveSEL, st.handleReserveSEL)
	reg.RegisterFunc(types.CommandGetSELEntry, st.handleGetSELEntry)
	reg.RegisterFunc(types.CommandClearSEL, st.handleClearSEL)
}

func (st *state) handleSensorReading(_ context.Context, _ *handlers.HandlerContext, req []byte) ([]byte, types.CompletionCode, error) {
	if len(req) < 1 {
		return nil, types.CodeRequestDataLengthInvalid, nil
	}
	s := st.sensorByNum(req[0])
	if s == nil {
		return nil, types.CodeRequestedDataNotPresent, nil
	}
	const flags = 0xC0 // event messages and scanning enabled
	if s.discrete {
		return []byte{0, flags, byte(s.state), byte(s.state >> 8)}, types.CodeOK, nil
	}
	v := st.value(s)
	var status byte
	switch {
	case s.ucr != 0 && v >= s.ucr:
		status |= 1<<4 | 1<<3
	case s.unc != 0 && v >= s.unc:
		status |= 1 << 3
	}
	switch {
	case s.lcr != 0 && v <= s.lcr:
		status |= 1<<1 | 1<<0
	case s.lnc != 0 && v <= s.lnc:
		status |= 1 << 0
	}
	return []byte{s.raw(v), flags, status, 0x80}, types.CodeOK, nil
}

func (st *state) handleSensorThresholds(_ context.Context, _ *handlers.HandlerContext, req []byte) ([]byte, types.CompletionCode, error) {
	if len(req) < 1 {
		return nil, types.CodeRequestDataLengthInvalid, nil
	}
	s := st.sensorByNum(req[0])
	if s == nil || s.discrete {
		return nil, types.CodeRequestedDataNotPresent, nil
	}
	var mask byte
	if s.lnc != 0 {
		mask |= 1 << 0
	}
	if s.lcr != 0 {
		mask |= 1 << 1
	}
	if s.unc != 0 {
		mask |= 1 << 3
	}
	if s.ucr != 0 {
		mask |= 1 << 4
	}
	return []byte{mask, s.raw(s.lnc), s.raw(s.lcr), 0, s.raw(s.unc), s.raw(s.ucr), 0}, types.CodeOK, nil
}

func ts32(t time.Time) uint32 {
	if t.IsZero() {
		return 0xffffffff
	}
	return uint32(t.Unix())
}

func (st *state) handleSELInfo(_ context.Context, _ *handlers.HandlerContext, _ []byte) ([]byte, types.CompletionCode, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]byte, 14)
	out[0] = 0x51 // SEL version 1.5/2.0
	binary.LittleEndian.PutUint16(out[1:], uint16(len(st.sel)))
	binary.LittleEndian.PutUint16(out[3:], uint16((512-len(st.sel))*16))
	var last time.Time
	if n := len(st.sel); n > 0 {
		last = st.sel[n-1].sel.Standard.Timestamp
	}
	binary.LittleEndian.PutUint32(out[5:], ts32(last))
	binary.LittleEndian.PutUint32(out[9:], ts32(st.erased))
	out[13] = 0x02 // Reserve SEL supported
	return out, types.CodeOK, nil
}

func (st *state) handleReserveSEL(_ context.Context, _ *handlers.HandlerContext, _ []byte) ([]byte, types.CompletionCode, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.resv++
	if st.resv == 0 {
		st.resv = 1
	}
	out := make([]byte, 2)
	binary.LittleEndian.PutUint16(out, st.resv)
	return out, types.CodeOK, nil
}

func (st *state) handleGetSELEntry(_ context.Context, _ *handlers.HandlerContext, req []byte) ([]byte, types.CompletionCode, error) {
	if len(req) < 6 {
		return nil, types.CodeRequestDataLengthInvalid, nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.sel) == 0 {
		return nil, types.CodeRequestedDataNotPresent, nil
	}
	want := binary.LittleEndian.Uint16(req[2:])
	idx := -1
	switch want {
	case 0x0000:
		idx = 0
	case 0xffff:
		idx = len(st.sel) - 1
	default:
		for i, r := range st.sel {
			if r.id == want {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return nil, types.CodeRequestedDataNotPresent, nil
	}
	next := uint16(0xffff)
	if idx+1 < len(st.sel) {
		next = st.sel[idx+1].id
	}
	out := make([]byte, 2, 18)
	binary.LittleEndian.PutUint16(out, next)
	out = append(out, st.sel[idx].sel.Pack()...)
	return out, types.CodeOK, nil
}

func (st *state) handleClearSEL(_ context.Context, _ *handlers.HandlerContext, req []byte) ([]byte, types.CompletionCode, error) {
	if len(req) < 6 || req[2] != 'C' || req[3] != 'L' || req[4] != 'R' {
		return nil, types.CodeRequestDataFieldInvalid, nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if binary.LittleEndian.Uint16(req[0:]) != st.resv {
		return nil, types.CodeReservationCanceled, nil
	}
	if req[5] == 0xAA {
		st.clearLocked()
	}
	return []byte{0x01}, types.CodeOK, nil // erasure completed
}

func (st *state) clearLocked() {
	st.sel = nil
	st.erased = time.Now()
}

// snapshotSEL copies the SEL for the Redfish view.
func (st *state) snapshotSEL() []selRecord {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]selRecord(nil), st.sel...)
}

func (st *state) clearSEL() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.clearLocked()
}
