// Package vault holds the data-encryption key (DEK) that protects stored BMC
// passwords.
//
// A single random DEK encrypts every secret in the database. Each local user
// keeps their own copy of the DEK, wrapped with a key derived from their login
// password (Argon2id + AES-256-GCM). Logging in therefore both authenticates
// the user and unlocks the vault; the database can be moved to another machine
// as long as someone remembers a password.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	keySize   = 32
	saltSize  = 16
	blobV1    = 0x01
	nonceSize = 12
)

var (
	// ErrBadPassword means the password did not unwrap the data key.
	ErrBadPassword = errors.New("vault: wrong password")
	// ErrCorrupt means an encrypted blob failed authentication.
	ErrCorrupt = errors.New("vault: ciphertext is corrupt or was encrypted with another key")

	wrapAAD = []byte("mdbmc/dek/v1")
)

// KDFParams are the Argon2id cost parameters stored next to each wrapped key.
type KDFParams struct {
	Time      uint32 `json:"t"`
	MemoryKiB uint32 `json:"m"`
	Threads   uint8  `json:"p"`
}

// DefaultKDF follows the OWASP Argon2id baseline with extra memory, which
// costs roughly 100-300 ms on a typical admin workstation.
var DefaultKDF = KDFParams{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}

// NewDataKey returns a fresh random DEK.
func NewDataKey() ([]byte, error) {
	k := make([]byte, keySize)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, nil
}

func deriveKEK(password string, salt []byte, p KDFParams) []byte {
	return argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, keySize)
}

// Wrap encrypts dek with a key derived from password and a new random salt.
func Wrap(dek []byte, password string, p KDFParams) (salt, wrapped []byte, err error) {
	if len(dek) != keySize {
		return nil, nil, fmt.Errorf("vault: data key must be %d bytes", keySize)
	}
	salt = make([]byte, saltSize)
	if _, err = rand.Read(salt); err != nil {
		return nil, nil, err
	}
	kek := deriveKEK(password, salt, p)
	defer Zero(kek)
	box, err := NewBox(kek)
	if err != nil {
		return nil, nil, err
	}
	return salt, box.seal(dek, wrapAAD), nil
}

// Unwrap recovers the DEK. A wrong password yields ErrBadPassword.
func Unwrap(wrapped, salt []byte, password string, p KDFParams) ([]byte, error) {
	kek := deriveKEK(password, salt, p)
	defer Zero(kek)
	box, err := NewBox(kek)
	if err != nil {
		return nil, err
	}
	dek, err := box.open(wrapped, wrapAAD)
	if err != nil {
		return nil, ErrBadPassword
	}
	return dek, nil
}

// Box seals and opens secrets with the DEK.
type Box struct {
	aead cipher.AEAD
}

// NewBox builds an AES-256-GCM box from a 32-byte key.
func NewBox(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("vault: key must be %d bytes", keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) seal(plaintext, aad []byte) []byte {
	out := make([]byte, 1+nonceSize, 1+nonceSize+len(plaintext)+b.aead.Overhead())
	out[0] = blobV1
	if _, err := rand.Read(out[1 : 1+nonceSize]); err != nil {
		panic("vault: crypto/rand failed: " + err.Error())
	}
	return b.aead.Seal(out, out[1:1+nonceSize], plaintext, aad)
}

func (b *Box) open(blob, aad []byte) ([]byte, error) {
	if len(blob) < 1+nonceSize+b.aead.Overhead() || blob[0] != blobV1 {
		return nil, ErrCorrupt
	}
	pt, err := b.aead.Open(nil, blob[1:1+nonceSize], blob[1+nonceSize:], aad)
	if err != nil {
		return nil, ErrCorrupt
	}
	return pt, nil
}

// SealString encrypts s. The purpose string is bound as associated data so a
// ciphertext cannot be replayed into a field with a different purpose.
func (b *Box) SealString(s, purpose string) []byte {
	return b.seal([]byte(s), []byte(purpose))
}

// OpenString decrypts a blob produced by SealString with the same purpose.
func (b *Box) OpenString(blob []byte, purpose string) (string, error) {
	pt, err := b.open(blob, []byte(purpose))
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// Zero overwrites a key buffer.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
