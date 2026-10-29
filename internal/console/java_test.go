package console

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// inspurJNLP has the shape of the JNLP an Inspur (AMI MegaRAC) BMC returns
// for its Java console; the token and cookie are placeholders.
const inspurJNLP = `<?xml version="1.0" encoding="UTF-8"?>
<jnlp spec="1.0+" codebase="https://172.16.1.200:443/Java">
  <information><title>JViewer</title><vendor>American Megatrends, Inc.</vendor></information>
  <security><all-permissions/></security>
  <resources><j2se version="1.5+"/><jar href="release/JViewer.jar"/></resources>
  <resources><j2se version="1.5+"/><jar href="release/JViewer-SOC.jar"/></resources>
  <resources os="Windows" arch="x86"><nativelib href="release/Win32.jar"/></resources>
  <resources os="Windows" arch="amd64"><nativelib href="release/Win64.jar"/></resources>
  <resources os="Linux" arch="x86_64"><nativelib href="release/Linux_x86_64.jar"/></resources>
  <resources os="Linux" arch="amd64"><nativelib href="release/Linux_x86_64.jar"/></resources>
  <resources os="Mac OS X" arch="x86_64"><nativelib href="release/Mac64.jar"/></resources>
  <application-desc>
    <argument>-apptype</argument><argument>JViewer</argument>
    <argument>-hostname</argument><argument>172.16.1.200</argument>
    <argument>-kvmtoken</argument><argument>TOKEN</argument>
    <argument>-kvmport</argument><argument>7578</argument>
    <argument>-webcookie</argument><argument>COOKIE</argument>
  </application-desc>
</jnlp>`

func urls(us []*url.URL) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.String())
	}
	return out
}

func TestParseInspurJNLP(t *testing.T) {
	src, _ := url.Parse("https://127.0.0.1:52405/Java/jviewer.jnlp?EXTRNIP=127.0.0.1&JNLPSTR=JViewer")
	plan, err := parseJNLP([]byte(inspurJNLP), src, platformFor("windows", "amd64"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://172.16.1.200:443/Java/release/JViewer.jar", "https://172.16.1.200:443/Java/release/JViewer-SOC.jar"}
	if got := urls(plan.Jars); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("jars = %v", got)
	}
	if got := urls(plan.NativeLibs); len(got) != 1 || !strings.HasSuffix(got[0], "/release/Win64.jar") {
		t.Fatalf("Windows x64 native libraries = %v", got)
	}
	if plan.MainClass != "" || plan.Title != "JViewer" || len(plan.Args) != 10 || plan.Args[3] != "172.16.1.200" {
		t.Fatalf("plan = %+v", plan)
	}
	plan, _ = parseJNLP([]byte(inspurJNLP), src, platformFor("linux", "amd64"))
	if got := urls(plan.NativeLibs); len(got) != 1 || !strings.HasSuffix(got[0], "/Linux_x86_64.jar") {
		t.Fatalf("Linux native libraries = %v", got)
	}
}

func TestJNLPMapToBMC(t *testing.T) {
	bmcURL, _ := url.Parse("https://10.0.0.5")
	src, _ := url.Parse("https://127.0.0.1:52405/viewer.jnlp")
	doc := `<jnlp codebase="https://127.0.0.1:52405/Java"><resources><jar href="a.jar"/><jar href="http://10.0.0.5/b.jar"/></resources>
<application-desc main-class="x.Main"><argument>-hostname</argument><argument>127.0.0.1</argument><argument>https://127.0.0.1:52405/kvm</argument></application-desc></jnlp>`
	plan, err := parseJNLP([]byte(doc), src, platformFor("windows", "amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.mapToBMC("127.0.0.1:52405", bmcURL); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(urls(plan.Jars), " "); got != "https://10.0.0.5/Java/a.jar https://10.0.0.5/b.jar" {
		t.Fatalf("jars = %s (plain HTTP must become the pinned HTTPS)", got)
	}
	if got := strings.Join(plan.Args, " "); got != "-hostname 10.0.0.5 https://10.0.0.5/kvm" {
		t.Fatalf("args = %s", got)
	}

	foreign := `<jnlp codebase="https://evil.example/"><resources><jar href="a.jar"/></resources><application-desc/></jnlp>`
	plan, _ = parseJNLP([]byte(foreign), src, platformFor("windows", "amd64"))
	if err := plan.mapToBMC("127.0.0.1:52405", bmcURL); err == nil {
		t.Fatal("jars from another server must be refused")
	}
	if _, err := parseJNLP([]byte(`<jnlp><applet-desc/></jnlp>`), src, platformFor("windows", "amd64")); err == nil || !strings.Contains(err.Error(), "Applet") {
		t.Fatalf("applet JNLP: %v", err)
	}
}

func TestLooksLikeJNLP(t *testing.T) {
	for _, c := range []struct {
		ct, path string
		want     bool
	}{
		{"application/x-java-jnlp-file", "/cgi/url_redirect.cgi", true},
		{"application/octet-stream", "/Java/jviewer.jnlp", true},
		{"text/html; charset=utf-8", "/Java/jviewer.jnlp", false}, // e.g. "session expired"
		{"text/html", "/index.html", false},
	} {
		if got := looksLikeJNLP(c.ct, c.path); got != c.want {
			t.Errorf("looksLikeJNLP(%q, %q) = %v", c.ct, c.path, got)
		}
	}
}

// makeJar builds a jar (zip) from name/content pairs.
func makeJar(t *testing.T, files ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for i := 0; i+1 < len(files); i += 2 {
		w, err := zw.Create(files[i])
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(files[i+1]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestManifestAndNatives(t *testing.T) {
	dir := t.TempDir()
	jar := filepath.Join(dir, "app.jar")
	os.WriteFile(jar, makeJar(t, "META-INF/MANIFEST.MF", "Manifest-Version: 1.0\r\nMain-Class: com.ami.kvm.jviewer.JV\r\n iewer\r\n\r\n"), 0o600)
	if main, err := manifestMainClass(jar); err != nil || main != "com.ami.kvm.jviewer.JViewer" {
		t.Fatalf("Main-Class = %q, %v", main, err)
	}
	natives := filepath.Join(dir, "Win64.jar")
	os.WriteFile(natives, makeJar(t, "../../evil.dll", "x", "win64/kvm.dll", "y", "readme.txt", "z"), 0o600)
	out := filepath.Join(dir, "native")
	if err := extractNatives(natives, out); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(out)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "evil.dll,kvm.dll" {
		t.Fatalf("extracted %v; only libraries, by base name", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.dll")); err == nil {
		t.Fatal("zip entry escaped the target directory")
	}
}

// TestProxyStartsJavaConsole serves a JNLP through the proxy and checks that
// the console starts java with the downloaded jars instead of handing the
// file to the browser.
func TestProxyStartsJavaConsole(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as java; Windows runs a real JVM in the smoke test")
	}
	jar := makeJar(t, "META-INF/MANIFEST.MF", "Main-Class: com.example.Viewer\n")
	natives := makeJar(t, "libkvm.so", "x")
	var jarGets, jarNotModified atomic.Int32
	srv, fp := fakeBMC(t, nil)
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("GET /Java/jviewer.jnlp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-java-jnlp-file")
		fmt.Fprintf(w, `<jnlp codebase="https://%s/Java"><information><title>JViewer</title></information>
<resources><jar href="release/JViewer.jar"/><property name="probe" value="1"/></resources>
<resources><nativelib href="release/natives.jar"/></resources>
<application-desc><argument>-hostname</argument><argument>%s</argument></application-desc></jnlp>`,
			r.Host, r.URL.Query().Get("EXTRNIP"))
	})
	mux.HandleFunc("GET /Java/release/{name}", func(w http.ResponseWriter, r *http.Request) {
		if c, _ := r.Cookie("SID"); c == nil {
			http.Error(w, "login first", http.StatusForbidden)
			return
		}
		if r.PathValue("name") == "JViewer.jar" {
			jarGets.Add(1)
			if r.Header.Get("If-None-Match") == `"v1"` {
				jarNotModified.Add(1)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			w.Write(jar)
			return
		}
		w.Write(natives)
	})

	p, ev := newTestProxy(t, srv.URL+"/", fp)
	javaHome := t.TempDir()
	out := filepath.Join(t.TempDir(), "args.txt")
	os.MkdirAll(filepath.Join(javaHome, "bin"), 0o700)
	os.WriteFile(filepath.Join(javaHome, "bin", "java"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+out+"\"\n"), 0o700)
	t.Setenv(javaHomeEnv, javaHome)
	p.java = newJavaLauncher(t.TempDir(), &http.Client{Transport: p.rp.Transport}, ev.add)

	c := browserFor(t, p)
	get(t, c, p.startURL(srv.URL+"/"))
	// The BMC's own session cookie, as after a login.
	u, _ := url.Parse(p.origin())
	c.Jar.SetCookies(u, []*http.Cookie{{Name: "SID", Value: "s3cret", Path: "/"}})

	launch := func(dest string) *http.Response {
		res, body := get(t, c, p.origin()+"/Java/jviewer.jnlp?EXTRNIP=127.0.0.1&JNLPSTR=JViewer", "Sec-Fetch-Dest", dest)
		if dest == "document" && (res.StatusCode != http.StatusOK || !strings.Contains(body, "正在启动 Java 远程控制台")) {
			t.Fatalf("JNLP page response: %d\n%s", res.StatusCode, body)
		}
		var st javaStatus
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			_, s := get(t, c, p.origin()+javaStatusPath)
			_ = json.Unmarshal([]byte(s), &st)
			if st.State == "exited" || st.State == "failed" {
				break
			}
		}
		if st.State != "exited" {
			t.Fatalf("java status %+v; events:\n%s", st, ev)
		}
		return res
	}

	launch("document")
	args, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(args)), "\n")
	got := strings.Join(lines, " ")
	for _, want := range []string{"-Djava.security.properties=", "-Djava.library.path=", "-Dprobe=1", "JViewer.jar com.example.Viewer -hostname 127.0.0.1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("java args %q lack %q", got, want)
		}
	}
	libDir := strings.TrimPrefix(lines[1], "-Djava.library.path=")
	if _, err := os.Stat(filepath.Join(libDir, "libkvm.so")); err != nil {
		t.Fatalf("native library not extracted: %v", err)
	}

	// A script fetching the JNLP gets no download; the cached jar is reused.
	if res := launch(""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("script fetch: %d", res.StatusCode)
	}
	if jarGets.Load() != 2 || jarNotModified.Load() != 1 {
		t.Fatalf("jar requests %d, not modified %d: want a revalidated cache", jarGets.Load(), jarNotModified.Load())
	}
}

func TestJavaMissingRuntime(t *testing.T) {
	t.Setenv(javaHomeEnv, t.TempDir())
	l := newJavaLauncher(t.TempDir(), http.DefaultClient, nil)
	src, _ := url.Parse("https://10.0.0.5/x.jnlp")
	plan, _ := parseJNLP([]byte(inspurJNLP), src, platformFor("windows", "amd64"))
	l.Launch(t.Context(), plan, "")
	if st := l.Status(); st.State != "failed" || !strings.Contains(st.Detail, "Java 运行环境") {
		t.Fatalf("status %+v", st)
	}
}

// A BMC that drops the first request for the Java console file (session
// still being set up) gets asked again; one that keeps dropping it gets an
// explanatory page.
func TestProxyRetriesDroppedJNLP(t *testing.T) {
	old := jnlpRetryDelays
	jnlpRetryDelays = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}
	t.Cleanup(func() { jnlpRetryDelays = old })

	var calls, failFirst atomic.Int32
	srv, fp := fakeBMC(t, nil)
	srv.Config.Handler.(*http.ServeMux).HandleFunc("GET /Java/jviewer.jnlp", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= failFirst.Load() {
			conn, _, _ := http.NewResponseController(w).Hijack()
			conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/x-java-jnlp-file")
		fmt.Fprint(w, "<jnlp/>")
	})
	p, _ := newTestProxy(t, srv.URL+"/", fp)
	c := browserFor(t, p)
	get(t, c, p.startURL(srv.URL+"/"))

	failFirst.Store(1)
	if res, body := get(t, c, p.origin()+"/Java/jviewer.jnlp?JNLPSTR=JViewer"); res.StatusCode != http.StatusOK || body != "<jnlp/>" || calls.Load() != 2 {
		t.Fatalf("after one dropped request: %d %q, %d calls", res.StatusCode, body, calls.Load())
	}
	calls.Store(0)
	failFirst.Store(10)
	res, body := get(t, c, p.origin()+"/Java/jviewer.jnlp?JNLPSTR=JViewer")
	// Three attempts here; Go's transport may add one more on a reused connection.
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(body, "BMC 没有返回 Java 远程控制台") || calls.Load() < 3 {
		t.Fatalf("always dropped: %d, %d calls\n%s", res.StatusCode, calls.Load(), body)
	}
}

// Some AMI MegaRAC firmware (Inspur) announces a longer Content-Length for
// the Java console file than it sends, or cuts the body off, and closes the
// connection. A file that arrived whole is used; a cut-off one is asked for
// again.
func TestProxyHandlesTruncatedJNLP(t *testing.T) {
	old := jnlpRetryDelays
	jnlpRetryDelays = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}
	t.Cleanup(func() { jnlpRetryDelays = old })
	t.Setenv(javaHomeEnv, t.TempDir()) // no java: the launch fails after the page is served

	var calls, cutFirst atomic.Int32
	srv, fp := fakeBMC(t, nil)
	srv.Config.Handler.(*http.ServeMux).HandleFunc("GET /Java/jviewer.jnlp", func(w http.ResponseWriter, r *http.Request) {
		full := strings.Replace(inspurJNLP, "172.16.1.200:443", r.Host, 1)
		body := full
		if calls.Add(1) <= cutFirst.Load() {
			body = body[:len(body)/2]
		}
		conn, rw, _ := http.NewResponseController(w).Hijack()
		fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Type: application/x-java-jnlp-file\r\nContent-Length: %d\r\n\r\n%s", len(full)+64, body)
		rw.Flush()
		conn.Close()
	})
	p, ev := newTestProxy(t, srv.URL+"/", fp)
	p.java = newJavaLauncher(t.TempDir(), &http.Client{Transport: p.rp.Transport}, ev.add)
	c := browserFor(t, p)
	get(t, c, p.startURL(srv.URL+"/"))
	open := func() (*http.Response, string) {
		return get(t, c, p.origin()+"/Java/jviewer.jnlp?JNLPSTR=JViewer", "Sec-Fetch-Dest", "document")
	}

	// The whole file arrived: used as is.
	if res, body := open(); res.StatusCode != http.StatusOK || !strings.Contains(body, "正在启动 Java 远程控制台") || calls.Load() != 1 {
		t.Fatalf("complete file, short of its Content-Length: %d, %d calls\n%s\nevents:\n%s", res.StatusCode, calls.Load(), body, ev)
	}

	// Cut off once: asked for again.
	calls.Store(0)
	cutFirst.Store(1)
	if res, body := open(); res.StatusCode != http.StatusOK || !strings.Contains(body, "正在启动 Java 远程控制台") || calls.Load() != 2 {
		t.Fatalf("cut off once: %d, %d calls\n%s", res.StatusCode, calls.Load(), body)
	}

	// Always cut off: explained.
	calls.Store(0)
	cutFirst.Store(10)
	if res, body := open(); res.StatusCode != http.StatusBadGateway || !strings.Contains(body, "BMC 没有返回 Java 远程控制台") || calls.Load() != 3 {
		t.Fatalf("always cut off: %d, %d calls\n%s", res.StatusCode, calls.Load(), body)
	}
}
