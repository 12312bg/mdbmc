// Command release prepares the update files of a release.
//
// Create the signing key pair once (keep the private key secret):
//
//	go run ./cmd/release genkey -out mdbmc-update.key
//
// CI then writes latest.json and latest.json.sig for each release, signed
// with the private key from $MDBMC_UPDATE_SIGNING_KEY and checked against
// the public key built into the program:
//
//	go run ./cmd/release manifest -version 1.2.3 -tag v1.2.3 -notes notes.md \
//	    -installer build/bin/mdbmc-amd64-installer.exe \
//	    -portable build/bin/mdbmc-portable-windows-amd64.zip \
//	    -pubkey "$UPDATE_PUBLIC_KEY" -out dist
package main

import (
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/12312bg/mdbmc/internal/update"
)

const signingKeyEnv = "MDBMC_UPDATE_SIGNING_KEY"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "genkey":
		err = genkey(os.Args[2:])
	case "manifest":
		err = manifest(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: release genkey [-out file] | release manifest -version v -tag t [-notes file] [-installer file] [-portable file] -pubkey key [-out dir]")
	os.Exit(2)
}

func genkey(args []string) error {
	fs := flag.NewFlagSet("genkey", flag.ExitOnError)
	out := fs.String("out", "mdbmc-update.key", "file for the private key")
	fs.Parse(args)
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, update.EncodeKey(priv.Seed())); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("私钥已写入 %s（填入 GitHub Secret UPDATE_SIGNING_KEY，然后离线妥善保存，不要提交到仓库）\n", *out)
	fmt.Printf("公钥（填入 GitHub Variable UPDATE_PUBLIC_KEY）：\n%s\n", update.EncodeKey(pub))
	return nil
}

func manifest(args []string) error {
	fs := flag.NewFlagSet("manifest", flag.ExitOnError)
	ver := fs.String("version", "", "version, e.g. 1.2.3")
	tag := fs.String("tag", "", "release tag, e.g. v1.2.3")
	notesFile := fs.String("notes", "", "release notes (Markdown)")
	installer := fs.String("installer", "", "NSIS installer")
	portable := fs.String("portable", "", "portable zip")
	pubkey := fs.String("pubkey", "", "public key built into the program, to check the signature")
	out := fs.String("out", ".", "output directory")
	fs.Parse(args)

	if *ver == "" || *tag == "" {
		return errors.New("-version and -tag are required")
	}
	priv, err := update.ParsePrivateKey(os.Getenv(signingKeyEnv))
	if err != nil {
		return fmt.Errorf("$%s: %w", signingKeyEnv, err)
	}
	keys, err := update.ParsePublicKeys(*pubkey)
	if err != nil || len(keys) == 0 {
		return errors.New("-pubkey is missing or invalid")
	}
	notes := ""
	if *notesFile != "" {
		b, err := os.ReadFile(*notesFile)
		if err != nil {
			return err
		}
		notes = string(b)
	}
	files := map[update.Mode]string{}
	if *installer != "" {
		files[update.ModeInstaller] = *installer
	}
	if *portable != "" {
		files[update.ModePortable] = *portable
	}
	if len(files) == 0 {
		return errors.New("no package given")
	}
	data, err := update.BuildManifest(*ver, *tag, notes, time.Now(), files)
	if err != nil {
		return err
	}
	sig := update.Sign(data, priv)
	// Catch a signing key that does not match the program's public key
	// before anything is published.
	if _, err := update.ParseManifest(data, sig, keys); err != nil {
		return fmt.Errorf("the signing key does not match -pubkey: %w", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, update.ManifestName), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, update.SignatureName), sig, 0o644); err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
