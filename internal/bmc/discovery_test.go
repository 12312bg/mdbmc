package bmc

import "testing"

func TestExpandTargets(t *testing.T) {
	cases := []struct {
		spec string
		n    int
		ok   bool
	}{
		{"192.168.1.0/24", 254, true},
		{"10.0.0.10-10.0.0.20", 11, true},
		{"10.0.0.10-20, 10.0.0.15", 11, true},
		{"10.0.0.1；10.0.0.2、10.0.0.3", 3, true},
		{"10.0.0.0/16", 0, false},
		{"10.0.0.20-10", 0, false},
		{"bmc01.lab, 10.0.0.1", 2, true},
		{"", 0, false},
	}
	for _, c := range cases {
		got, err := ExpandTargets(c.spec, 0)
		if (err == nil) != c.ok || (c.ok && len(got) != c.n) {
			t.Errorf("ExpandTargets(%q) = %d hosts, err %v; want %d ok=%v", c.spec, len(got), err, c.n, c.ok)
		}
	}
	got, _ := ExpandTargets("192.168.1.0/30", 0)
	if len(got) != 2 || got[0] != "192.168.1.1" || got[1] != "192.168.1.2" {
		t.Errorf("/30 = %v", got)
	}
}

func TestParseChannelAuthCaps(t *testing.T) {
	// Captured shape of a real IPMI 2.0 BMC response.
	resp := []byte{
		0x06, 0x00, 0xff, 0x07, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0x10,
		0x81, 0x1c, 0x63, 0x20, 0x00, 0x38, 0x00, 0x01, 0x97, 0x04, 0x03, 0x00, 0x00, 0x00, 0x00, 0x0e,
	}
	v, ok := parseChannelAuthCaps(resp)
	if !ok || v != "2.0" {
		t.Fatalf("got %q %v", v, ok)
	}
	if _, ok := parseChannelAuthCaps(resp[:10]); ok {
		t.Fatal("short packet accepted")
	}
}

func TestDetectVendor(t *testing.T) {
	cases := map[string]string{
		"Supermicro":                     VendorSupermicro,
		"Dell Inc.":                      VendorDell,
		"HPE":                            VendorHPE,
		"Hewlett Packard Enterprise":     VendorHPE,
		"Lenovo":                         VendorLenovo,
		"Huawei":                         VendorHuawei,
		"xFusion":                        VendorHuawei,
		"Inspur":                         VendorInspur,
		"New H3C Technologies Co., Ltd.": VendorH3C,
		"American Megatrends":            VendorGeneric,
		"Shpe Corp":                      VendorGeneric,
	}
	for name, want := range cases {
		if got := DetectVendor(0, name); got != want {
			t.Errorf("DetectVendor(%q) = %s, want %s", name, got, want)
		}
	}
	if got := DetectVendor(674); got != VendorDell {
		t.Errorf("IANA 674 = %s", got)
	}
	if got := DetectVendor(10876, "", "AMI"); got != VendorSupermicro {
		t.Errorf("IANA fallback = %s", got)
	}
}
