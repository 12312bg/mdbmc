// Command bmcsim runs one or more simulated Supermicro BMCs (IPMI 2.0 +
// Redfish) for trying MDBMC without real hardware.
//
//	go run ./cmd/bmcsim -count 3
//
// Simulator i listens on UDP ipmi-port+i and TCP https-port+i.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/12312bg/mdbmc/internal/bmcsim"
)

func main() {
	count := flag.Int("count", 1, "number of simulated BMCs")
	host := flag.String("host", "127.0.0.1", "listen address")
	ipmiPort := flag.Int("ipmi-port", 6230, "first IPMI (UDP) port")
	httpsPort := flag.Int("https-port", 8443, "first HTTPS (Redfish/Web) port")
	user := flag.String("user", "ADMIN", "BMC username")
	pass := flag.String("password", "ADMIN", "BMC password")
	legacyTLS := flag.Bool("legacy-tls", false, "HTTPS with TLS 1.0 and RSA key exchange only, like old BMC firmware")
	flag.Parse()

	certDir := filepath.Join(os.TempDir(), "mdbmc-bmcsim")
	_ = os.MkdirAll(certDir, 0o700)

	var sims []*bmcsim.Sim
	for i := 0; i < *count; i++ {
		certFile := fmt.Sprintf("bmc-%d.pem", *httpsPort+i)
		if *legacyTLS {
			certFile = fmt.Sprintf("bmc-%d-legacy.pem", *httpsPort+i)
		}
		s, err := bmcsim.Start(bmcsim.Options{
			Index: i, User: *user, Password: *pass, ListenHost: *host,
			IPMIPort: *ipmiPort + i, HTTPSPort: *httpsPort + i,
			CertFile:  filepath.Join(certDir, certFile),
			LegacyTLS: *legacyTLS,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "simulator %d: %v\n", i, err)
			os.Exit(1)
		}
		sims = append(sims, s)
		fmt.Printf("BMC #%d  IPMI udp://%s:%d  Redfish/Web https://%s:%d  (%s / %s)\n",
			i, *host, s.IPMIPort, *host, s.HTTPSPort, *user, *pass)
	}
	fmt.Println("按 Ctrl+C 退出")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	for _, s := range sims {
		s.Close()
	}
}
