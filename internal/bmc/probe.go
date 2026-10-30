package bmc

import (
	"context"
	"encoding/binary"
	"net"
	"strconv"
	"time"
)

// Probe reports whether a BMC answers on the network, without logging in:
// an RMCP/ASF Presence Ping on the IPMI port and a TCP connection to the
// HTTPS port, as enabled. Either answer counts. ctx bounds the wait.
func Probe(ctx context.Context, host string, ipmiPort, httpsPort int, ipmi, https bool) bool {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan bool, 2)
	n := 0
	if https {
		n++
		go func() { results <- tcpOpen(ctx, host, httpsPort) }()
	}
	if ipmi {
		n++
		go func() { results <- asfPing(ctx, host, ipmiPort) }()
	}
	for range n {
		if <-results {
			return true
		}
	}
	return false
}

func tcpOpen(ctx context.Context, host string, port int) bool {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// asfPing sends RMCP/ASF Presence Pings (twice, UDP may drop one) and waits
// for a Presence Pong.
func asfPing(ctx context.Context, host string, port int) bool {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.SetDeadline(time.Now())
	}()
	const asfIANA = 4542
	buf := make([]byte, 64)
	for tag := byte(1); tag <= 2; tag++ {
		ping := []byte{
			0x06, 0x00, 0xFF, 0x06, // RMCP: version, reserved, no ACK, class ASF
			0, 0, 0, 0, // ASF IANA
			0x80, tag, 0x00, 0x00, // Presence Ping, tag, reserved, no data
		}
		binary.BigEndian.PutUint32(ping[4:8], asfIANA)
		if _, err := conn.Write(ping); err != nil {
			return false
		}
		_ = conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		for {
			n, err := conn.Read(buf)
			if err != nil {
				break // timeout or ICMP unreachable: try again or give up
			}
			if n >= 12 && buf[3] == 0x06 && buf[8] == 0x40 {
				return true
			}
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}
