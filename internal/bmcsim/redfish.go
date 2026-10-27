package bmcsim

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bougou/go-ipmi/pkg/types"
)

type obj = map[string]any

func link(path string) obj { return obj{"@odata.id": path} }

func collection(path, name string, members ...string) obj {
	ms := make([]obj, 0, len(members))
	for _, m := range members {
		ms = append(ms, link(m))
	}
	return obj{"@odata.id": path, "Name": name, "Members": ms, "Members@odata.count": len(ms)}
}

func health(h string) obj { return obj{"State": "Enabled", "Health": h, "HealthRollup": h} }

func (s *Sim) routes() http.Handler {
	mux := http.NewServeMux()
	web := &webUI{sim: s, sessions: map[string]bool{}, tokens: map[string]time.Time{}}

	// Unauthenticated service root, like real BMCs.
	mux.HandleFunc("GET /redfish/v1/{$}", s.serviceRoot)
	mux.HandleFunc("GET /redfish/v1", s.serviceRoot)
	mux.Handle("/redfish/v1/", s.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.redfish(w, r, web)
	})))

	mux.HandleFunc("GET /{$}", web.loginPage)
	mux.HandleFunc("POST /cgi/login.cgi", web.login)
	mux.HandleFunc("GET /cgi/url_redirect.cgi", web.redirect)
	return mux
}

func (s *Sim) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(s.opt.User)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(s.opt.Password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Redfish"`)
			writeJSON(w, http.StatusUnauthorized, obj{"error": obj{"code": "Base.1.8.GeneralError", "message": "Unauthorized"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Sim) serviceRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, obj{
		"@odata.id":      "/redfish/v1",
		"@odata.type":    "#ServiceRoot.v1_11_0.ServiceRoot",
		"Id":             "ServiceRoot",
		"Name":           "Root Service",
		"RedfishVersion": "1.11.0",
		"Vendor":         "Supermicro",
		"Product":        "SYS-120U-TNR",
		"UUID":           fmt.Sprintf("00000000-0000-0000-0000-3cecef%06x", s.opt.Index),
		"Systems":        link("/redfish/v1/Systems"),
		"Chassis":        link("/redfish/v1/Chassis"),
		"Managers":       link("/redfish/v1/Managers"),
		"UpdateService":  link("/redfish/v1/UpdateService"),
		"SessionService": link("/redfish/v1/SessionService"),
		"Oem":            obj{"Supermicro": obj{}},
	})
}

func (s *Sim) redfish(w http.ResponseWriter, r *http.Request, web *webUI) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	st := s.state
	if r.Method == http.MethodPost {
		switch path {
		case "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset":
			var body struct{ ResetType string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if err := s.reset(r.Context(), body.ResetType); err != nil {
				writeJSON(w, http.StatusBadRequest, obj{"error": obj{"message": err.Error()}})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/redfish/v1/Managers/1/LogServices/Log1/Actions/LogService.ClearLog":
			st.clearSEL()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	v, ok := s.resource(path, web)
	if !ok {
		writeJSON(w, http.StatusNotFound, obj{"error": obj{"message": "Resource not found"}})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Sim) reset(ctx context.Context, resetType string) error {
	ch := s.hal.Chassis()
	switch resetType {
	case "On", "ForceOn":
		return ch.SetPower(ctx, true)
	case "ForceOff", "GracefulShutdown":
		return ch.SetPower(ctx, false)
	case "ForceRestart", "GracefulRestart":
		return ch.ColdReset(ctx)
	case "PowerCycle":
		return ch.PowerCycle(ctx)
	}
	return fmt.Errorf("unsupported ResetType %q", resetType)
}

func (s *Sim) resource(path string, web *webUI) (any, bool) {
	st := s.state
	idx := s.opt.Index
	powerState := "Off"
	if st.powered() {
		powerState = "On"
	}
	sysHealth := "Warning" // PS2 failure and FAN4 below threshold
	switch path {
	case "/redfish/v1/Systems":
		return collection(path, "Computer System Collection", "/redfish/v1/Systems/1"), true
	case "/redfish/v1/Systems/1":
		return obj{
			"@odata.id": path, "@odata.type": "#ComputerSystem.v1_16_0.ComputerSystem", "Id": "1", "Name": "System",
			"Manufacturer": "Supermicro", "Model": "SYS-120U-TNR", "SerialNumber": fmt.Sprintf("S5040SIM%04d", idx),
			"SKU": "Default string", "UUID": fmt.Sprintf("2c1b5a00-0000-11ec-8000-3cecef%06x", idx), "HostName": fmt.Sprintf("node%02d", idx+1),
			"BiosVersion": "1.4a", "PowerState": powerState, "Status": health(sysHealth),
			"ProcessorSummary": obj{"Count": 2, "CoreCount": 64, "LogicalProcessorCount": 128, "Model": "Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz", "Status": health("OK")},
			"MemorySummary":    obj{"TotalSystemMemoryGiB": 256, "Status": health("OK")},
			"Processors":       link(path + "/Processors"), "Memory": link(path + "/Memory"),
			"Storage": link(path + "/Storage"), "EthernetInterfaces": link(path + "/EthernetInterfaces"),
			"LogServices": link(path + "/LogServices"),
			"Actions": obj{"#ComputerSystem.Reset": obj{
				"target":                            path + "/Actions/ComputerSystem.Reset",
				"ResetType@Redfish.AllowableValues": []string{"On", "ForceOff", "GracefulShutdown", "GracefulRestart", "ForceRestart", "PowerCycle"},
			}},
		}, true
	case "/redfish/v1/Systems/1/Processors":
		return collection(path, "Processor Collection", path+"/1", path+"/2"), true
	case "/redfish/v1/Systems/1/Processors/1", "/redfish/v1/Systems/1/Processors/2":
		n := path[len(path)-1:]
		return obj{
			"@odata.id": path, "Id": n, "Name": "Processor", "Socket": "CPU" + n, "Manufacturer": "Intel(R) Corporation",
			"Model": "Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz", "TotalCores": 32, "TotalThreads": 64, "MaxSpeedMHz": 3200,
			"ProcessorType": "CPU", "Status": health("OK"),
		}, true
	case "/redfish/v1/Systems/1/Memory":
		var ms []string
		for i := 1; i <= 8; i++ {
			ms = append(ms, fmt.Sprintf("%s/%d", path, i))
		}
		return collection(path, "Memory Collection", ms...), true
	case "/redfish/v1/Systems/1/Storage":
		return collection(path, "Storage Collection", path+"/HA-RAID"), true
	case "/redfish/v1/Systems/1/Storage/HA-RAID":
		return obj{
			"@odata.id": path, "Id": "HA-RAID", "Name": "HA-RAID", "Status": health("OK"),
			"Drives": []obj{link("/redfish/v1/Chassis/1/Drives/Disk.Bay.0"), link("/redfish/v1/Chassis/1/Drives/Disk.Bay.1")},
		}, true
	case "/redfish/v1/Chassis/1/Drives/Disk.Bay.0", "/redfish/v1/Chassis/1/Drives/Disk.Bay.1":
		bay := path[len(path)-1:]
		return obj{
			"@odata.id": path, "Id": "Disk.Bay." + bay, "Name": "Disk.Bay." + bay, "Model": "SAMSUNG MZ7L3960HCJR-00A07",
			"SerialNumber": fmt.Sprintf("S664NE0T%03d%s", idx, bay), "CapacityBytes": 960197124096, "MediaType": "SSD",
			"Protocol": "SATA", "Revision": "JXTC104Q", "Status": health("OK"),
		}, true
	case "/redfish/v1/Systems/1/EthernetInterfaces":
		return collection(path, "Ethernet Interface Collection", path+"/1", path+"/2"), true
	case "/redfish/v1/Systems/1/EthernetInterfaces/1", "/redfish/v1/Systems/1/EthernetInterfaces/2":
		n := path[len(path)-1:]
		nic := obj{
			"@odata.id": path, "Id": n, "Name": "Ethernet Interface " + n, "MACAddress": fmt.Sprintf("3c:ec:ef:%02x:%02x:0%s", idx>>8&0xff, idx&0xff, n),
			"SpeedMbps": 10000, "LinkStatus": "LinkUp", "Status": health("OK"),
		}
		if n == "1" && st.powered() {
			nic["IPv4Addresses"] = []obj{{"Address": fmt.Sprintf("10.20.%d.%d", idx/250, idx%250+10), "AddressOrigin": "DHCP"}}
		} else {
			nic["LinkStatus"] = "LinkDown"
		}
		return nic, true
	case "/redfish/v1/Systems/1/LogServices":
		return collection(path, "Log Service Collection"), true
	case "/redfish/v1/Chassis":
		return collection(path, "Chassis Collection", "/redfish/v1/Chassis/1"), true
	case "/redfish/v1/Chassis/1":
		return obj{
			"@odata.id": path, "Id": "1", "Name": "Computer System Chassis", "ChassisType": "RackMount",
			"Manufacturer": "Supermicro", "Model": "CSE-819UTS-R1K02P-T", "SerialNumber": fmt.Sprintf("C8190SIM%04d", idx),
			"Status": health(sysHealth), "Thermal": link(path + "/Thermal"), "Power": link(path + "/Power"),
		}, true
	case "/redfish/v1/Chassis/1/Thermal":
		return s.thermal(path), true
	case "/redfish/v1/Chassis/1/Power":
		return s.power(path), true
	case "/redfish/v1/Managers":
		return collection(path, "Manager Collection", "/redfish/v1/Managers/1"), true
	case "/redfish/v1/Managers/1":
		return obj{
			"@odata.id": path, "Id": "1", "Name": "Manager", "ManagerType": "BMC", "Model": "ASPEED",
			"FirmwareVersion": "01.01.10", "Status": health("OK"), "PowerState": "On",
			"LogServices": link(path + "/LogServices"), "EthernetInterfaces": link(path + "/EthernetInterfaces"),
		}, true
	case "/redfish/v1/Managers/1/EthernetInterfaces":
		return collection(path, "Ethernet Network Interface Collection", path+"/1"), true
	case "/redfish/v1/Managers/1/EthernetInterfaces/1":
		return obj{
			"@odata.id": path, "Id": "1", "Name": "Manager Ethernet Interface", "MACAddress": fmt.Sprintf("3c:ec:ef:%02x:%02x:ff", idx>>8&0xff, idx&0xff),
			"IPv4Addresses": []obj{{"Address": "127.0.0.1", "AddressOrigin": "Static"}}, "Status": health("OK"),
		}, true
	case "/redfish/v1/Managers/1/LogServices":
		return collection(path, "Log Service Collection", path+"/Log1"), true
	case "/redfish/v1/Managers/1/LogServices/Log1":
		return obj{
			"@odata.id": path, "Id": "Log1", "Name": "Log Service", "Entries": link(path + "/Entries"),
			"Actions": obj{"#LogService.ClearLog": obj{"target": path + "/Actions/LogService.ClearLog"}},
		}, true
	case "/redfish/v1/Managers/1/LogServices/Log1/Entries":
		return s.logEntries(path), true
	case "/redfish/v1/Managers/1/Oem/Supermicro/IKVM":
		return obj{
			"@odata.id": path, "@odata.type": "#IKVM.v1_0_2.IKVM", "Id": "IKVM", "Name": "IKVM",
			"Current interface": "HTML 5", "Mode": "Relative",
			"URI": "/cgi/url_redirect.cgi?url_name=man_ikvm_html5_auto&token=" + web.newToken(),
		}, true
	case "/redfish/v1/UpdateService":
		return obj{"@odata.id": path, "Id": "UpdateService", "Name": "Update Service", "FirmwareInventory": link(path + "/FirmwareInventory")}, true
	case "/redfish/v1/UpdateService/FirmwareInventory":
		return collection(path, "Firmware Inventory Collection", path+"/BMC", path+"/BIOS"), true
	case "/redfish/v1/UpdateService/FirmwareInventory/BMC":
		return obj{"@odata.id": path, "Id": "BMC", "Name": "BMC Firmware", "Version": "01.01.10", "Updateable": true, "Status": health("OK")}, true
	case "/redfish/v1/UpdateService/FirmwareInventory/BIOS":
		return obj{"@odata.id": path, "Id": "BIOS", "Name": "BIOS Firmware", "Version": "1.4a", "Updateable": true, "Status": health("OK")}, true
	}
	if strings.HasPrefix(path, "/redfish/v1/Systems/1/Memory/") {
		n := strings.TrimPrefix(path, "/redfish/v1/Systems/1/Memory/")
		i, err := strconv.Atoi(n)
		if err != nil || i < 1 || i > 8 {
			return nil, false
		}
		return obj{
			"@odata.id": path, "Id": n, "Name": "Memory " + n, "DeviceLocator": fmt.Sprintf("P%d-DIMM%c1", (i-1)/4+1, 'A'+(i-1)%4),
			"CapacityMiB": 32768, "MemoryDeviceType": "DDR4", "OperatingSpeedMhz": 3200, "Manufacturer": "Samsung",
			"PartNumber": "M393A4K40EB3-CWE", "SerialNumber": fmt.Sprintf("%08X", 0x40000000+idx*16+i), "Status": health("OK"),
		}, true
	}
	if strings.HasPrefix(path, "/redfish/v1/Managers/1/LogServices/Log1/Entries/") {
		id := strings.TrimPrefix(path, "/redfish/v1/Managers/1/LogServices/Log1/Entries/")
		for _, e := range s.logEntryList("/redfish/v1/Managers/1/LogServices/Log1/Entries") {
			if e["Id"] == id {
				return e, true
			}
		}
	}
	return nil, false
}

func (s *Sim) sensorObj(sn *simSensor) (reading float64, status string) {
	v := s.state.value(sn)
	reading = math.Round(v*100) / 100
	status = "OK"
	switch {
	case sn.ucr != 0 && v >= sn.ucr, sn.lcr != 0 && v <= sn.lcr:
		status = "Critical"
	case sn.unc != 0 && v >= sn.unc, sn.lnc != 0 && v <= sn.lnc:
		status = "Warning"
	}
	return
}

func thr(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func (s *Sim) thermal(path string) obj {
	var temps, fans []obj
	for i, sn := range s.state.sensors {
		if sn.discrete {
			continue
		}
		reading, status := s.sensorObj(sn)
		member := fmt.Sprintf("%d", i)
		switch sn.stype {
		case types.SensorTypeTemperature:
			temps = append(temps, obj{
				"@odata.id": fmt.Sprintf("%s#/Temperatures/%d", path, len(temps)), "MemberId": member, "Name": sn.name,
				"ReadingCelsius": reading, "Status": health(status), "PhysicalContext": "SystemBoard",
				"UpperThresholdNonCritical": thr(sn.unc), "UpperThresholdCritical": thr(sn.ucr),
			})
		case types.SensorTypeFan:
			fans = append(fans, obj{
				"@odata.id": fmt.Sprintf("%s#/Fans/%d", path, len(fans)), "MemberId": member, "Name": sn.name,
				"Reading": int(reading), "ReadingUnits": "RPM", "Status": health(status),
				"LowerThresholdNonCritical": thr(sn.lnc), "LowerThresholdCritical": thr(sn.lcr),
			})
		}
	}
	return obj{"@odata.id": path, "Id": "Thermal", "Name": "Thermal", "Temperatures": temps, "Fans": fans}
}

func (s *Sim) power(path string) obj {
	var volts []obj
	var consumed float64
	for i, sn := range s.state.sensors {
		if sn.discrete {
			continue
		}
		reading, status := s.sensorObj(sn)
		switch sn.stype {
		case types.SensorTypeVoltage:
			volts = append(volts, obj{
				"@odata.id": fmt.Sprintf("%s#/Voltages/%d", path, len(volts)), "MemberId": fmt.Sprint(i), "Name": sn.name,
				"ReadingVolts": reading, "Status": health(status),
				"LowerThresholdCritical": thr(sn.lcr), "LowerThresholdNonCritical": thr(sn.lnc),
				"UpperThresholdNonCritical": thr(sn.unc), "UpperThresholdCritical": thr(sn.ucr),
			})
		case types.SensorTypePowerSupply:
			consumed = reading
		}
	}
	return obj{
		"@odata.id": path, "Id": "Power", "Name": "Power", "Voltages": volts,
		"PowerControl": []obj{{"@odata.id": path + "#/PowerControl/0", "MemberId": "0", "Name": "System Power Control",
			"PowerConsumedWatts": consumed, "PowerCapacityWatts": 2000, "Status": health("OK")}},
		"PowerSupplies": []obj{
			{"@odata.id": path + "#/PowerSupplies/0", "MemberId": "0", "Name": "PS1", "Model": "PWS-1K02A-1R", "SerialNumber": fmt.Sprintf("P1K02AL%05d", s.opt.Index*2),
				"PowerCapacityWatts": 1000, "PowerInputWatts": consumed, "FirmwareVersion": "1.2", "Status": health("OK")},
			{"@odata.id": path + "#/PowerSupplies/1", "MemberId": "1", "Name": "PS2", "Model": "PWS-1K02A-1R", "SerialNumber": fmt.Sprintf("P1K02AL%05d", s.opt.Index*2+1),
				"PowerCapacityWatts": 1000, "FirmwareVersion": "1.2", "Status": health("Critical")},
		},
	}
}

func (s *Sim) logEntryList(path string) []obj {
	var out []obj
	for _, r := range s.state.snapshotSEL() {
		std := r.sel.Standard
		sev := "OK"
		switch strings.ToLower(string(std.EventSeverity())) {
		case "warning", "degraded", "non-fatal":
			sev = "Warning"
		case "critical":
			sev = "Critical"
		}
		dir := "Assertion"
		if std.EventDir {
			dir = "Deassertion"
		}
		name := ""
		if sn := s.state.sensorByNum(uint8(std.SensorNumber)); sn != nil {
			name = sn.name
		}
		id := fmt.Sprint(r.id)
		out = append(out, obj{
			"@odata.id": path + "/" + id, "Id": id, "Name": name, "EntryType": "SEL",
			"Created": std.Timestamp.Format(time.RFC3339), "Severity": sev,
			"SensorType": std.SensorType.String(), "SensorNumber": int(std.SensorNumber),
			"Message": fmt.Sprintf("%s - %s", std.EventString(), dir),
		})
	}
	return out
}

func (s *Sim) logEntries(path string) obj {
	entries := s.logEntryList(path)
	return obj{"@odata.id": path, "Name": "Log Entry Collection", "Members": entries, "Members@odata.count": len(entries)}
}

// webUI imitates the classic Supermicro web login and iKVM pages so the
// console window and its auto-login can be exercised.
type webUI struct {
	sim      *Sim
	mu       sync.Mutex
	sessions map[string]bool
	tokens   map[string]time.Time
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (u *webUI) newToken() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	t := randHex(12)
	u.tokens[t] = time.Now().Add(2 * time.Minute)
	return t
}

const pageStyle = `<style>body{font-family:Segoe UI,Arial,sans-serif;background:#0b2a4a;color:#fff;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;color:#222;padding:32px 40px;border-radius:8px;min-width:320px;box-shadow:0 8px 30px rgba(0,0,0,.3)}
input{display:block;width:100%;margin:6px 0 14px;padding:8px;box-sizing:border-box}button{padding:8px 24px;background:#1565c0;color:#fff;border:0;border-radius:4px}
.kvm{width:80vw;height:70vh;background:#000;color:#0f0;font-family:Consolas,monospace;padding:16px;box-sizing:border-box;border:2px solid #444}</style>`

func (u *webUI) loginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	msg := ""
	if r.URL.Query().Get("err") != "" {
		msg = `<p style="color:#c62828">Invalid username or password</p>`
	}
	fmt.Fprintf(w, `<!doctype html><html><head><title>Supermicro BMC Login (simulated)</title>%s</head><body>
<form class="box" name="form1" action="/cgi/login.cgi" method="post"><h2>BMC 模拟器 #%d</h2>%s
<label>Username<input type="text" name="name" autocomplete="username"></label>
<label>Password<input type="password" name="pwd" autocomplete="current-password"></label>
<button type="submit" id="login_word">Login</button></form></body></html>`, pageStyle, u.sim.opt.Index, msg)
}

func (u *webUI) login(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("name") != u.sim.opt.User || r.FormValue("pwd") != u.sim.opt.Password {
		http.Redirect(w, r, "/?err=1", http.StatusFound)
		return
	}
	sid := randHex(16)
	u.mu.Lock()
	u.sessions[sid] = true
	u.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "SID", Value: sid, Path: "/", Secure: true, HttpOnly: true})
	http.Redirect(w, r, "/cgi/url_redirect.cgi?url_name=mainmenu", http.StatusFound)
}

func (u *webUI) redirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	authed := false
	if c, err := r.Cookie("SID"); err == nil {
		u.mu.Lock()
		authed = u.sessions[c.Value]
		u.mu.Unlock()
	}
	if t := q.Get("token"); t != "" {
		u.mu.Lock()
		if exp, ok := u.tokens[t]; ok && time.Now().Before(exp) {
			authed = true
			delete(u.tokens, t)
		}
		u.mu.Unlock()
	}
	if !authed {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	power := "OFF"
	if u.sim.PowerOn() {
		power = "ON"
	}
	switch q.Get("url_name") {
	case "man_ikvm_html5_bootstrap", "man_ikvm_html5_auto":
		fmt.Fprintf(w, `<!doctype html><html><head><title>iKVM (simulated)</title>%s</head><body><div class="kvm">
Supermicro HTML5 iKVM (simulated) — BMC #%d<br><br>Host power: %s<br><br>localhost login: _</div></body></html>`, pageStyle, u.sim.opt.Index, power)
	default:
		fmt.Fprintf(w, `<!doctype html><html><head><title>BMC Main Menu (simulated)</title>%s</head><body><div class="box">
<h2>已登录 BMC 模拟器 #%d</h2><p>Host power: %s</p><p><a href="/cgi/url_redirect.cgi?url_name=man_ikvm_html5_bootstrap">Launch iKVM (HTML5)</a></p></div></body></html>`,
			pageStyle, u.sim.opt.Index, html.EscapeString(power))
	}
}
