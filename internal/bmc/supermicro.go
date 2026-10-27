package bmc

import (
	"context"
	"errors"
	"strings"
)

// Supermicro specifics.
//
// X11 and newer firmware expose an OEM Redfish resource that returns a
// one-time URI for the HTML5 iKVM, which opens without a separate login.
// Older firmware falls back to logging into the web UI and opening the
// classic HTML5 iKVM page.
const supermicroLegacyKVMPath = "/cgi/url_redirect.cgi?url_name=man_ikvm_html5_bootstrap"

type supermicroIKVM struct {
	CurrentInterface string `json:"Current interface"`
	URI              string `json:"URI"`
}

func (c *Client) supermicroKVM(ctx context.Context, base string) (*LaunchSpec, error) {
	legacy := &LaunchSpec{URL: base + "/", AutoLogin: true, AfterLoginURL: base + supermicroLegacyKVMPath}
	if !c.allowed(ProtocolRedfish) {
		return legacy, nil
	}
	rf, err := c.redfish(ctx)
	if err != nil {
		var mm *FingerprintMismatchError
		if errors.As(err, &mm) {
			return nil, err
		}
		return legacy, nil
	}
	m, err := rf.firstManager()
	if err != nil {
		return legacy, nil
	}
	var ikvm supermicroIKVM
	if err := rf.getJSON(m.ODataID+"/Oem/Supermicro/IKVM", &ikvm); err != nil || ikvm.URI == "" {
		return legacy, nil
	}
	uri := ikvm.URI
	if !strings.HasPrefix(uri, "/") && !strings.HasPrefix(uri, "http") {
		uri = "/" + uri
	}
	if strings.HasPrefix(uri, "http") {
		return &LaunchSpec{URL: uri}, nil
	}
	return &LaunchSpec{URL: base + uri}, nil
}
