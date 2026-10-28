package bmc

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FingerprintMismatchError is returned when a BMC presents a certificate that
// differs from the pinned one. The caller may ask the user to trust Observed.
type FingerprintMismatchError struct {
	Expected string
	Observed string
}

func (e *FingerprintMismatchError) Error() string {
	return fmt.Sprintf("BMC 证书指纹与已信任的不一致（已信任 %s，当前 %s）。如果确认 BMC 更换过证书，请在设备详情中重新信任", short(e.Expected), short(e.Observed))
}

func short(fp string) string {
	if len(fp) > 16 {
		return fp[:16] + "…"
	}
	return fp
}

// Fingerprint returns the hex SHA-256 of a DER certificate.
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// pinner verifies the BMC leaf certificate against a pinned fingerprint.
// BMCs nearly always use self-signed certificates, so chain validation is
// replaced by trust-on-first-use pinning.
type pinner struct {
	expected string
	mu       sync.Mutex
	observed string
	spki     string
}

func (p *pinner) Observed() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.observed
}

// mismatchOr returns a FingerprintMismatchError when the handshake saw an
// unexpected certificate (libraries may flatten the original error), else err.
func (p *pinner) mismatchOr(err error) error {
	var mm *FingerprintMismatchError
	if errors.As(err, &mm) {
		return mm
	}
	if obs := p.Observed(); p.expected != "" && obs != "" && !strings.EqualFold(obs, p.expected) {
		return &FingerprintMismatchError{Expected: p.expected, Observed: obs}
	}
	return err
}

func (p *pinner) tlsConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // replaced by VerifyPeerCertificate below
		MinVersion:         tls.VersionTLS10,
		CipherSuites:       compatCipherSuites(),
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("BMC 未提供证书")
			}
			fp := Fingerprint(rawCerts[0])
			spki := ""
			if cert, err := x509.ParseCertificate(rawCerts[0]); err == nil {
				sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
				spki = base64.StdEncoding.EncodeToString(sum[:])
			}
			p.mu.Lock()
			p.observed, p.spki = fp, spki
			p.mu.Unlock()
			if p.expected != "" && !strings.EqualFold(fp, p.expected) {
				return &FingerprintMismatchError{Expected: p.expected, Observed: fp}
			}
			return nil
		},
	}
}

// compatCipherSuites enables the legacy suites that older BMC firmware still
// requires. Pinning, not cipher strength, is what authenticates the peer here.
func compatCipherSuites() []uint16 {
	var ids []uint16
	for _, cs := range tls.CipherSuites() {
		ids = append(ids, cs.ID)
	}
	for _, cs := range tls.InsecureCipherSuites() {
		if strings.Contains(cs.Name, "RC4") {
			continue
		}
		ids = append(ids, cs.ID)
	}
	return ids
}

func newHTTPClient(p *pinner, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               nil, // BMCs live on the local management network
			DialContext:         dialer.DialContext,
			TLSClientConfig:     p.tlsConfig(),
			TLSHandshakeTimeout: 10 * time.Second,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

// CertInfo identifies a BMC certificate.
type CertInfo struct {
	// Fingerprint is the hex SHA-256 of the whole certificate (the pin).
	Fingerprint string
	// SPKIHash is the base64 SHA-256 of the public key, the form Chromium's
	// --ignore-certificate-errors-spki-list expects. Empty if the
	// certificate could not be parsed.
	SPKIHash string
}

// ProbeCertificate connects to host:port and returns the certificate it
// presents. With expected set, a different certificate is an error.
func ProbeCertificate(ctx context.Context, host string, port int, expected string) (CertInfo, error) {
	p := &pinner{expected: expected}
	d := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 6 * time.Second}, Config: p.tlsConfig()}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	p.mu.Lock()
	info := CertInfo{Fingerprint: p.observed, SPKIHash: p.spki}
	p.mu.Unlock()
	if err != nil {
		var mm *FingerprintMismatchError
		if errors.As(err, &mm) {
			return info, mm
		}
		return info, fmt.Errorf("无法建立 HTTPS 连接: %w", err)
	}
	conn.Close()
	return info, nil
}

func baseURL(host string, port int) string {
	h := host
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		h = "[" + h + "]"
	}
	if port == 0 || port == 443 {
		return "https://" + h
	}
	return "https://" + h + ":" + strconv.Itoa(port)
}
