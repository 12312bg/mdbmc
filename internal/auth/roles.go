// Package auth defines the fixed local roles, their permissions and the
// in-memory login session.
package auth

import "slices"

// Role is one of the three fixed local roles.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Roles lists the roles in descending order of privilege.
var Roles = []Role{RoleAdmin, RoleOperator, RoleViewer}

// Permission is a single capability checked by the service layer.
type Permission string

const (
	PermDeviceView       Permission = "device.view"       // see devices, status, sensors, SEL, inventory
	PermDeviceManage     Permission = "device.manage"     // add/edit/delete/scan devices, groups, credential profiles
	PermCredentialReveal Permission = "credential.reveal" // read stored BMC passwords in clear text
	PermPowerControl     Permission = "power.control"     // power on/off/reset/cycle, identify LED
	PermSELClear         Permission = "sel.clear"         // clear the system event log
	PermWebOpen          Permission = "console.web"       // open the BMC web login page (no auto-login)
	PermWebAutoLogin     Permission = "console.autologin" // open the BMC web UI with credentials filled in
	PermKVM              Permission = "console.kvm"       // open the remote KVM console
	PermUserManage       Permission = "user.manage"       // manage local users
	PermSettingsManage   Permission = "settings.manage"   // change application settings
	PermAuditView        Permission = "audit.view"        // read the audit log
)

// AllPermissions is every permission, used for the administrator role.
var AllPermissions = []Permission{
	PermDeviceView, PermDeviceManage, PermCredentialReveal, PermPowerControl,
	PermSELClear, PermWebOpen, PermWebAutoLogin, PermKVM,
	PermUserManage, PermSettingsManage, PermAuditView,
}

var rolePermissions = map[Role][]Permission{
	RoleAdmin: AllPermissions,
	RoleOperator: {
		PermDeviceView, PermDeviceManage, PermCredentialReveal, PermPowerControl,
		PermSELClear, PermWebOpen, PermWebAutoLogin, PermKVM,
	},
	RoleViewer: {
		PermDeviceView, PermWebOpen,
	},
}

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	_, ok := rolePermissions[r]
	return ok
}

// Can reports whether the role grants p.
func (r Role) Can(p Permission) bool {
	return slices.Contains(rolePermissions[r], p)
}

// Permissions returns the permissions granted to the role.
func (r Role) Permissions() []Permission {
	return slices.Clone(rolePermissions[r])
}

// Label is the Chinese display name of the role.
func (r Role) Label() string {
	switch r {
	case RoleAdmin:
		return "管理员"
	case RoleOperator:
		return "操作员"
	case RoleViewer:
		return "只读"
	}
	return string(r)
}
