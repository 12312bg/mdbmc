package update

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestManifestSignVerify(t *testing.T) {
	pub, priv := testKey(t)
	dir := t.TempDir()
	inst := writeFile(t, dir, "mdbmc-amd64-installer.exe", "installer bytes")
	data, err := BuildManifest("v1.2.3", "v1.2.3", "- 新功能", time.Now(), map[Mode]string{ModeInstaller: inst})
	if err != nil {
		t.Fatal(err)
	}
	sig := Sign(data, priv)

	m, err := ParseManifest(data, sig, []ed25519.PublicKey{pub})
	if err != nil {
		t.Fatal(err)
	}
	a := m.Assets[ModeInstaller]
	if m.Version != "1.2.3" || m.Tag != "v1.2.3" || a.Name != "mdbmc-amd64-installer.exe" || a.Size != 15 {
		t.Fatalf("manifest %+v", m)
	}
	if err := verifyFile(inst, a); err != nil {
		t.Fatal(err)
	}

	// Any change to the manifest breaks the signature.
	tampered := []byte(strings.Replace(string(data), "1.2.3", "9.9.9", 1))
	if _, err := ParseManifest(tampered, sig, []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("tampered manifest accepted")
	}
	// So does another key; with several keys any one may match.
	other, _ := testKey(t)
	if _, err := ParseManifest(data, sig, []ed25519.PublicKey{other}); err == nil {
		t.Fatal("manifest accepted with the wrong key")
	}
	if _, err := ParseManifest(data, sig, []ed25519.PublicKey{other, pub}); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseManifest(data, sig, nil); err != errUnsigned {
		t.Fatalf("no keys: %v", err)
	}
	if err := os.WriteFile(inst, []byte("installer bytez"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyFile(inst, a); err == nil {
		t.Fatal("modified package accepted")
	}
}

func TestManifestRejectsUnsafeFields(t *testing.T) {
	pub, priv := testKey(t)
	good := Manifest{Product: Product, Version: "1.0.0", Tag: "v1.0.0",
		Assets: map[Mode]Asset{ModePortable: {Name: "a.zip", Size: 1, SHA256: strings.Repeat("a", 64)}}}
	for name, mutate := range map[string]func(*Manifest){
		"product": func(m *Manifest) { m.Product = "other" },
		"version": func(m *Manifest) { m.Version = "1.0" },
		"tag":     func(m *Manifest) { m.Tag = "../v1" },
		"asset name": func(m *Manifest) {
			m.Assets[ModePortable] = Asset{Name: `..\evil.exe`, Size: 1, SHA256: strings.Repeat("a", 64)}
		},
		"hash": func(m *Manifest) { m.Assets[ModePortable] = Asset{Name: "a.zip", Size: 1, SHA256: "xyz"} },
	} {
		m := good
		m.Assets = map[Mode]Asset{ModePortable: good.Assets[ModePortable]}
		mutate(&m)
		data, _ := json.Marshal(m)
		if _, err := ParseManifest(data, Sign(data, priv), []ed25519.PublicKey{pub}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestKeys(t *testing.T) {
	pub, priv := testKey(t)
	keys, err := ParsePublicKeys(" " + EncodeKey(pub) + " ,")
	if err != nil || len(keys) != 1 || !keys[0].Equal(pub) {
		t.Fatalf("ParsePublicKeys: %v %v", keys, err)
	}
	if _, err := ParsePublicKeys("bm90IGEga2V5"); err == nil {
		t.Fatal("short key accepted")
	}
	for _, s := range []string{EncodeKey(priv.Seed()), EncodeKey(priv)} {
		k, err := ParsePrivateKey(s)
		if err != nil || !k.Equal(priv) {
			t.Fatalf("ParsePrivateKey: %v", err)
		}
	}

	old := PublicKey
	defer func() { PublicKey = old }()
	PublicKey = ""
	t.Setenv(publicKeyEnv, EncodeKey(pub))
	if keys, _ := Keys(); len(keys) != 1 {
		t.Fatal("development key from the environment not used")
	}
	// A release build ignores the environment.
	other, _ := testKey(t)
	PublicKey = EncodeKey(other)
	if keys, _ := Keys(); len(keys) != 1 || !keys[0].Equal(other) {
		t.Fatal("built-in key overridden by the environment")
	}
}

func TestNormalize(t *testing.T) {
	if s, err := NormalizeSource(" https://mirror.example/releases/ "); err != nil || s != "https://mirror.example/releases" {
		t.Fatalf("source: %q %v", s, err)
	}
	for _, bad := range []string{"ftp://x", "https://", "https://x/?a=1", "x"} {
		if _, err := NormalizeSource(bad); err == nil {
			t.Errorf("source %q accepted", bad)
		}
	}
	if s, err := NormalizeProxy("10.0.0.1:3128"); err != nil || s != "http://10.0.0.1:3128" {
		t.Fatalf("proxy: %q %v", s, err)
	}
	if _, err := NormalizeProxy("socks5://127.0.0.1:1080"); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeProxy("ftp://x:1"); err == nil {
		t.Fatal("ftp proxy accepted")
	}
	if got := assetURL("", "v1.0.0", "a b.zip"); got != DefaultSource+"/download/v1.0.0/a%20b.zip" {
		t.Fatal(got)
	}
}
