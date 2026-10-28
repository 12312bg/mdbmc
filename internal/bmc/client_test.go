package bmc_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/bmcsim"
)

func startSim(t *testing.T, opt bmcsim.Options) *bmcsim.Sim {
	t.Helper()
	sim, err := bmcsim.Start(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sim.Close)
	return sim
}

func target(sim *bmcsim.Sim, proto bmc.Protocol) bmc.Target {
	return bmc.Target{
		Host: "127.0.0.1", IPMIPort: sim.IPMIPort, HTTPSPort: sim.HTTPSPort,
		Username: "ADMIN", Password: "ADMIN", Protocol: proto,
		IPMITimeout: 2 * time.Second, RedfishTimeout: 10 * time.Second,
	}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestIdentifyBothProtocols(t *testing.T) {
	sim := startSim(t, bmcsim.Options{Index: 3})
	c := bmc.New(target(sim, bmc.ProtocolAuto))
	defer c.Close()
	id, err := c.Identify(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	if !id.HasIPMI || !id.HasRedfish {
		t.Fatalf("protocols: ipmi=%v redfish=%v warnings=%v", id.HasIPMI, id.HasRedfish, id.Warnings)
	}
	if id.Vendor != bmc.VendorSupermicro || id.ManufacturerID != 10876 {
		t.Fatalf("vendor = %q (iana %d)", id.Vendor, id.ManufacturerID)
	}
	if id.Model != "SYS-120U-TNR" || !strings.HasSuffix(id.Serial, "0003") || id.BMCFirmware != "01.01.10" {
		t.Fatalf("identity = %+v", id)
	}
	if id.TLSFingerprint != sim.Fingerprint {
		t.Fatalf("fingerprint %s, want %s", id.TLSFingerprint, sim.Fingerprint)
	}
	if id.PowerState != bmc.PowerOn {
		t.Fatalf("power = %s", id.PowerState)
	}
}

func TestPowerAndStatusPerProtocol(t *testing.T) {
	for _, proto := range []bmc.Protocol{bmc.ProtocolIPMI, bmc.ProtocolRedfish} {
		t.Run(string(proto), func(t *testing.T) {
			sim := startSim(t, bmcsim.Options{})
			tg := target(sim, proto)
			c := bmc.New(tg)
			via, err := c.Power(ctx(t), bmc.ActionOff)
			c.Close()
			if err != nil || via != proto {
				t.Fatalf("power off via %s: %v", via, err)
			}
			if sim.PowerOn() {
				t.Fatal("simulator still powered on")
			}
			c = bmc.New(tg)
			st := c.Status(ctx(t))
			c.Close()
			if !st.Reachable || st.Power != bmc.PowerOff || st.Via != proto {
				t.Fatalf("status = %+v", st)
			}
			c = bmc.New(tg)
			if _, err := c.Power(ctx(t), bmc.ActionOn); err != nil {
				t.Fatal(err)
			}
			c.Close()
			if !sim.PowerOn() {
				t.Fatal("simulator not powered on")
			}
		})
	}
}

func TestSensorsAndSEL(t *testing.T) {
	for _, proto := range []bmc.Protocol{bmc.ProtocolIPMI, bmc.ProtocolRedfish} {
		t.Run(string(proto), func(t *testing.T) {
			sim := startSim(t, bmcsim.Options{})
			c := bmc.New(target(sim, proto))
			defer c.Close()

			sensors, via, err := c.Sensors(ctx(t))
			if err != nil || via != proto {
				t.Fatalf("sensors via %s: %v", via, err)
			}
			byName := map[string]bmc.Sensor{}
			for _, s := range sensors {
				byName[s.Name] = s
			}
			cpu, ok := byName["CPU1 Temp"]
			if !ok || cpu.Reading == nil || *cpu.Reading < 40 || *cpu.Reading > 52 || cpu.Category != "temperature" || cpu.Status != "ok" {
				t.Fatalf("CPU1 Temp = %+v", cpu)
			}
			if cpu.UpperCritical == nil || *cpu.UpperCritical != 95 {
				t.Fatalf("CPU1 Temp UCR = %v", cpu.UpperCritical)
			}
			if fan := byName["FAN4"]; fan.Status != "warning" {
				t.Fatalf("FAN4 should be warning, got %+v", fan)
			}
			if v := byName["12V"]; v.Reading == nil || *v.Reading < 11.9 || *v.Reading > 12.2 || v.Unit != "V" {
				t.Fatalf("12V = %+v", v)
			}

			sel, _, err := c.SEL(ctx(t))
			if err != nil {
				t.Fatal(err)
			}
			if len(sel) != 6 {
				t.Fatalf("sel entries = %d", len(sel))
			}
			if sel[0].Time < sel[len(sel)-1].Time {
				t.Fatal("SEL should be newest first")
			}
			var sawPSU bool
			for _, e := range sel {
				if strings.Contains(e.Sensor, "PS2") && e.Severity == "critical" {
					sawPSU = true
				}
			}
			if !sawPSU {
				t.Fatalf("missing critical PS2 event: %+v", sel)
			}

			if _, err := c.ClearSEL(ctx(t)); err != nil {
				t.Fatal(err)
			}
			sel, _, err = c.SEL(ctx(t))
			if err != nil || len(sel) != 0 {
				t.Fatalf("after clear: %d entries, err %v", len(sel), err)
			}
		})
	}
}

func TestInventory(t *testing.T) {
	sim := startSim(t, bmcsim.Options{Index: 1})
	c := bmc.New(target(sim, bmc.ProtocolAuto))
	defer c.Close()
	inv, err := c.Inventory(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Source != bmc.ProtocolRedfish || len(inv.Processors) != 2 || len(inv.Memory) != 8 ||
		len(inv.Drives) != 2 || len(inv.PowerSupplies) != 2 || len(inv.Firmware) != 2 || inv.System.MemoryGiB != 256 {
		t.Fatalf("inventory = %+v", inv)
	}
	if inv.Memory[0].Slot != "P1-DIMMA1" || inv.Memory[7].Slot != "P2-DIMMD1" {
		t.Fatalf("dimm slots = %s .. %s", inv.Memory[0].Slot, inv.Memory[7].Slot)
	}
	if len(inv.FRU) == 0 || inv.FRU[0].BoardProduct != "X12DPU-6" {
		t.Fatalf("fru = %+v", inv.FRU)
	}
}

func TestAuthFailureAndPinning(t *testing.T) {
	sim := startSim(t, bmcsim.Options{})
	tg := target(sim, bmc.ProtocolAuto)
	tg.Password = "wrong"
	c := bmc.New(tg)
	_, err := c.Identify(ctx(t))
	c.Close()
	if err == nil || !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("want auth failure, got %v", err)
	}

	tg = target(sim, bmc.ProtocolRedfish)
	tg.TLSFingerprint = strings.Repeat("ab", 32)
	c = bmc.New(tg)
	_, _, err = c.Sensors(ctx(t))
	c.Close()
	var mm *bmc.FingerprintMismatchError
	if !errors.As(err, &mm) || mm.Observed != sim.Fingerprint {
		t.Fatalf("want fingerprint mismatch, got %v", err)
	}

	tg.TLSFingerprint = sim.Fingerprint
	c = bmc.New(tg)
	if st := c.Status(ctx(t)); !st.Reachable {
		t.Fatalf("pinned status failed: %+v", st)
	}
	c.Close()
}

func TestKVMLaunchSupermicro(t *testing.T) {
	sim := startSim(t, bmcsim.Options{})
	tg := target(sim, bmc.ProtocolAuto)
	tg.Vendor = bmc.VendorSupermicro
	c := bmc.New(tg)
	defer c.Close()
	spec, err := c.KVMLaunch(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	if spec.AutoLogin || !strings.Contains(spec.URL, "man_ikvm_html5_auto&token=") {
		t.Fatalf("spec = %+v", spec)
	}
}

func TestScanFindsSimulator(t *testing.T) {
	sim := startSim(t, bmcsim.Options{})
	var found []bmc.ScanResult
	bmc.Scan(ctx(t), []string{"127.0.0.1"}, bmc.ScanOptions{IPMIPort: sim.IPMIPort, HTTPSPort: sim.HTTPSPort, Timeout: time.Second},
		func(r bmc.ScanResult) { found = append(found, r) }, nil)
	if len(found) != 1 || !found[0].IPMI || found[0].IPMIVersion != "2.0" || !found[0].Redfish || found[0].Vendor != bmc.VendorSupermicro {
		t.Fatalf("found = %+v", found)
	}
	if found[0].TLSFingerprint != sim.Fingerprint {
		t.Fatal("scan should record the certificate fingerprint")
	}
}

func TestProbeCertificate(t *testing.T) {
	sim := startSim(t, bmcsim.Options{})
	cert, err := bmc.ProbeCertificate(ctx(t), "127.0.0.1", sim.HTTPSPort, "")
	if err != nil || cert.Fingerprint != sim.Fingerprint || cert.SPKIHash == "" {
		t.Fatalf("probe = %+v, %v", cert, err)
	}
	if _, err := bmc.ProbeCertificate(ctx(t), "127.0.0.1", sim.HTTPSPort, strings.Repeat("00", 32)); err == nil {
		t.Fatal("a different pinned fingerprint must be rejected")
	}
}
