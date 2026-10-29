//go:build windows

package update

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxy reads the proxy set in Windows' Internet Options (the same
// setting browsers use). Automatic configuration scripts (PAC) are not
// evaluated.
func systemProxy() func(*http.Request) (*url.URL, error) {
	none := func(*http.Request) (*url.URL, error) { return nil, nil }
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return none
	}
	defer k.Close()
	if on, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || on == 0 {
		return none
	}
	server, _, _ := k.GetStringValue("ProxyServer")
	override, _, _ := k.GetStringValue("ProxyOverride")
	proxies := parseProxyServer(server)
	if len(proxies) == 0 {
		return none
	}
	return func(r *http.Request) (*url.URL, error) {
		if bypassProxy(r.URL.Hostname(), override) {
			return nil, nil
		}
		if u, ok := proxies[r.URL.Scheme]; ok {
			return u, nil
		}
		return proxies["*"], nil
	}
}

// parseProxyServer reads "host:port" (all protocols) or
// "http=host:port;https=host:port" into scheme -> proxy, "*" for all.
func parseProxyServer(s string) map[string]*url.URL {
	out := map[string]*url.URL{}
	add := func(scheme, addr string) {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			return
		}
		if !strings.Contains(addr, "://") {
			addr = "http://" + addr
		}
		if u, err := url.Parse(addr); err == nil && u.Host != "" {
			out[scheme] = u
		}
	}
	if !strings.Contains(s, "=") {
		add("*", s)
		return out
	}
	for _, part := range strings.Split(s, ";") {
		scheme, addr, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if scheme = strings.ToLower(strings.TrimSpace(scheme)); scheme == "http" || scheme == "https" {
			add(scheme, addr)
		}
	}
	return out
}

// bypassProxy applies Internet Options' exception list ("*.corp;10.*;<local>").
func bypassProxy(host, list string) bool {
	host = strings.ToLower(host)
	for _, p := range strings.Split(list, ";") {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case p == "":
		case p == "<local>":
			if !strings.Contains(host, ".") {
				return true
			}
		default:
			if ok, _ := path.Match(p, host); ok {
				return true
			}
		}
	}
	return false
}
