package bmc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/bmcsim"
)

func TestProbe(t *testing.T) {
	sim, err := bmcsim.Start(bmcsim.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer sim.Close()
	// A port nothing listens on, for TCP and UDP alike.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l.Addr().(*net.TCPAddr).Port
	l.Close()

	probe := func(ipmiPort, httpsPort int, ipmi, https bool) bool {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		return bmc.Probe(ctx, "127.0.0.1", ipmiPort, httpsPort, ipmi, https)
	}
	if !probe(sim.IPMIPort, 0, true, false) {
		t.Error("IPMI presence ping not answered")
	}
	if !probe(0, sim.HTTPSPort, false, true) {
		t.Error("HTTPS port not reachable")
	}
	if !probe(closed, sim.HTTPSPort, true, true) {
		t.Error("one answering protocol should be enough")
	}
	start := time.Now()
	if probe(closed, closed, true, true) {
		t.Error("closed ports reported reachable")
	}
	if time.Since(start) > 4*time.Second {
		t.Error("probe ignored its deadline")
	}
}
