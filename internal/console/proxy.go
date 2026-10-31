package console

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/12312bg/mdbmc/internal/bmc"
)

// proxy is the local HTTPS endpoint a console window's browser talks to.
//
// Chromium refuses much of what BMC web servers still use: TLS 1.0/1.1,
// 3DES, certificates it cannot parse or will not let a user override. The
// IPMI/Redfish clients already reach these BMCs through Go's TLS stack, so
// the console window does too: the browser only connects to this proxy on
// a loopback address (with a throwaway certificate the window pins), and the
// proxy forwards each request to the BMC through bmc.PinnedTransport, which
// accepts nothing but the BMC's pinned certificate.
//
// The proxy listens on 127.0.0.1 with a random high port: security software
// tends to inspect or block TLS on well-known ports such as 443, even on
// loopback. Requests must carry a session cookie that only the window's
// start URL sets, so other local programs cannot use the proxy.
//
// When the BMC answers with a JNLP file (a Java remote console), the proxy
// starts it with java (see jnlp.go) instead of handing the download to the
// browser.
type proxy struct {
	target *url.URL // https://bmc[:port]
	ln     net.Listener
	srv    *http.Server
	rp     *httputil.ReverseProxy
	host   string // location.host of the proxied pages
	token  string
	cookie string
	// certFP is the fingerprint of the proxy's own certificate, which the
	// window accepts.
	certFP string
	report func(name, detail string)
	// java starts Java consoles; nil leaves JNLP files to the browser.
	java *javaLauncher

	mu       sync.Mutex
	reported map[string]time.Time
}

const (
	startPath = "/__mdbmc/start"
	// javaStatusPath reports the Java console's progress to its status page.
	javaStatusPath = "/__mdbmc/java"
	// cookiePrefix marks the proxy's own cookies, which are not forwarded.
	cookiePrefix = "__Host-mdbmc-"
)

// startProxy serves a proxy to the BMC whose web UI is at bmcURL and whose
// certificate has the given fingerprint. report receives diagnostics.
func startProxy(bmcURL, fingerprint string, report func(name, detail string)) (*proxy, error) {
	u, err := url.Parse(bmcURL)
	if err != nil || u.Host == "" || u.Scheme != "https" {
		return nil, errors.New("无效的控制台地址")
	}
	if fingerprint == "" {
		return nil, errors.New("缺少 BMC 证书指纹")
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("无法监听本地端口: %w", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	cert, certFP, err := selfSignedCert(addr.IP)
	if err != nil {
		ln.Close()
		return nil, err
	}
	token := make([]byte, 16)
	_, _ = rand.Read(token)
	p := &proxy{
		target:   &url.URL{Scheme: "https", Host: u.Host},
		ln:       ln,
		host:     hostPort(addr.IP.String(), addr.Port, 443),
		token:    hex.EncodeToString(token),
		cookie:   cookiePrefix + strconv.Itoa(addr.Port),
		certFP:   certFP,
		report:   report,
		reported: map[string]time.Time{},
	}
	p.rp = &httputil.ReverseProxy{
		Rewrite:        p.rewrite,
		Transport:      retryJNLP{base: bmc.PinnedTransport(fingerprint), report: p.reportOnce},
		ModifyResponse: p.modifyResponse,
		ErrorHandler:   p.upstreamError,
		// Upstream failures after the response started (body copy errors).
		ErrorLog: log.New(reportWriter{p, "proxy-log"}, "", 0),
	}
	p.srv = &http.Server{
		Handler:           p,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// BMC web servers speak HTTP/1.1, and WebSockets (HTML5 KVM) are
		// only proxied over HTTP/1.1.
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"http/1.1"},
		},
		// Failures between the browser and the proxy, e.g. TLS handshakes.
		ErrorLog: log.New(reportWriter{p, "proxy-server"}, "", 0),
	}
	go func() { _ = p.srv.ServeTLS(ln, "", "") }()
	return p, nil
}

func (p *proxy) Close() error { return p.srv.Close() }

func (p *proxy) origin() string { return "https://" + p.host }

func selfSignedCert(ip net.IP) (tls.Certificate, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "MDBMC console proxy"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{ip},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, bmc.Fingerprint(der), nil
}

func portOf(u *url.URL) int {
	if n, err := strconv.Atoi(u.Port()); err == nil {
		return n
	}
	if u.Scheme == "http" || u.Scheme == "ws" {
		return 80
	}
	return 443
}

func hostPort(host string, port, defaultPort int) string {
	if port == defaultPort {
		if strings.Contains(host, ":") {
			return "[" + host + "]"
		}
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// isBMC reports whether an absolute URL points at the BMC's web server.
func (p *proxy) isBMC(u *url.URL) bool {
	if !strings.EqualFold(u.Hostname(), p.target.Hostname()) {
		return false
	}
	switch u.Scheme {
	case "https", "wss":
		return portOf(u) == portOf(p.target)
	case "http", "ws":
		return true // plain-HTTP links and redirects to the same BMC
	}
	return false
}

// localURL maps a BMC URL to the same page through the proxy. Other URLs,
// and relative ones, are returned unchanged.
func (p *proxy) localURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || !p.isBMC(u) {
		return raw
	}
	scheme := "https"
	if u.Scheme == "ws" || u.Scheme == "wss" {
		scheme = "wss"
	}
	out := *u
	out.Scheme, out.Host = scheme, p.host
	return out.String()
}

// startURL is the first URL a window opens: it sets the session cookie and
// redirects to target, a BMC URL.
func (p *proxy) startURL(target string) string {
	next := "/"
	if u, err := url.Parse(p.localURL(target)); err == nil && u.Host == p.host {
		next = u.RequestURI()
	}
	return p.origin() + startPath + "?" + url.Values{"t": {p.token}, "next": {next}}.Encode()
}

// toBMC maps an Origin or Referer value of a proxied page to the BMC, which
// may check them against its own address.
func (p *proxy) toBMC(v string) string {
	o := p.origin()
	if v == o || strings.HasPrefix(v, o+"/") {
		return p.target.Scheme + "://" + p.target.Host + v[len(o):]
	}
	return v
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == startPath {
		p.start(w, r)
		return
	}
	c, err := r.Cookie(p.cookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(p.token)) != 1 {
		writePage(w, http.StatusForbidden, page{
			Tone:    "error",
			Title:   "此地址仅供控制台窗口使用",
			Message: "请在服务器资源管理系统的设备列表中重新打开 Web 管理或远程控制台。",
		})
		return
	}
	if r.URL.Path == javaStatusPath && p.java != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(p.java.Status())
		return
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest == "document" || dest == "iframe" || dest == "frame" {
		// Pages (not their resources) are logged to follow what the window does.
		rec := &statusRecorder{ResponseWriter: w}
		begin := time.Now()
		p.rp.ServeHTTP(rec, r)
		p.event("proxy-request", fmt.Sprintf("%s %s -> %d (%d ms)", r.Method, r.URL.Path, rec.status, time.Since(begin).Milliseconds()))
		return
	}
	p.rp.ServeHTTP(w, r)
}

// statusRecorder remembers the status code of a response.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush and Hijack.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// reportWriter turns log output into diagnostic events.
type reportWriter struct {
	p    *proxy
	name string
}

func (w reportWriter) Write(b []byte) (int, error) {
	w.p.reportOnce(w.name, strings.TrimSpace(string(b)))
	return len(b), nil
}

func (p *proxy) start(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(q.Get("t")), []byte(p.token)) != 1 {
		writePage(w, http.StatusForbidden, page{
			Tone:    "error",
			Title:   "控制台链接已失效",
			Message: "请在服务器资源管理系统的设备列表中重新打开 Web 管理或远程控制台。",
		})
		return
	}
	next := q.Get("next")
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		next = "/"
	}
	http.SetCookie(w, &http.Cookie{
		Name: p.cookie, Value: p.token, Path: "/",
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, next, http.StatusFound)
}

func (p *proxy) rewrite(pr *httputil.ProxyRequest) {
	pr.SetURL(p.target)
	h := pr.Out.Header
	for _, k := range []string{"Origin", "Referer"} {
		if v := h.Get(k); v != "" {
			h.Set(k, p.toBMC(v))
		}
	}
	// Forward the BMC's cookies, not the proxy's session cookie.
	if cookies := h.Values("Cookie"); len(cookies) > 0 {
		var keep []string
		for _, line := range cookies {
			for part := range strings.SplitSeq(line, ";") {
				if part = strings.TrimSpace(part); part != "" && !strings.HasPrefix(part, cookiePrefix) {
					keep = append(keep, part)
				}
			}
		}
		h.Del("Cookie")
		if len(keep) > 0 {
			h.Set("Cookie", strings.Join(keep, "; "))
		}
	}
}

func (p *proxy) modifyResponse(res *http.Response) error {
	h := res.Header
	for _, k := range []string{"Location", "Content-Location"} {
		if v := h.Get(k); v != "" {
			h.Set(k, p.localURL(v))
		}
	}
	if v := h.Get("Access-Control-Allow-Origin"); v != "" && v != "*" {
		if u, err := url.Parse(v); err == nil && p.isBMC(u) {
			h.Set("Access-Control-Allow-Origin", p.origin())
		}
	}
	// A Domain attribute naming the BMC would make the browser drop the
	// cookie on the proxy's address.
	if cookies := h.Values("Set-Cookie"); len(cookies) > 0 {
		h.Del("Set-Cookie")
		for _, c := range cookies {
			h.Add("Set-Cookie", dropCookieDomain(c))
		}
	}
	if p.java != nil && res.StatusCode == http.StatusOK && looksLikeJNLP(h.Get("Content-Type"), res.Request.URL.Path) {
		return p.startJava(res)
	}
	return nil
}

// startJava starts the Java console described by a JNLP response and
// replaces the download with a status page (or nothing, when the page
// fetched the JNLP from a script).
func (p *proxy) startJava(res *http.Response) error {
	body := res.Body
	defer body.Close()
	var r io.Reader = io.LimitReader(body, 1<<20)
	if strings.EqualFold(res.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		r = gz
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	plan, err := parseJNLP(data, res.Request.URL, platformFor(runtime.GOOS, runtime.GOARCH))
	if err == nil {
		err = plan.mapToBMC(p.host, p.target)
	}
	pg := page{
		Tone:    "java",
		Title:   "正在启动 Java 远程控制台",
		Message: "远程控制台会在单独的 Java 窗口中打开。首次打开要从 BMC 下载程序，可能需要十几秒到一分钟。关闭本窗口会同时关闭 Java 远程控制台。",
		Target:  p.target.Host,
	}
	if err != nil {
		p.java.setStatus("failed", err.Error())
		pg = page{Tone: "error", Title: "无法启动 Java 远程控制台", Message: err.Error(), Target: p.target.Host}
	} else {
		cookie := res.Request.Header.Get("Cookie")
		p.java.setStatus("preparing", "")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), launchTimeout)
			defer cancel()
			p.java.Launch(ctx, plan, cookie)
		}()
	}
	h := http.Header{"Cache-Control": {"no-store"}}
	if c := res.Header.Values("Set-Cookie"); len(c) > 0 {
		h["Set-Cookie"] = c
	}
	res.Header = h
	if dest := res.Request.Header.Get("Sec-Fetch-Dest"); dest != "document" && dest != "iframe" && dest != "frame" {
		res.StatusCode, res.Status = http.StatusNoContent, "204 No Content"
		res.Body, res.ContentLength = http.NoBody, 0
		return nil
	}
	html := pg.render()
	h.Set("Content-Type", "text/html; charset=utf-8")
	res.Body, res.ContentLength = io.NopCloser(strings.NewReader(html)), int64(len(html))
	return nil
}

func dropCookieDomain(c string) string {
	parts := strings.Split(c, ";")
	out := parts[:1]
	for _, a := range parts[1:] {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(a)), "domain=") {
			out = append(out, a)
		}
	}
	return strings.Join(out, ";")
}

// retryJNLP retries a Java console file (JNLP) the BMC dropped: some
// firmware closes the connection when the file is asked for while the web
// session is still being set up right after the login, or cuts the file
// off. Fetching it is a GET, so asking again is safe.
type retryJNLP struct {
	base   http.RoundTripper
	report func(name, detail string)
}

// jnlpRetryDelays are the waits before each retry.
var jnlpRetryDelays = []time.Duration{1500 * time.Millisecond, 3 * time.Second}

// maxJNLPSize bounds a Java console file.
const maxJNLPSize = 1 << 20

func (t retryJNLP) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || !strings.HasSuffix(strings.ToLower(req.URL.Path), ".jnlp") {
		return t.base.RoundTrip(req)
	}
	for attempt := 0; ; attempt++ {
		res, err := t.base.RoundTrip(req)
		if err == nil {
			if err = t.readJNLP(res); err == nil {
				return res, nil
			}
		}
		if attempt == len(jnlpRetryDelays) || !droppedConnection(err) {
			return nil, err
		}
		select {
		case <-time.After(jnlpRetryDelays[attempt]):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

// readJNLP reads the body of a successful response into memory. AMI
// MegaRAC firmware (Inspur and others) can announce a longer Content-Length
// than it sends and close the connection; a file that nevertheless ends
// with </jnlp> is complete and used, anything else that was cut off is an
// error to retry.
func (t retryJNLP) readJNLP(res *http.Response) error {
	if res.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxJNLPSize+1))
	res.Body.Close()
	switch {
	case err == nil:
	case droppedConnection(err) && isPlainEncoding(res.Header.Get("Content-Encoding")) &&
		bytes.Contains(bytes.ToLower(data), []byte("</jnlp>")):
		if t.report != nil {
			t.report("proxy-log", fmt.Sprintf("%s: BMC closed the connection after %d of %d bytes; the file is complete, using it",
				res.Request.URL.Path, len(data), res.ContentLength))
		}
	default:
		return err
	}
	if len(data) > maxJNLPSize {
		return errors.New("Java 控制台启动文件过大")
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	res.ContentLength = int64(len(data))
	res.TransferEncoding = nil
	res.Header.Set("Content-Length", strconv.Itoa(len(data)))
	return nil
}

func isPlainEncoding(enc string) bool {
	return enc == "" || strings.EqualFold(enc, "identity")
}

// droppedConnection reports a connection the server closed or reset
// without an answer.
func droppedConnection(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "EOF") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "forcibly closed")
}

func (p *proxy) upstreamError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		return // the page went away
	}
	pg := describeUpstreamError(err)
	if strings.HasSuffix(strings.ToLower(r.URL.Path), ".jnlp") && droppedConnection(err) {
		pg = page{
			Tone:    "error",
			Title:   "BMC 没有返回 Java 远程控制台",
			Message: "BMC 在生成 Java 控制台启动文件时断开了连接（已自动重试）。",
			Hints: []string{
				"点“重新连接”再试一次；BMC 刚登录时偶尔会这样。",
				"也可以打开该设备的 Web 管理，登录后点击 BMC 页面里的 Java 远程控制台按钮，同样会直接打开。",
			},
		}
	}
	pg.Target = p.target.Host
	pg.Retry = p.startURL(p.target.String() + r.URL.RequestURI())
	pg.Detail = fmt.Sprintf("%s %s\n%v", r.Method, r.URL.RequestURI(), err)
	p.reportOnce("proxy-error", r.URL.Path+": "+err.Error())
	writePage(w, http.StatusBadGateway, pg)
}

func (p *proxy) event(name, detail string) {
	if p.report != nil {
		p.report(name, detail)
	}
}

// reportOnce passes a diagnostic on, but not the same one again within a few
// seconds (a page that fails loads many resources).
func (p *proxy) reportOnce(name, detail string) {
	if p.report == nil {
		return
	}
	key := name + detail
	p.mu.Lock()
	last, seen := p.reported[key]
	now := time.Now()
	if !seen || now.Sub(last) > 10*time.Second {
		p.reported[key] = now
	}
	p.mu.Unlock()
	if !seen || now.Sub(last) > 10*time.Second {
		p.report(name, detail)
	}
}

func describeUpstreamError(err error) page {
	var mm *bmc.FingerprintMismatchError
	var netErr net.Error
	var alert tls.AlertError
	var recErr tls.RecordHeaderError
	switch {
	case errors.As(err, &mm):
		return page{
			Tone:    "warning",
			Title:   "BMC 证书与已信任的不一致",
			Message: "BMC 当前出示的证书与之前固定的证书不同，为防止中间人攻击，已拒绝连接。",
			Hints: []string{
				"如果确认 BMC 更换过证书（例如更新固件、重置 BMC 或上传了新证书），请在设备详情中点击“信任当前证书”，然后重新打开。",
				"如果没有人更换过证书，请先排查网络中是否有设备冒用该 BMC 的地址。",
			},
		}
	case errors.As(err, &netErr) && netErr.Timeout():
		return page{
			Tone:    "error",
			Title:   "连接 BMC 超时",
			Message: "BMC 在规定时间内没有响应。",
			Hints: []string{
				"确认本机能访问 BMC 管理网络，且 BMC 的 HTTPS 端口没有被防火墙拦截。",
				"BMC 的 Web 服务偶尔会卡住，可稍后重试；长时间无响应时需要重启 BMC。",
			},
		}
	case errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "actively refused"):
		return page{
			Tone:    "error",
			Title:   "BMC 拒绝了连接",
			Message: "BMC 没有在该端口上提供 HTTPS 服务。",
			Hints:   []string{"检查设备的 HTTPS 端口设置，以及 BMC 是否禁用了 Web 服务。"},
		}
	case errors.As(err, &alert), errors.As(err, &recErr):
		return page{
			Tone:    "error",
			Title:   "与 BMC 建立加密连接失败",
			Message: "BMC 不接受本软件支持的任何 TLS 版本或加密套件。",
			Hints:   []string{"升级 BMC 固件通常可以解决。"},
		}
	}
	return page{
		Tone:    "error",
		Title:   "无法打开 BMC 管理页面",
		Message: "连接 BMC 的 Web 服务时出错。",
		Hints:   []string{"确认 BMC 在线且本机能访问其管理网络，然后重新连接。"},
	}
}

func writePage(w http.ResponseWriter, status int, pg page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, pg.render())
}
