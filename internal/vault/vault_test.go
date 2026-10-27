package vault

import (
	"bytes"
	"errors"
	"testing"
)

var fastKDF = KDFParams{Time: 1, MemoryKiB: 8 * 1024, Threads: 1}

func TestWrapUnwrap(t *testing.T) {
	dek, err := NewDataKey()
	if err != nil {
		t.Fatal(err)
	}
	salt, wrapped, err := Wrap(dek, "correct horse", fastKDF)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unwrap(wrapped, salt, "correct horse", fastKDF)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, dek) {
		t.Fatal("unwrapped key differs")
	}
	if _, err := Unwrap(wrapped, salt, "wrong", fastKDF); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("want ErrBadPassword, got %v", err)
	}
}

func TestBoxPurposeBinding(t *testing.T) {
	dek, _ := NewDataKey()
	box, err := NewBox(dek)
	if err != nil {
		t.Fatal(err)
	}
	blob := box.SealString("s3cret", "device-password")
	if s, err := box.OpenString(blob, "device-password"); err != nil || s != "s3cret" {
		t.Fatalf("open: %q %v", s, err)
	}
	if _, err := box.OpenString(blob, "credential-password"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("purpose mismatch should fail, got %v", err)
	}
	other, _ := NewDataKey()
	box2, _ := NewBox(other)
	if _, err := box2.OpenString(blob, "device-password"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("other key should fail, got %v", err)
	}
	if bytes.Equal(blob, box.SealString("s3cret", "device-password")) {
		t.Fatal("nonce reuse: identical ciphertexts")
	}
}
