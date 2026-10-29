//go:build !windows

package update

import (
	"net/http"
	"net/url"
)

// systemProxy: other systems only use the proxy environment variables.
func systemProxy() func(*http.Request) (*url.URL, error) {
	return func(*http.Request) (*url.URL, error) { return nil, nil }
}
