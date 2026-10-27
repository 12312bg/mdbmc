package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/store"
	"github.com/12312bg/mdbmc/internal/vault"
)

// AdminService manages local users, settings and the audit log.
type AdminService struct{ a *App }

// Roles lists the fixed roles and their permissions.
func (s *AdminService) Roles() []RoleOption {
	out := make([]RoleOption, 0, len(auth.Roles))
	for _, r := range auth.Roles {
		perms := []string{}
		for _, p := range r.Permissions() {
			perms = append(perms, string(p))
		}
		out = append(out, RoleOption{Value: string(r), Label: r.Label(), Permissions: perms})
	}
	return out
}

// ListUsers returns all local users.
func (s *AdminService) ListUsers() ([]UserDTO, error) {
	if _, err := s.a.auth.Require(auth.PermUserManage); err != nil {
		return nil, err
	}
	users, err := s.a.store.ListUsers(s.a.bg())
	if err != nil {
		return nil, err
	}
	out := make([]UserDTO, 0, len(users))
	for _, u := range users {
		out = append(out, UserDTO{
			ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: u.Role,
			RoleLabel: auth.Role(u.Role).Label(), Disabled: u.Disabled, MustChangePassword: u.MustChangePassword,
			CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
		})
	}
	return out, nil
}

// CreateUser adds a user; they must change the initial password at first login.
func (s *AdminService) CreateUser(in UserInput) error {
	sess, err := s.a.auth.Require(auth.PermUserManage)
	if err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	if err := validateUsername(in.Username); err != nil {
		return err
	}
	if !auth.Role(in.Role).Valid() {
		return errors.New("请选择有效的角色")
	}
	if err := validatePassword(in.Password); err != nil {
		return err
	}
	salt, wrapped, err := vault.Wrap(sess.DataKey(), in.Password, vault.DefaultKDF)
	if err != nil {
		return err
	}
	u := &store.User{
		Username: in.Username, DisplayName: strings.TrimSpace(in.DisplayName), Role: in.Role,
		KDFSalt: salt, KDF: vault.DefaultKDF, WrappedKey: wrapped, MustChangePassword: true,
	}
	err = s.a.store.CreateUser(s.a.bg(), u)
	s.a.audit(sess, "user.create", in.Username, err, "角色: "+auth.Role(in.Role).Label())
	return err
}

// UpdateUser changes a user's display name, role or disabled flag.
func (s *AdminService) UpdateUser(in UserInput) error {
	sess, err := s.a.auth.Require(auth.PermUserManage)
	if err != nil {
		return err
	}
	ctx := s.a.bg()
	u, err := s.a.store.UserByID(ctx, in.ID)
	if err != nil {
		return err
	}
	if !auth.Role(in.Role).Valid() {
		return errors.New("请选择有效的角色")
	}
	if u.ID == sess.UserID && (in.Role != u.Role || in.Disabled) {
		return errors.New("不能修改自己的角色或停用自己")
	}
	if err := s.keepAnAdmin(u, in.Role, in.Disabled); err != nil {
		return err
	}
	err = s.a.store.UpdateUserProfile(ctx, u.ID, strings.TrimSpace(in.DisplayName), in.Role, in.Disabled)
	detail := fmt.Sprintf("角色: %s", auth.Role(in.Role).Label())
	if in.Disabled {
		detail += "，已停用"
	}
	s.a.audit(sess, "user.update", u.Username, err, detail)
	return err
}

// keepAnAdmin refuses changes that would leave no enabled administrator.
func (s *AdminService) keepAnAdmin(u *store.User, newRole string, disabled bool) error {
	if u.Role != string(auth.RoleAdmin) || u.Disabled {
		return nil
	}
	if newRole == string(auth.RoleAdmin) && !disabled {
		return nil
	}
	n, err := s.a.store.CountActiveAdmins(s.a.bg())
	if err != nil {
		return err
	}
	if n <= 1 {
		return errors.New("至少需要保留一个启用的管理员")
	}
	return nil
}

// ResetPassword sets a new password for another user; they must change it
// at next login.
func (s *AdminService) ResetPassword(id int64, newPassword string) error {
	sess, err := s.a.auth.Require(auth.PermUserManage)
	if err != nil {
		return err
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	ctx := s.a.bg()
	u, err := s.a.store.UserByID(ctx, id)
	if err != nil {
		return err
	}
	if u.ID == sess.UserID {
		return errors.New("请在“修改密码”中修改自己的密码")
	}
	salt, wrapped, err := vault.Wrap(sess.DataKey(), newPassword, vault.DefaultKDF)
	if err != nil {
		return err
	}
	err = s.a.store.UpdateUserKey(ctx, u.ID, salt, vault.DefaultKDF, wrapped, true)
	s.a.audit(sess, "user.reset_password", u.Username, err, "")
	return err
}

// DeleteUser removes a user.
func (s *AdminService) DeleteUser(id int64) error {
	sess, err := s.a.auth.Require(auth.PermUserManage)
	if err != nil {
		return err
	}
	ctx := s.a.bg()
	u, err := s.a.store.UserByID(ctx, id)
	if err != nil {
		return err
	}
	if u.ID == sess.UserID {
		return errors.New("不能删除当前登录的账号")
	}
	if err := s.keepAnAdmin(u, "", true); err != nil {
		return err
	}
	err = s.a.store.DeleteUser(ctx, id)
	s.a.audit(sess, "user.delete", u.Username, err, "")
	return err
}

// GetSettings returns the settings; any logged-in user may read them.
func (s *AdminService) GetSettings() (*Settings, error) {
	if _, err := s.a.auth.Require(); err != nil {
		return nil, err
	}
	st := s.a.Settings()
	return &st, nil
}

// SaveSettings validates and stores new settings.
func (s *AdminService) SaveSettings(in Settings) error {
	sess, err := s.a.auth.Require(auth.PermSettingsManage)
	if err != nil {
		return err
	}
	err = s.a.saveSettings(s.a.bg(), in)
	s.a.audit(sess, "settings.update", "", err, fmt.Sprintf("刷新间隔 %ds，并发 %d，空闲锁定 %d 分钟",
		in.PollIntervalSec, in.MaxConcurrency, in.IdleLockMinutes))
	return err
}

// ListAudit returns a page of the audit log.
func (s *AdminService) ListAudit(q AuditQuery) (*AuditPage, error) {
	if _, err := s.a.auth.Require(auth.PermAuditView); err != nil {
		return nil, err
	}
	if q.Size <= 0 || q.Size > 200 {
		q.Size = 50
	}
	if q.Page < 1 {
		q.Page = 1
	}
	rows, total, err := s.a.store.ListAudit(s.a.bg(), store.AuditQuery{
		Keyword: strings.TrimSpace(q.Keyword), Since: q.Since, Until: q.Until,
		Offset: (q.Page - 1) * q.Size, Limit: q.Size,
	})
	if err != nil {
		return nil, err
	}
	return &AuditPage{Items: auditDTOs(rows), Total: total}, nil
}

func auditDTOs(rows []*store.AuditEntry) []AuditEntry {
	out := make([]AuditEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuditEntry{ID: r.ID, TS: r.TS, Username: r.Username, Action: r.Action, Target: r.Target, Success: r.Success, Detail: r.Detail})
	}
	return out
}
