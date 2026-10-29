package update

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSource serves a release in the GitHub Releases layout.
type fakeSource struct {
	t        *testing.T
	srv      *httptest.Server
	priv     ed25519.PrivateKey
	pub      ed25519.PublicKey
	manifest []byte
	sig      []byte
	files    map[string][]byte // tag/name -> content served
	mu       sync.Mutex
	ranges   []string
	block    chan struct{} // when set, asset downloads stall until closed
}

func newFakeSource(t *testing.T) *fakeSource {
	f := &fakeSource{t: t, files: map[string][]byte{}}
	f.pub, f.priv = testKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /releases/latest/download/{name}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("name") {
		case ManifestName:
			w.Write(f.manifest)
		case SignatureName:
			w.Write(f.sig)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("GET /releases/download/{tag}/{name}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.ranges = append(f.ranges, r.Header.Get("Range"))
		body, ok := f.files[r.PathValue("tag")+"/"+r.PathValue("name")]
		block := f.block
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		if block != nil {
			w.Header().Set("Content-Length", "1000000")
			w.WriteHeader(http.StatusOK)
			w.Write(body[:1])
			w.(http.Flusher).Flush()
			select {
			case <-block:
			case <-r.Context().Done():
			}
			return
		}
		http.ServeContent(w, r, r.PathValue("name"), time.Time{}, strings.NewReader(string(body)))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSource) source() string { return f.srv.URL + "/releases" }

// publish makes version the latest release with a portable package.
func (f *fakeSource) publish(version string, pkg []byte) {
	f.t.Helper()
	dir := f.t.TempDir()
	name := "mdbmc-portable-windows-amd64.zip"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, pkg, 0o600); err != nil {
		f.t.Fatal(err)
	}
	data, err := BuildManifest(version, "v"+version, "更新说明 "+version, time.Now(), map[Mode]string{ModePortable: p})
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.manifest, f.sig = data, Sign(data, f.priv)
	f.files["v"+version+"/"+name] = pkg
	f.mu.Unlock()
}

func newTestUpdater(t *testing.T, f *fakeSource, current string) (*Updater, *atomic.Int32) {
	var changes atomic.Int32
	u := New(Options{
		Current: current, Mode: ModePortable, Dir: t.TempDir(),
		Keys: []ed25519.PublicKey{f.pub}, OnChange: func() { changes.Add(1) },
	})
	u.SetConfig(Config{Source: f.source()})
	return u, &changes
}

func waitSettled(t *testing.T, u *Updater) Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := u.Status(); st.State != StateDownloading {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("download did not finish")
	return Status{}
}

func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return b
}

func TestCheckUpToDate(t *testing.T) {
	f := newFakeSource(t)
	f.publish("1.0.0", payload(10))
	u, _ := newTestUpdater(t, f, "1.0.0")
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := u.Status(); st.State != StateUpToDate || st.Latest != nil || st.CheckedAt.IsZero() {
		t.Fatalf("status %+v", st)
	}
	// Download has nothing to do.
	if err := u.Download(); err == nil {
		t.Fatal("download without a newer version")
	}
}

func TestCheckDownloadAndPrepare(t *testing.T) {
	f := newFakeSource(t)
	pkg := payload(300 << 10)
	f.publish("1.1.0", pkg)
	u, changes := newTestUpdater(t, f, "1.0.0")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(u.Check(context.Background()))
	st := u.Status()
	if st.State != StateAvailable || st.Latest.Version != "1.1.0" || st.Latest.Notes != "更新说明 1.1.0" {
		t.Fatalf("status %+v", st)
	}
	must(u.Download())
	st = waitSettled(t, u)
	if st.State != StateReady || st.Error != "" || st.Downloaded != int64(len(pkg)) {
		t.Fatalf("after download: %+v", st)
	}
	if changes.Load() < 3 {
		t.Fatalf("only %d change notifications", changes.Load())
	}
	path, m, a, err := u.Prepared()
	must(err)
	if m.Version != "1.1.0" || filepath.Base(path) != a.Name {
		t.Fatalf("prepared %s %+v", path, m)
	}

	// A new check of the same release keeps the download.
	must(u.Check(context.Background()))
	if st := u.Status(); st.State != StateReady {
		t.Fatalf("re-check: %+v", st)
	}
	// So does a fresh updater (program restarted) finding the file.
	u2 := New(Options{Current: "1.0.0", Mode: ModePortable, Dir: u.opts.Dir, Keys: u.opts.Keys})
	u2.SetConfig(u.Config())
	must(u2.Check(context.Background()))
	if st := u2.Status(); st.State != StateReady {
		t.Fatalf("restart: %+v", st)
	}

	// A changed file on disk is caught before installing.
	must(os.WriteFile(path, []byte("tampered"), 0o600))
	if _, _, _, err := u.Prepared(); err == nil {
		t.Fatal("tampered package prepared")
	}
	if st := u.Status(); st.State != StateAvailable || st.Error == "" {
		t.Fatalf("after tampering: %+v", st)
	}

	// A newer release replaces the old download.
	f.publish("1.2.0", payload(1000))
	must(u.Check(context.Background()))
	if st := u.Status(); st.State != StateAvailable || st.Latest.Version != "1.2.0" {
		t.Fatalf("newer release: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(u.opts.Dir, "1.1.0")); !os.IsNotExist(err) {
		t.Fatal("old download not removed")
	}
}

func TestDownloadResumes(t *testing.T) {
	f := newFakeSource(t)
	pkg := payload(100 << 10)
	f.publish("1.1.0", pkg)
	u, _ := newTestUpdater(t, f, "1.0.0")
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := u.Status().Latest
	part := u.packagePath(m, m.Assets[ModePortable]) + ".part"
	if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part, pkg[:40<<10], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	if st := waitSettled(t, u); st.State != StateReady {
		t.Fatalf("status %+v", st)
	}
	if got := f.ranges[len(f.ranges)-1]; got != "bytes=40960-" {
		t.Fatalf("range header %q", got)
	}
	if _, _, _, err := u.Prepared(); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadRejectsWrongContent(t *testing.T) {
	f := newFakeSource(t)
	f.publish("1.1.0", payload(5000))
	// The server delivers different bytes of the same size.
	for k := range f.files {
		f.files[k] = payload(5001)[1:]
	}
	u, _ := newTestUpdater(t, f, "1.0.0")
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	st := waitSettled(t, u)
	if st.State != StateAvailable || !strings.Contains(st.Error, "校验失败") {
		t.Fatalf("status %+v", st)
	}
	m := st.Latest
	if fi, err := os.Stat(u.packagePath(m, m.Assets[ModePortable]) + ".part"); err == nil && fi.Size() != 0 {
		t.Fatal("bad partial download kept for resuming")
	}
}

func TestCheckRejectsBadSignature(t *testing.T) {
	f := newFakeSource(t)
	f.publish("1.1.0", payload(10))
	_, otherPriv := testKey(t)
	f.sig = Sign(f.manifest, otherPriv)
	u, _ := newTestUpdater(t, f, "1.0.0")
	err := u.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "签名") {
		t.Fatalf("err %v", err)
	}
	if st := u.Status(); st.Latest != nil || st.Error == "" {
		t.Fatalf("status %+v", st)
	}
}

func TestCheckNoRelease(t *testing.T) {
	f := newFakeSource(t)
	u, _ := newTestUpdater(t, f, "1.0.0")
	f.manifest = nil
	u.SetConfig(Config{Source: f.srv.URL + "/missing"})
	if err := u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err %v", err)
	}
}

func TestDownloadCancel(t *testing.T) {
	f := newFakeSource(t)
	f.publish("1.1.0", payload(10))
	f.block = make(chan struct{})
	defer close(f.block)
	u, _ := newTestUpdater(t, f, "1.0.0")
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	for u.Status().Downloaded == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	u.Cancel()
	if st := waitSettled(t, u); st.State != StateAvailable || st.Error != "" {
		t.Fatalf("after cancel: %+v", st)
	}
}
