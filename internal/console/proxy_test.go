package console

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/12312bg/mdbmc/internal/bmc"
)

// fakeBMC is a BMC web server: a login that sets a cookie with a Domain and
// redirects to an absolute URL, an endpoint that echoes what it received,
// and a WebSocket-style upgrade that echoes bytes.
func fakeBMC(t *testing.T, cfg *tls.Config) (*httptest.Server, string) {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "login page")
	})
	mux.HandleFunc("POST /cgi/login.cgi", func(w http.ResponseWriter, r *http.Request) {
		host := strings.Split(r.Host, ":")[0]
		w.Header().Add("Set-Cookie", "SID=s3cret; Domain="+host+"; Path=/; Secure; HttpOnly")
		http.Redirect(w, r, srv.URL+"/main?x=1", http.StatusFound)
	})
	mux.HandleFunc("GET /echo", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "host=%s\norigin=%s\nreferer=%s\ncookie=%s\n",
			r.Host, r.Header.Get("Origin"), r.Header.Get("Referer"), r.Header.Get("Cookie"))
	})
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "upgrade required", http.StatusBadRequest)
			return
		}
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nX-Origin: %s\r\n\r\n", r.Header.Get("Origin"))
		rw.Flush()
		line, _ := rw.ReadString('\n')
		fmt.Fprint(rw, "echo:"+line)
		rw.Flush()
	})
	srv = httptest.NewUnstartedServer(mux)
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, bmc.Fingerprint(srv.Certificate().Raw)
}

type events struct {
	mu   sync.Mutex
	list []string
}

func (e *events) add(name, detail string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, name+" "+detail)
}

func (e *events) String() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.list, "\n")
}

func newTestProxy(t *testing.T, bmcURL, fingerprint string) (*proxy, *events) {
	t.Helper()
	ev := &events{}
	p, err := startProxy(bmcURL, fingerprint, ev.add)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p, ev
}

// browser is an HTTP client that, like the console window, accepts only the
// proxy's certificate and keeps cookies.
func browserFor(t *testing.T, p *proxy) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar: jar,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if bmc.Fingerprint(cs.PeerCertificates[0].Raw) != p.certFP {
					return fmt.Errorf("not the proxy's certificate")
				}
				return nil
			},
		}},
	}
}

func get(t *testing.T, c *http.Client, u string, header ...string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestProxyForwardsAndRewrites(t *testing.T) {
	srv, fp := fakeBMC(t, nil)
	p, _ := newTestProxy(t, srv.URL+"/", fp)
	c := browserFor(t, p)

	if !strings.HasPrefix(p.host, "127.") {
		t.Fatalf("proxy should listen on loopback, got %s", p.host)
	}
	res, body := get(t, c, p.startURL(srv.URL+"/"))
	if res.StatusCode != http.StatusOK || body != "login page" {
		t.Fatalf("start: %d %q", res.StatusCode, body)
	}

	// The login redirect names the BMC; it must come back through the proxy,
	// with the BMC's cookie usable on the proxy's address.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := c.PostForm(p.origin()+"/cgi/login.cgi", url.Values{"name": {"ADMIN"}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if loc := res.Header.Get("Location"); loc != p.origin()+"/main?x=1" {
		t.Fatalf("Location not mapped to the proxy: %q", loc)
	}
	if sc := res.Header.Get("Set-Cookie"); strings.Contains(strings.ToLower(sc), "domain=") || !strings.HasPrefix(sc, "SID=s3cret") {
		t.Fatalf("Set-Cookie not rewritten: %q", sc)
	}

	_, body = get(t, c, p.origin()+"/echo", "Origin", p.origin(), "Referer", p.origin()+"/main?x=1")
	bmcOrigin := "https://" + srv.Listener.Addr().String()
	for _, want := range []string{
		"host=" + srv.Listener.Addr().String() + "\n",
		"origin=" + bmcOrigin + "\n",
		"referer=" + bmcOrigin + "/main?x=1\n",
		"cookie=SID=s3cret\n", // the proxy's own session cookie is not forwarded
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("BMC saw\n%s\nwant %q", body, want)
		}
	}
}

func TestProxyRequiresSessionCookie(t *testing.T) {
	srv, fp := fakeBMC(t, nil)
	p, _ := newTestProxy(t, srv.URL+"/", fp)
	c := browserFor(t, p)

	if res, _ := get(t, c, p.origin()+"/echo"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("request without the session cookie: %d", res.StatusCode)
	}
	bad := strings.Replace(p.startURL(srv.URL+"/"), "t="+p.token, "t=00", 1)
	if res, _ := get(t, c, bad); res.StatusCode != http.StatusForbidden {
		t.Fatalf("start with a wrong token: %d", res.StatusCode)
	}
	// The start page only redirects within the proxy.
	evil := p.origin() + startPath + "?" + url.Values{"t": {p.token}, "next": {"//evil.example/"}}.Encode()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if res, _ := get(t, c, evil); res.Header.Get("Location") != "/" {
		t.Fatalf("open redirect: %q", res.Header.Get("Location"))
	}
}

func TestProxyLegacyTLS(t *testing.T) {
	// A BMC that only speaks TLS 1.0 with RSA key exchange, which Chromium
	// refuses but the proxy (Go's TLS client) still accepts.
	srv, fp := fakeBMC(t, &tls.Config{
		MinVersion:   tls.VersionTLS10,
		MaxVersion:   tls.VersionTLS10,
		CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA},
	})
	p, _ := newTestProxy(t, srv.URL+"/", fp)
	c := browserFor(t, p)
	res, body := get(t, c, p.startURL(srv.URL+"/"))
	if res.StatusCode != http.StatusOK || body != "login page" {
		t.Fatalf("legacy BMC through proxy: %d %q", res.StatusCode, body)
	}
}

func TestProxyRejectsOtherCertificate(t *testing.T) {
	srv, _ := fakeBMC(t, nil)
	p, ev := newTestProxy(t, srv.URL+"/", strings.Repeat("ab", 32))
	c := browserFor(t, p)
	res, body := get(t, c, p.startURL(srv.URL+"/"))
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(body, "BMC 证书与已信任的不一致") {
		t.Fatalf("unpinned BMC certificate: %d\n%s", res.StatusCode, body)
	}
	if !strings.Contains(body, "重新连接") || !strings.Contains(ev.String(), "proxy-error") {
		t.Fatalf("error page or event missing; events:\n%s", ev)
	}
}

func TestProxyUnreachableBMC(t *testing.T) {
	srv, fp := fakeBMC(t, nil)
	addr := srv.URL
	srv.Close()
	p, ev := newTestProxy(t, addr+"/", fp)
	res, body := get(t, browserFor(t, p), p.startURL(addr+"/"))
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(body, "BMC 拒绝了连接") {
		t.Fatalf("closed BMC port: %d\n%s", res.StatusCode, body)
	}
	if !strings.Contains(ev.String(), "proxy-error /") {
		t.Fatalf("no event: %s", ev)
	}
}

func TestProxyWebSocket(t *testing.T) {
	srv, fp := fakeBMC(t, nil)
	p, _ := newTestProxy(t, srv.URL+"/", fp)
	c := browserFor(t, p)
	get(t, c, p.startURL(srv.URL+"/"))
	u, _ := url.Parse(p.origin())
	var cookie string
	for _, ck := range c.Jar.Cookies(u) {
		cookie = ck.Name + "=" + ck.Value
	}

	conn, err := tls.Dial("tcp", p.ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nOrigin: %s\r\nCookie: %s\r\n\r\n", p.host, p.origin(), cookie)
	r := bufio.NewReader(conn)
	res, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade: %s", res.Status)
	}
	if got := res.Header.Get("X-Origin"); got != "https://"+srv.Listener.Addr().String() {
		t.Fatalf("BMC saw WebSocket Origin %q", got)
	}
	fmt.Fprint(conn, "frame\n")
	if line, _ := r.ReadString('\n'); line != "echo:frame\n" {
		t.Fatalf("echo %q", line)
	}
}

func TestProxyLocalURL(t *testing.T) {
	p := &proxy{target: &url.URL{Scheme: "https", Host: "10.0.0.5"}, host: "127.9.9.9"}
	for in, want := range map[string]string{
		"https://10.0.0.5/":                    "https://127.9.9.9/",
		"https://10.0.0.5:443/cgi/x.cgi?a=1#f": "https://127.9.9.9/cgi/x.cgi?a=1#f",
		"http://10.0.0.5/page":                 "https://127.9.9.9/page",
		"wss://10.0.0.5/kvm":                   "wss://127.9.9.9/kvm",
		"https://10.0.0.5:8443/":               "https://10.0.0.5:8443/",
		"https://10.0.0.50/":                   "https://10.0.0.50/",
		"/relative":                            "/relative",
		"":                                     "",
		"https://10.0.0.5/cgi/url_redirect.cgi?x=%2F": "https://127.9.9.9/cgi/url_redirect.cgi?x=%2F",
	} {
		if got := p.localURL(in); got != want {
			t.Errorf("localURL(%q) = %q, want %q", in, got, want)
		}
	}
	if got := p.startURL("https://10.0.0.5/cgi/a.cgi?b=1"); !strings.Contains(got, "next=%2Fcgi%2Fa.cgi%3Fb%3D1") {
		t.Errorf("startURL = %q", got)
	}
}

func TestDropCookieDomain(t *testing.T) {
	got := dropCookieDomain("SID=1; domain=10.0.0.5; Path=/; Secure")
	if got != "SID=1; Path=/; Secure" {
		t.Fatalf("got %q", got)
	}
}

func TestPageEscapes(t *testing.T) {
	html := page{Tone: "error", Title: "<b>", Detail: "</pre><script>", Retry: "javascript:alert(1)"}.render()
	if strings.Contains(html, "<b>") || strings.Contains(html, "</pre><script>") || strings.Contains(html, `href="javascript:`) {
		t.Fatalf("page does not escape its fields:\n%s", html)
	}
}

func TestOnlyLoadingPageOpensTheBMC(t *testing.T) {
	if !strings.Contains(page{Tone: "loading"}.render(), openMessage) {
		t.Fatal("the loading page must ask the window to open the BMC")
	}
	if strings.Contains(page{Tone: "error"}.render(), openMessage) {
		t.Fatal("an error page must not reopen the BMC by itself")
	}
}
