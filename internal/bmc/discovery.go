package bmc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MaxScanHosts caps a single scan so a typo cannot sweep a /8.
const MaxScanHosts = 4096

// ExpandTargets parses a comma/newline separated list of addresses, CIDR
// blocks (192.168.1.0/24) and ranges (10.0.0.10-10.0.0.20 or 10.0.0.10-20).
func ExpandTargets(spec string, max int) ([]string, error) {
	if max <= 0 {
		max = MaxScanHosts
	}
	seen := map[string]bool{}
	var out []string
	add := func(a netip.Addr) error {
		s := a.String()
		if seen[s] {
			return nil
		}
		if len(out) >= max {
			return fmt.Errorf("扫描范围超过 %d 个地址，请缩小范围", max)
		}
		seen[s] = true
		out = append(out, s)
		return nil
	}
	fields := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '、' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	for _, f := range fields {
		switch {
		case strings.Contains(f, "/"):
			pfx, err := netip.ParsePrefix(f)
			if err != nil || !pfx.Addr().Is4() {
				return nil, fmt.Errorf("无效的网段 %q（仅支持 IPv4 CIDR）", f)
			}
			pfx = pfx.Masked()
			if pfx.Bits() < 32-12 { // more than 4096 addresses
				return nil, fmt.Errorf("网段 %s 过大，最大支持 /20", f)
			}
			first := pfx.Addr()
			size := 1 << (32 - pfx.Bits())
			a := first
			for i := 0; i < size; i++ {
				// Skip network and broadcast addresses for /30 and larger.
				if pfx.Bits() <= 30 && (i == 0 || i == size-1) {
					a = a.Next()
					continue
				}
				if err := add(a); err != nil {
					return nil, err
				}
				a = a.Next()
			}
		case isRange(f):
			lo, hi, _ := strings.Cut(f, "-")
			start, _ := netip.ParseAddr(strings.TrimSpace(lo))
			hi = strings.TrimSpace(hi)
			var end netip.Addr
			if n, err := strconv.Atoi(hi); err == nil && n >= 0 && n <= 255 {
				b := start.As4()
				b[3] = byte(n)
				end = netip.AddrFrom4(b)
			} else if end, err = netip.ParseAddr(hi); err != nil || !end.Is4() {
				return nil, fmt.Errorf("无效的地址范围 %q", f)
			}
			if end.Less(start) {
				return nil, fmt.Errorf("地址范围 %q 的结束地址小于起始地址", f)
			}
			for a := start; ; a = a.Next() {
				if err := add(a); err != nil {
					return nil, err
				}
				if a == end {
					break
				}
			}
		default:
			if a, err := netip.ParseAddr(f); err == nil {
				if err := add(a); err != nil {
					return nil, err
				}
				continue
			}
			// A hostname is passed through as-is.
			if !seen[f] {
				if len(out) >= max {
					return nil, fmt.Errorf("扫描范围超过 %d 个地址，请缩小范围", max)
				}
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("请输入要扫描的地址、网段或范围")
	}
	return out, nil
}

// isRange reports whether f looks like "a.b.c.d-x"; anything else with a
// hyphen is a host name such as bmc-rack1-01.example.com.
func isRange(f string) bool {
	lo, _, ok := strings.Cut(f, "-")
	if !ok {
		return false
	}
	a, err := netip.ParseAddr(strings.TrimSpace(lo))
	return err == nil && a.Is4()
}

// ScanResult describes a BMC found by discovery.
type ScanResult struct {
	Host           string `json:"host"`
	IPMI           bool   `json:"ipmi"`
	IPMIVersion    string `json:"ipmiVersion"`
	Redfish        bool   `json:"redfish"`
	RedfishVersion string `json:"redfishVersion"`
	Vendor         string `json:"vendor"`
	VendorName     string `json:"vendorName"`
	Product        string `json:"product"`
	UUID           string `json:"uuid"`
	TLSFingerprint string `json:"tlsFingerprint"`
}

// ScanOptions tunes a scan.
type ScanOptions struct {
	IPMIPort    int
	HTTPSPort   int
	Timeout     time.Duration
	Concurrency int
}

// Scan probes hosts concurrently and reports BMCs through onFound.
// onProgress receives the number of probed hosts.
func Scan(ctx context.Context, hosts []string, opt ScanOptions, onFound func(ScanResult), onProgress func(done, total int)) {
	if opt.IPMIPort == 0 {
		opt.IPMIPort = 623
	}
	if opt.HTTPSPort == 0 {
		opt.HTTPSPort = 443
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 1500 * time.Millisecond
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = 64
	}
	jobs := make(chan string)
	var done atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < opt.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range jobs {
				if r, ok := ProbeHost(ctx, h, opt); ok {
					onFound(r)
				}
				n := done.Add(1)
				if onProgress != nil {
					onProgress(int(n), len(hosts))
				}
			}
		}()
	}
feed:
	for _, h := range hosts {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- h:
		}
	}
	close(jobs)
	wg.Wait()
}

// ProbeHost checks one host for IPMI (UDP 623) and Redfish (HTTPS).
func ProbeHost(ctx context.Context, host string, opt ScanOptions) (ScanResult, bool) {
	r := ScanResult{Host: host}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if v, ok := probeIPMI(ctx, host, opt.IPMIPort, opt.Timeout); ok {
			r.IPMI, r.IPMIVersion = true, v
		}
	}()
	go func() {
		defer wg.Done()
		// TLS handshakes on BMCs are slow; allow a little more than for UDP.
		info, fp, err := probeServiceRoot(ctx, host, opt.HTTPSPort, opt.Timeout*3)
		if err == nil {
			r.Redfish = true
			r.RedfishVersion = info.RedfishVersion
			r.Product = info.Product
			r.UUID = info.UUID
			r.TLSFingerprint = fp
			r.Vendor = DetectVendor(0, append([]string{info.Vendor, info.Product}, info.OemKeys...)...)
		}
	}()
	wg.Wait()
	if !r.IPMI && !r.Redfish {
		return r, false
	}
	if r.Vendor == "" {
		r.Vendor = VendorGeneric
	}
	r.VendorName = VendorName(r.Vendor)
	return r, true
}

// getChannelAuthCapsRequest is an IPMI v1.5 session-less "Get Channel
// Authentication Capabilities" request (channel = current, privilege =
// administrator, IPMI v2.0 extended data requested) wrapped in RMCP.
var getChannelAuthCapsRequest = []byte{
	0x06, 0x00, 0xff, 0x07, // RMCP: version, reserved, seq (no ack), class IPMI
	0x00,                   // auth type none
	0x00, 0x00, 0x00, 0x00, // session sequence
	0x00, 0x00, 0x00, 0x00, // session id
	0x09,             // message length
	0x20, 0x18, 0xc8, // rsAddr BMC, netFn App / LUN 0, checksum
	0x81, 0x00, 0x38, // rqAddr, rqSeq, cmd Get Channel Auth Capabilities
	0x8e, 0x04, // channel E + v2.0 bit, privilege admin
	0xb5, // checksum
}

func probeIPMI(ctx context.Context, host string, port int, timeout time.Duration) (string, bool) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return "", false
	}
	defer conn.Close()
	buf := make([]byte, 512)
	for attempt := 0; attempt < 2; attempt++ {
		if ctx.Err() != nil {
			return "", false
		}
		if _, err := conn.Write(getChannelAuthCapsRequest); err != nil {
			return "", false
		}
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if err != nil {
			continue
		}
		if v, ok := parseChannelAuthCaps(buf[:n]); ok {
			return v, true
		}
	}
	return "", false
}

// parseChannelAuthCaps validates a Get Channel Authentication Capabilities
// response and returns the supported IPMI version.
func parseChannelAuthCaps(b []byte) (string, bool) {
	// RMCP(4) + auth type(1) + seq(4) + session id(4) [+ auth code(16)] + len(1)
	if len(b) < 14 || b[0] != 0x06 || b[3]&0x1f != 0x07 {
		return "", false
	}
	off := 4
	authType := b[off]
	off += 1 + 4 + 4
	if authType != 0 {
		off += 16
	}
	if len(b) < off+1 {
		return "", false
	}
	msgLen := int(b[off])
	off++
	msg := b[off:]
	if len(msg) < msgLen || msgLen < 7+8 {
		return "", false
	}
	// rqAddr, netFn/LUN, chk, rsAddr, rqSeq, cmd, completion code, data...
	if msg[1]>>2 != 0x07 || msg[5] != 0x38 || msg[6] != 0x00 {
		return "", false
	}
	data := msg[7:]
	ver := "1.5"
	if data[1]&0x80 != 0 && data[3]&0x02 != 0 {
		ver = "2.0"
	}
	return ver, true
}
