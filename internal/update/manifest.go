// Package update checks the release channel (GitHub Releases of
// 12312bg/mdbmc-release by default) for a newer version, downloads its
// package and installs it.
//
// Every release carries latest.json, a manifest naming the version, the
// release notes and the SHA-256 of each package, and latest.json.sig, an
// Ed25519 signature of the manifest made by CI with a key whose public half
// is built into the program. A package is only installed when the manifest
// verifies against that key and the package matches the manifest's hash, so
// neither the release repository nor a mirror or proxy in between can push
// a modified program.
package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// PublicKey is the base64 Ed25519 public key release manifests are signed
// with, set at build time with
// -ldflags "-X github.com/12312bg/mdbmc/internal/update.PublicKey=...".
// Several keys may be listed, separated by commas, while rotating keys.
var PublicKey = ""

// publicKeyEnv supplies a key to builds without one (development builds);
// release builds ignore it.
const publicKeyEnv = "MDBMC_UPDATE_PUBLIC_KEY"

// Product is the manifest's product name; manifests for anything else are
// refused.
const Product = "mdbmc"

// File names of the manifest and its signature in each release.
const (
	ManifestName  = "latest.json"
	SignatureName = "latest.json.sig"
)

// Mode is how this copy of the program was installed, which decides the
// package an update uses.
type Mode string

const (
	ModeInstaller   Mode = "installer"   // installed with the NSIS installer: run the new installer
	ModePortable    Mode = "portable"    // unpacked from the portable zip: replace the files
	ModeUnsupported Mode = "unsupported" // not Windows: no automatic install
)

// Manifest describes one release.
type Manifest struct {
	Product     string    `json:"product"`
	Version     string    `json:"version"`
	Tag         string    `json:"tag"`
	PublishedAt time.Time `json:"publishedAt"`
	Notes       string    `json:"notes"`
	// Assets maps an install mode to its package.
	Assets map[Mode]Asset `json:"assets"`
}

// Asset is a downloadable package in a release.
type Asset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

var (
	safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	hexHash  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// validate checks the fields used to build URLs and file names.
func (m *Manifest) validate() error {
	if m.Product != Product {
		return fmt.Errorf("更新清单不属于本软件（%q）", m.Product)
	}
	if !ValidVersion(m.Version) {
		return fmt.Errorf("更新清单中的版本号无效: %q", m.Version)
	}
	if !safeName.MatchString(m.Tag) {
		return fmt.Errorf("更新清单中的标签无效: %q", m.Tag)
	}
	for mode, a := range m.Assets {
		if !safeName.MatchString(a.Name) || a.Size <= 0 || !hexHash.MatchString(a.SHA256) {
			return fmt.Errorf("更新清单中的 %s 安装包信息无效", mode)
		}
	}
	return nil
}

// Keys returns the public keys built into the program.
func Keys() ([]ed25519.PublicKey, error) {
	s := PublicKey
	if s == "" {
		s = os.Getenv(publicKeyEnv)
	}
	return ParsePublicKeys(s)
}

// ParsePublicKeys decodes a comma-separated list of base64 public keys.
func ParsePublicKeys(s string) ([]ed25519.PublicKey, error) {
	var keys []ed25519.PublicKey
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(part)
		if err != nil || len(b) != ed25519.PublicKeySize {
			return nil, errors.New("更新签名公钥格式不正确")
		}
		keys = append(keys, ed25519.PublicKey(b))
	}
	return keys, nil
}

// ParsePrivateKey decodes a base64 Ed25519 seed (32 bytes) or private key
// (64 bytes).
func ParsePrivateKey(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, errors.New("签名私钥不是有效的 base64")
	}
	switch len(b) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(b), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(b), nil
	}
	return nil, errors.New("签名私钥长度不正确")
}

// EncodeKey is the base64 text form of a key.
func EncodeKey(k []byte) string { return base64.StdEncoding.EncodeToString(k) }

// Sign returns the signature file content for manifest bytes.
func Sign(manifest []byte, key ed25519.PrivateKey) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, manifest)) + "\n")
}

// errUnsigned is returned when the program has no key to verify with.
var errUnsigned = errors.New("此版本没有内置更新签名公钥（开发版本），无法校验更新")

// ParseManifest verifies the signature of manifest bytes against keys and
// decodes them.
func ParseManifest(data, sig []byte, keys []ed25519.PublicKey) (*Manifest, error) {
	if len(keys) == 0 {
		return nil, errUnsigned
	}
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(s) != ed25519.SignatureSize {
		return nil, errors.New("更新清单的签名格式不正确")
	}
	ok := false
	for _, k := range keys {
		if ed25519.Verify(k, data, s) {
			ok = true
			break
		}
	}
	if !ok {
		return nil, errors.New("更新清单签名校验失败，已拒绝该更新")
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("更新清单格式不正确: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// BuildManifest describes a release from its package files (mode -> path)
// and returns the manifest bytes.
func BuildManifest(ver, tag, notes string, published time.Time, files map[Mode]string) ([]byte, error) {
	m := Manifest{
		Product: Product, Version: strings.TrimPrefix(ver, "v"), Tag: tag,
		PublishedAt: published.UTC().Truncate(time.Second), Notes: notes, Assets: map[Mode]Asset{},
	}
	for mode, path := range files {
		sum, size, err := hashFile(path)
		if err != nil {
			return nil, err
		}
		name := path[strings.LastIndexAny(path, `/\`)+1:]
		m.Assets[mode] = Asset{Name: name, Size: size, SHA256: sum}
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// verifyFile checks that path is the asset described by a.
func verifyFile(path string, a Asset) error {
	sum, size, err := hashFile(path)
	if err != nil {
		return err
	}
	if size != a.Size || sum != a.SHA256 {
		return errors.New("安装包校验失败（大小或 SHA-256 与更新清单不一致）")
	}
	return nil
}
