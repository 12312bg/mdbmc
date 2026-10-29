// Package bmcsim runs a simulated Supermicro-like BMC that speaks IPMI 2.0
// (RMCP+, via the go-ipmi reference server) and Redfish over HTTPS, sharing
// one power/sensor/SEL state. It backs integration tests and lets the
// application be tried without real hardware (see cmd/bmcsim).
package bmcsim

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bougou/go-ipmi/pkg/bmc"
	"github.com/bougou/go-ipmi/pkg/clock"
	"github.com/bougou/go-ipmi/pkg/hal/mock"
	"github.com/bougou/go-ipmi/pkg/handlers"
	"github.com/bougou/go-ipmi/pkg/server"
	"github.com/bougou/go-ipmi/pkg/transport/udp"
)

// Options configures a simulator.
type Options struct {
	// Index distinguishes several simulators (serial numbers, names).
	Index    int
	User     string
	Password string
	// ListenHost is the address both servers bind to (default 127.0.0.1).
	ListenHost string
	// IPMIPort / HTTPSPort; 0 picks a free port.
	IPMIPort  int
	HTTPSPort int
	// CertFile stores the self-signed certificate so its fingerprint stays
	// stable across restarts; empty generates an ephemeral one.
	CertFile string
	// DisableRedfish / DisableIPMI simulate single-protocol BMCs.
	DisableRedfish bool
	DisableIPMI    bool
	// LegacyTLS limits HTTPS to TLS 1.0 with RSA key exchange, like old BMC
	// firmware that current browsers (and WebView2) refuse to talk to.
	LegacyTLS bool
}

// Sim is a running simulator.
type Sim struct {
	opt         Options
	IPMIPort    int
	HTTPSPort   int
	Fingerprint string

	state *state
	hal   *mock.HAL

	ipmiSrv  *server.Server
	httpSrv  *http.Server
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// Start launches the simulator.
func Start(opt Options) (*Sim, error) {
	if opt.User == "" {
		opt.User = "ADMIN"
	}
	if opt.Password == "" {
		opt.Password = "ADMIN"
	}
	if opt.ListenHost == "" {
		opt.ListenHost = "127.0.0.1"
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Sim{opt: opt, cancel: cancel, hal: mock.New()}
	s.state = newState(opt.Index, s.hal)
	if err := s.hal.Chassis().SetPower(ctx, true); err != nil {
		cancel()
		return nil, err
	}

	if !opt.DisableIPMI {
		if err := s.startIPMI(ctx); err != nil {
			s.Close()
			return nil, fmt.Errorf("ipmi: %w", err)
		}
	}
	if !opt.DisableRedfish {
		if err := s.startHTTPS(); err != nil {
			s.Close()
			return nil, fmt.Errorf("https: %w", err)
		}
	}
	return s, nil
}

// Close stops both servers.
func (s *Sim) Close() {
	s.stopOnce.Do(func() {
		s.cancel()
		if s.ipmiSrv != nil {
			_ = s.ipmiSrv.Close()
		}
		if s.httpSrv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.httpSrv.Shutdown(ctx)
			cancel()
		}
		s.wg.Wait()
	})
}

// PowerOn reports the simulated host power state.
func (s *Sim) PowerOn() bool {
	on, _ := s.hal.Chassis().PowerState(context.Background())
	return on
}

func (s *Sim) startIPMI(ctx context.Context) error {
	info := bmc.DeviceInfo{
		DeviceID:                0x20,
		DeviceRevision:          1,
		FirmwareMajor:           1,
		FirmwareMinor:           0x73,
		IPMIVersion:             0x20,
		ManufacturerID:          10876, // Supermicro
		ProductID:               0x1b11,
		AdditionalDeviceSupport: 0x3f,
	}
	var guid [16]byte
	copy(guid[:], fmt.Sprintf("mdbmc-sim-%05d", s.opt.Index))

	if err := s.state.seedStorage(ctx); err != nil {
		return err
	}
	b := bmc.New(info, guid, s.hal, bmc.WithClock(clock.Real))
	user, err := b.Users.Add(2, s.opt.User)
	if err != nil {
		return err
	}
	user.SetPassword([]byte(s.opt.Password))
	user.Enabled = true
	user.ChannelAccess[1] = bmc.UserChannelAccess{MaxPrivilege: bmc.PrivilegeLevelAdministrator, Enabled: true}

	conn, err := udp.Listen(net.JoinHostPort(s.opt.ListenHost, strconv.Itoa(s.opt.IPMIPort)))
	if err != nil {
		return err
	}
	if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		s.IPMIPort = ua.Port
	}

	reg := handlers.NewRegistry()
	handlers.RegisterAllHandlers(reg)
	s.state.registerHandlers(reg)

	s.ipmiSrv = server.NewServer(b, conn, server.WithHandlerRegistry(reg))
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer conn.Close()
		if err := s.ipmiSrv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "bmcsim: ipmi serve: %v\n", err)
		}
	}()
	return nil
}

func (s *Sim) startHTTPS() error {
	cert, err := loadOrCreateCert(s.opt.CertFile, s.opt.LegacyTLS)
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
	if s.opt.LegacyTLS {
		tlsConfig.MinVersion = tls.VersionTLS10
		tlsConfig.MaxVersion = tls.VersionTLS10
		tlsConfig.CipherSuites = []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_RSA_WITH_AES_256_CBC_SHA}
		tlsConfig.NextProtos = []string{"http/1.1"} // HTTP/2 needs TLS 1.2
	}
	sum := sha256.Sum256(cert.Certificate[0])
	s.Fingerprint = hex.EncodeToString(sum[:])

	ln, err := net.Listen("tcp", net.JoinHostPort(s.opt.ListenHost, strconv.Itoa(s.opt.HTTPSPort)))
	if err != nil {
		return err
	}
	s.HTTPSPort = ln.Addr().(*net.TCPAddr).Port
	s.httpSrv = &http.Server{
		Handler:           s.routes(),
		TLSConfig:         tlsConfig,
		TLSNextProto:      map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.httpSrv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "bmcsim: https serve: %v\n", err)
		}
	}()
	return nil
}

// loadOrCreateCert returns the simulator's certificate, with an RSA key
// when rsaKey is set (RSA key exchange needs one), else ECDSA.
func loadOrCreateCert(path string, rsaKey bool) (tls.Certificate, error) {
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			if c, err := tls.X509KeyPair(b, b); err == nil {
				if _, isRSA := c.PrivateKey.(*rsa.PrivateKey); isRSA == rsaKey {
					return c, nil
				}
			}
		}
	}
	var key crypto.Signer
	var err error
	if rsaKey {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "IPMI", Organization: []string{"Super Micro Computer (simulated)"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	if rsaKey {
		tmpl.KeyUsage |= x509.KeyUsageKeyEncipherment
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	pemBytes := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})...)
	if path != "" {
		_ = os.WriteFile(path, pemBytes, 0o600)
	}
	return tls.X509KeyPair(pemBytes, pemBytes)
}
