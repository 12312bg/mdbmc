// Package bmc talks to baseboard management controllers over IPMI 2.0
// (RMCP+) and Redfish, hiding the protocol and vendor differences behind one
// Client. Standard protocols do the heavy lifting; vendor adapters only cover
// what the standards leave open (vendor detection, KVM launch).
package bmc

import "time"

// Protocol selects how a device is managed.
type Protocol string

const (
	ProtocolAuto    Protocol = "auto"
	ProtocolIPMI    Protocol = "ipmi"
	ProtocolRedfish Protocol = "redfish"
)

// Valid reports whether p is a known protocol mode.
func (p Protocol) Valid() bool {
	return p == ProtocolAuto || p == ProtocolIPMI || p == ProtocolRedfish
}

// Target is everything needed to reach one BMC.
type Target struct {
	Host      string
	IPMIPort  int
	HTTPSPort int
	Username  string
	Password  string
	Protocol  Protocol
	// HasIPMI / HasRedfish are the capabilities learned by Identify. When both
	// are false the device was never identified and both are tried.
	HasIPMI    bool
	HasRedfish bool
	// TLSFingerprint is the pinned SHA-256 of the BMC certificate (hex). An
	// empty value trusts the first certificate seen.
	TLSFingerprint string
	// Vendor is the detected vendor id (see vendors.go); optional.
	Vendor string

	IPMITimeout    time.Duration
	RedfishTimeout time.Duration
}

// PowerState is the host power state.
type PowerState string

const (
	PowerOn      PowerState = "on"
	PowerOff     PowerState = "off"
	PowerUnknown PowerState = "unknown"
)

// Health is a coarse health rollup.
type Health string

const (
	HealthOK       Health = "ok"
	HealthWarning  Health = "warning"
	HealthCritical Health = "critical"
	HealthUnknown  Health = "unknown"
)

// PowerAction is a chassis power operation.
type PowerAction string

const (
	ActionOn    PowerAction = "on"    // power up
	ActionOff   PowerAction = "off"   // immediate power off
	ActionSoft  PowerAction = "soft"  // graceful (ACPI) shutdown
	ActionReset PowerAction = "reset" // hard reset
	ActionCycle PowerAction = "cycle" // power off, then on
)

// Valid reports whether a is a known action.
func (a PowerAction) Valid() bool {
	switch a {
	case ActionOn, ActionOff, ActionSoft, ActionReset, ActionCycle:
		return true
	}
	return false
}

// Identity is what Identify learns about a BMC.
type Identity struct {
	Vendor         string     `json:"vendor"`
	VendorName     string     `json:"vendorName"`
	Manufacturer   string     `json:"manufacturer"`
	Model          string     `json:"model"`
	Serial         string     `json:"serial"`
	BMCFirmware    string     `json:"bmcFirmware"`
	BIOSVersion    string     `json:"biosVersion"`
	HasIPMI        bool       `json:"hasIpmi"`
	HasRedfish     bool       `json:"hasRedfish"`
	IPMIVersion    string     `json:"ipmiVersion"`
	RedfishVersion string     `json:"redfishVersion"`
	ManufacturerID uint32     `json:"manufacturerId"`
	PowerState     PowerState `json:"powerState"`
	Health         Health     `json:"health"`
	// TLSFingerprint is the certificate fingerprint observed during Identify.
	TLSFingerprint string   `json:"tlsFingerprint"`
	Warnings       []string `json:"warnings"`
}

// Status is the result of a lightweight poll.
type Status struct {
	Reachable bool       `json:"reachable"`
	Power     PowerState `json:"power"`
	Health    Health     `json:"health"`
	Via       Protocol   `json:"via"`
	Error     string     `json:"error"`
}

// Sensor is one normalized sensor reading.
type Sensor struct {
	Name     string   `json:"name"`
	Category string   `json:"category"` // temperature | fan | voltage | current | power | other
	Type     string   `json:"type"`     // protocol-specific type label
	Reading  *float64 `json:"reading"`
	Unit     string   `json:"unit"`
	Status   string   `json:"status"` // ok | warning | critical | unknown | absent
	Text     string   `json:"text"`   // discrete state description

	LowerCritical *float64 `json:"lowerCritical"`
	LowerWarning  *float64 `json:"lowerWarning"`
	UpperWarning  *float64 `json:"upperWarning"`
	UpperCritical *float64 `json:"upperCritical"`
	Source        Protocol `json:"source"`
}

// SELEntry is one normalized event log entry.
type SELEntry struct {
	ID         string   `json:"id"`
	Time       int64    `json:"time"` // unix seconds, 0 if unknown
	Sensor     string   `json:"sensor"`
	SensorType string   `json:"sensorType"`
	Message    string   `json:"message"`
	Severity   string   `json:"severity"`  // info | ok | warning | critical
	Direction  string   `json:"direction"` // assert | deassert | ""
	Source     Protocol `json:"source"`
}

// LaunchSpec tells the console window what to open.
type LaunchSpec struct {
	URL string `json:"url"`
	// AutoLogin fills Username/Password into the BMC login form.
	AutoLogin bool   `json:"autoLogin"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	// AfterLoginURL is opened once the login form has been submitted.
	AfterLoginURL string `json:"afterLoginUrl,omitempty"`
	// Hint is shown to the user when the vendor needs a manual step.
	Hint string `json:"hint,omitempty"`
}
