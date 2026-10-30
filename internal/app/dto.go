package app

import (
	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/store"
)

// UserInfo describes the logged-in user to the frontend.
type UserInfo struct {
	ID                 int64    `json:"id"`
	Username           string   `json:"username"`
	DisplayName        string   `json:"displayName"`
	Role               string   `json:"role"`
	RoleLabel          string   `json:"roleLabel"`
	Permissions        []string `json:"permissions"`
	MustChangePassword bool     `json:"mustChangePassword"`
}

func userInfo(u *store.User) *UserInfo {
	role := auth.Role(u.Role)
	perms := make([]string, 0)
	for _, p := range role.Permissions() {
		perms = append(perms, string(p))
	}
	return &UserInfo{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: u.Role, RoleLabel: role.Label(),
		Permissions: perms, MustChangePassword: u.MustChangePassword,
	}
}

// AuthStatus tells the frontend which screen to show.
type AuthStatus struct {
	Initialized bool      `json:"initialized"`
	User        *UserInfo `json:"user"`
	Version     string    `json:"version"`
	DataDir     string    `json:"dataDir"`
}

// UserDTO is a row in the user management table.
type UserDTO struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	RoleLabel          string `json:"roleLabel"`
	Disabled           bool   `json:"disabled"`
	MustChangePassword bool   `json:"mustChangePassword"`
	CreatedAt          int64  `json:"createdAt"`
	LastLoginAt        int64  `json:"lastLoginAt"`
}

// UserInput creates or updates a user.
type UserInput struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Password    string `json:"password"`
	Disabled    bool   `json:"disabled"`
}

// RoleOption lists a role for selection widgets.
type RoleOption struct {
	Value       string   `json:"value"`
	Label       string   `json:"label"`
	Permissions []string `json:"permissions"`
}

// AuditQuery filters the audit log.
type AuditQuery struct {
	Keyword string `json:"keyword"`
	Since   int64  `json:"since"`
	Until   int64  `json:"until"`
	Page    int    `json:"page"`
	Size    int    `json:"size"`
}

// AuditEntry is one audit row.
type AuditEntry struct {
	ID       int64  `json:"id"`
	TS       int64  `json:"ts"`
	Username string `json:"username"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	Success  bool   `json:"success"`
	Detail   string `json:"detail"`
}

// AuditPage is a page of audit rows.
type AuditPage struct {
	Items []AuditEntry `json:"items"`
	Total int          `json:"total"`
}

// DeviceDTO is a managed device as shown in the UI. Secrets never leave the
// backend through this type.
type DeviceDTO struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	IPMIPort       int    `json:"ipmiPort"`
	HTTPSPort      int    `json:"httpsPort"`
	Protocol       string `json:"protocol"`
	GroupID        int64  `json:"groupId"`
	GroupName      string `json:"groupName"`
	CredentialID   int64  `json:"credentialId"`
	CredentialName string `json:"credentialName"`
	Username       string `json:"username"`
	HasPassword    bool   `json:"hasPassword"`
	Note           string `json:"note"`

	Vendor         string `json:"vendor"`
	VendorName     string `json:"vendorName"`
	Manufacturer   string `json:"manufacturer"`
	Model          string `json:"model"`
	Serial         string `json:"serial"`
	BMCFirmware    string `json:"bmcFirmware"`
	BIOSVersion    string `json:"biosVersion"`
	HasIPMI        bool   `json:"hasIpmi"`
	HasRedfish     bool   `json:"hasRedfish"`
	TLSFingerprint string `json:"tlsFingerprint"`

	PowerState   string `json:"powerState"`
	Health       string `json:"health"`
	Reachable    bool   `json:"reachable"`
	LastError    string `json:"lastError"`
	CheckedAt    int64  `json:"checkedAt"`
	IdentifiedAt int64  `json:"identifiedAt"`
	CreatedAt    int64  `json:"createdAt"`
}

func deviceDTO(d *store.Device, groups map[int64]string, creds map[int64]string) DeviceDTO {
	dto := DeviceDTO{
		ID: d.ID, Name: d.Name, Host: d.Host, IPMIPort: d.IPMIPort, HTTPSPort: d.HTTPSPort, Protocol: d.Protocol,
		GroupID: d.GroupID, GroupName: groups[d.GroupID], CredentialID: d.CredentialID, CredentialName: creds[d.CredentialID],
		Username: d.Username, HasPassword: len(d.PasswordEnc) > 0, Note: d.Note,
		Vendor: d.Vendor, VendorName: bmc.VendorName(d.Vendor), Manufacturer: d.Manufacturer, Model: d.Model,
		Serial: d.Serial, BMCFirmware: d.BMCFirmware, BIOSVersion: d.BIOSVersion, HasIPMI: d.HasIPMI,
		HasRedfish: d.HasRedfish, TLSFingerprint: d.TLSFingerprint, PowerState: d.PowerState, Health: d.Health,
		Reachable: d.Reachable, LastError: d.LastError, CheckedAt: d.CheckedAt, IdentifiedAt: d.IdentifiedAt,
		CreatedAt: d.CreatedAt,
	}
	if d.CredentialID != 0 {
		dto.Username = ""
	}
	return dto
}

// DeviceInput creates or updates a device. On update an empty Password
// keeps the stored one.
type DeviceInput struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Host         string `json:"host"`
	IPMIPort     int    `json:"ipmiPort"`
	HTTPSPort    int    `json:"httpsPort"`
	Protocol     string `json:"protocol"`
	GroupID      int64  `json:"groupId"`
	CredentialID int64  `json:"credentialId"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Note         string `json:"note"`
}

// DeviceStatusEvent is pushed to the frontend when a device's state changes.
type DeviceStatusEvent struct {
	ID         int64  `json:"id"`
	PowerState string `json:"powerState"`
	Health     string `json:"health"`
	Reachable  bool   `json:"reachable"`
	LastError  string `json:"lastError"`
	CheckedAt  int64  `json:"checkedAt"`
}

// GroupDTO is a device group.
type GroupDTO struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DeviceCount int    `json:"deviceCount"`
}

// CredentialDTO is a credential profile without its password.
type CredentialDTO struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Username    string `json:"username"`
	Note        string `json:"note"`
	DeviceCount int    `json:"deviceCount"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// CredentialInput creates or updates a credential profile. On update an
// empty Password keeps the stored one.
type CredentialInput struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	Note     string `json:"note"`
}

// BatchItem is the outcome of a batch operation on one device.
type BatchItem struct {
	BatchID  string `json:"batchId"`
	DeviceID int64  `json:"deviceId"`
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Error    string `json:"error"`
	Via      string `json:"via"`
}

// BatchResult summarizes a batch operation.
type BatchResult struct {
	BatchID   string      `json:"batchId"`
	Total     int         `json:"total"`
	Succeeded int         `json:"succeeded"`
	Failed    int         `json:"failed"`
	Items     []BatchItem `json:"items"`
}

// SensorsResult carries sensor readings.
type SensorsResult struct {
	Sensors   []bmc.Sensor `json:"sensors"`
	Via       string       `json:"via"`
	FetchedAt int64        `json:"fetchedAt"`
}

// SELResult carries event log entries.
type SELResult struct {
	Entries   []bmc.SELEntry `json:"entries"`
	Via       string         `json:"via"`
	FetchedAt int64          `json:"fetchedAt"`
}

// InventoryResult carries a (possibly cached) inventory.
type InventoryResult struct {
	Inventory *bmc.Inventory `json:"inventory"`
	UpdatedAt int64          `json:"updatedAt"`
	Cached    bool           `json:"cached"`
}

// ConsoleResult tells the UI what was opened.
type ConsoleResult struct {
	URL       string `json:"url"`
	AutoLogin bool   `json:"autoLogin"`
	Hint      string `json:"hint"`
}

// ScanRequest starts a discovery scan. ScanID is chosen by the caller so it
// can match progress events that arrive before StartScan returns.
type ScanRequest struct {
	ScanID    string `json:"scanId"`
	Targets   string `json:"targets"`
	IPMIPort  int    `json:"ipmiPort"`
	HTTPSPort int    `json:"httpsPort"`
}

// ScanStarted acknowledges a scan.
type ScanStarted struct {
	ScanID string `json:"scanId"`
	Total  int    `json:"total"`
}

// ScanFound is one discovered BMC.
type ScanFound struct {
	ScanID         string `json:"scanId"`
	Host           string `json:"host"`
	IPMI           bool   `json:"ipmi"`
	IPMIVersion    string `json:"ipmiVersion"`
	Redfish        bool   `json:"redfish"`
	RedfishVersion string `json:"redfishVersion"`
	Vendor         string `json:"vendor"`
	VendorName     string `json:"vendorName"`
	Product        string `json:"product"`
	TLSFingerprint string `json:"tlsFingerprint"`
	ManagedID      int64  `json:"managedId"`
}

// ScanProgress reports scan progress.
type ScanProgress struct {
	ScanID    string `json:"scanId"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Found     int    `json:"found"`
	Finished  bool   `json:"finished"`
	Cancelled bool   `json:"cancelled"`
}

// DiscoveredHost is a scan result selected for adding.
type DiscoveredHost struct {
	Host           string `json:"host"`
	Name           string `json:"name"`
	IPMI           bool   `json:"ipmi"`
	Redfish        bool   `json:"redfish"`
	Vendor         string `json:"vendor"`
	TLSFingerprint string `json:"tlsFingerprint"`
}

// AddDiscoveredRequest adds discovered hosts with shared settings.
type AddDiscoveredRequest struct {
	Hosts        []DiscoveredHost `json:"hosts"`
	IPMIPort     int              `json:"ipmiPort"`
	HTTPSPort    int              `json:"httpsPort"`
	GroupID      int64            `json:"groupId"`
	CredentialID int64            `json:"credentialId"`
	Username     string           `json:"username"`
	Password     string           `json:"password"`
}

// AddDiscoveredResult reports which hosts were added.
type AddDiscoveredResult struct {
	Added   int      `json:"added"`
	Skipped []string `json:"skipped"`
}

// Overview feeds the dashboard.
type Overview struct {
	Total       int          `json:"total"`
	Reachable   int          `json:"reachable"`
	Unreachable int          `json:"unreachable"`
	PoweredOn   int          `json:"poweredOn"`
	PoweredOff  int          `json:"poweredOff"`
	HealthOK    int          `json:"healthOk"`
	Warning     int          `json:"warning"`
	Critical    int          `json:"critical"`
	Unknown     int          `json:"unknown"`
	ByVendor    []CountItem  `json:"byVendor"`
	ByGroup     []CountItem  `json:"byGroup"`
	Problems    []DeviceDTO  `json:"problems"`
	Recent      []AuditEntry `json:"recent"`
	LastPoll    int64        `json:"lastPoll"`
	Polling     bool         `json:"polling"`
}

// CountItem is a labelled count.
type CountItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// UpdateStatus is the software update state; every logged-in user sees it.
type UpdateStatus struct {
	CurrentVersion string `json:"currentVersion"`
	// State is idle, checking, upToDate, available, downloading, ready or
	// installing (see update.State).
	State string `json:"state"`
	// Mode is installer, portable or unsupported (see update.Mode).
	Mode       string         `json:"mode"`
	CanInstall bool           `json:"canInstall"`
	Latest     *UpdateRelease `json:"latest"`
	Downloaded int64          `json:"downloaded"`
	Total      int64          `json:"total"`
	Speed      int64          `json:"speed"`
	Error      string         `json:"error"`
	CheckedAt  int64          `json:"checkedAt"`
	// LastResult is the outcome of the update installed before this start,
	// until a user has seen it.
	LastResult *UpdateResult `json:"lastResult"`
}

// UpdateRelease describes the newer version.
type UpdateRelease struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	PublishedAt int64  `json:"publishedAt"`
	Notes       string `json:"notes"`
	Size        int64  `json:"size"` // of the package for this install mode, 0 when there is none
}

// UpdateResult is the outcome of an installed update.
type UpdateResult struct {
	From    string `json:"from"`
	Version string `json:"version"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Time    int64  `json:"time"`
}

// UpdateConfig is where updates come from; administrators only.
type UpdateConfig struct {
	CheckHours int    `json:"checkHours"` // automatic check interval, 0 disables
	Source     string `json:"source"`     // "" for the default (GitHub)
	Proxy      string `json:"proxy"`      // "" for the system proxy
}
