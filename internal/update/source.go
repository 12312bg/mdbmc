package update

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultSource is the GitHub Releases page of the release repository.
//
// A source is laid out like GitHub Releases: the newest manifest is at
// <source>/latest/download/latest.json and the files of a release at
// <source>/download/<tag>/<name>. A mirror of the GitHub URL or a web
// server on the local network with the same layout can stand in for it.
const DefaultSource = "https://github.com/12312bg/mdbmc-release/releases"

// NormalizeSource checks a configured source and trims the trailing slash;
// an empty value means DefaultSource.
func NormalizeSource(s string) (string, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("更新源需为 http:// 或 https:// 开头的地址，且不带查询参数")
	}
	return s, nil
}

// NormalizeProxy checks a configured proxy address; an empty value means
// the system proxy.
func NormalizeProxy(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
		return "", errors.New("代理地址格式应为 http://主机:端口 或 socks5://主机:端口")
	}
	return strings.TrimRight(s, "/"), nil
}

func sourceOrDefault(s string) string {
	if s == "" {
		return DefaultSource
	}
	return s
}

// latestURL is the address of a file in the newest release.
func latestURL(source, name string) string {
	return sourceOrDefault(source) + "/latest/download/" + name
}

func assetURL(source, tag, name string) string {
	return sourceOrDefault(source) + "/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
}

// ReleaseURL is the page of a release on the source (GitHub's release page
// for the default source).
func ReleaseURL(source, tag string) string {
	return sourceOrDefault(source) + "/tag/" + url.PathEscape(tag)
}

// newHTTPClient returns a client that uses proxy if set, and otherwise the
// HTTPS_PROXY / HTTP_PROXY variables or the system proxy settings.
func newHTTPClient(proxy string) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 30 * time.Second
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		tr.Proxy = http.ProxyURL(u)
	} else {
		sys := systemProxy()
		tr.Proxy = func(r *http.Request) (*url.URL, error) {
			if u, err := http.ProxyFromEnvironment(r); u != nil || err != nil {
				return u, err
			}
			return sys(r)
		}
	}
	return &http.Client{Transport: tr}, nil
}
