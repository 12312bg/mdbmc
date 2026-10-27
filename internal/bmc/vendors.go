package bmc

import (
	"slices"
	"strings"
)

// Vendor ids used throughout the application.
const (
	VendorSupermicro = "supermicro"
	VendorDell       = "dell"
	VendorHPE        = "hpe"
	VendorLenovo     = "lenovo"
	VendorHuawei     = "huawei"
	VendorInspur     = "inspur"
	VendorH3C        = "h3c"
	VendorGeneric    = "generic"
)

type vendorDef struct {
	id   string
	name string
	// IANA enterprise numbers reported in IPMI Get Device ID.
	iana []uint32
	// keywords matched against manufacturer / Redfish vendor strings.
	keywords []string
	// exact tokens matched as whole words (for short names such as "HP").
	tokens []string
}

var vendorDefs = []vendorDef{
	{id: VendorSupermicro, name: "Supermicro", iana: []uint32{10876, 47488}, keywords: []string{"supermicro", "super micro", "smci"}},
	{id: VendorDell, name: "Dell", iana: []uint32{674}, keywords: []string{"dell"}},
	{id: VendorHPE, name: "HPE", iana: []uint32{11, 47196}, keywords: []string{"hewlett"}, tokens: []string{"hp", "hpe"}},
	{id: VendorLenovo, name: "Lenovo", iana: []uint32{19046}, keywords: []string{"lenovo"}},
	{id: VendorHuawei, name: "华为 / 超聚变", iana: []uint32{2011}, keywords: []string{"huawei", "xfusion"}},
	{id: VendorInspur, name: "浪潮", keywords: []string{"inspur", "ieit"}},
	{id: VendorH3C, name: "新华三", iana: []uint32{25506}, keywords: []string{"h3c"}},
}

// VendorName returns the display name of a vendor id.
func VendorName(id string) string {
	for _, v := range vendorDefs {
		if v.id == id {
			return v.name
		}
	}
	if id == "" {
		return ""
	}
	return "通用"
}

// VendorOption is a vendor id with its display name.
type VendorOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Vendors lists the known vendors for UI filters.
func Vendors() []VendorOption {
	out := make([]VendorOption, 0, len(vendorDefs)+1)
	for _, v := range vendorDefs {
		out = append(out, VendorOption{ID: v.id, Name: v.name})
	}
	return append(out, VendorOption{ID: VendorGeneric, Name: "通用"})
}

// DetectVendor guesses the vendor from free-form names (manufacturer,
// Redfish vendor, product, OEM keys) and the IPMI manufacturer id.
func DetectVendor(iana uint32, names ...string) string {
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		words := strings.FieldsFunc(n, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
		})
		for _, v := range vendorDefs {
			for _, k := range v.keywords {
				if strings.Contains(n, k) {
					return v.id
				}
			}
			for _, t := range v.tokens {
				if slices.Contains(words, t) {
					return v.id
				}
			}
		}
	}
	if iana != 0 {
		for _, v := range vendorDefs {
			if slices.Contains(v.iana, iana) {
				return v.id
			}
		}
	}
	return VendorGeneric
}
