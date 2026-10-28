package app

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/store"
	"github.com/12312bg/mdbmc/internal/vault"
)

// AuthService handles first-run setup, login and the user's own password.
type AuthService struct{ a *App }

const minPasswordLen = 8

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]{2,32}$`)

func validateUsername(u string) error {
	if !usernameRe.MatchString(u) {
		return errors.New("用户名需为 2-32 位字母、数字或 _ . @ -")
	}
	return nil
}

func validatePassword(p string) error {
	if utf8.RuneCountInString(p) < minPasswordLen {
		return errors.New("密码长度至少 8 位")
	}
	if strings.TrimSpace(p) != p {
		return errors.New("密码首尾不能包含空格")
	}
	return nil
}

// Status reports whether setup is done and who is logged in.
func (s *AuthService) Status() (*AuthStatus, error) {
	n, err := s.a.store.CountUsers(s.a.bg())
	if err != nil {
		return nil, err
	}
	st := &AuthStatus{Initialized: n > 0, Version: Version, DataDir: s.a.dataDir}
	if sess := s.a.auth.Current(); sess != nil {
		if u, err := s.a.store.UserByID(s.a.bg(), sess.UserID); err == nil {
			st.User = userInfo(u)
		}
	}
	return st, nil
}

// Setup creates the vault and the first administrator, then logs in.
func (s *AuthService) Setup(username, displayName, password string) (*UserInfo, error) {
	ctx := s.a.bg()
	username = strings.TrimSpace(username)
	if err := validateUsername(username); err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	n, err := s.a.store.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, errors.New("系统已初始化")
	}
	dek, err := vault.NewDataKey()
	if err != nil {
		return nil, err
	}
	salt, wrapped, err := vault.Wrap(dek, password, vault.DefaultKDF)
	if err != nil {
		return nil, err
	}
	u := &store.User{
		Username: username, DisplayName: strings.TrimSpace(displayName), Role: string(auth.RoleAdmin),
		KDFSalt: salt, KDF: vault.DefaultKDF, WrappedKey: wrapped,
	}
	if err := s.a.store.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	return s.startSession(ctx, u, dek)
}

func (s *AuthService) startSession(ctx context.Context, u *store.User, dek []byte) (*UserInfo, error) {
	sess, err := auth.NewSession(u.ID, u.Username, u.DisplayName, auth.Role(u.Role), dek, u.MustChangePassword)
	if err != nil {
		return nil, err
	}
	s.a.auth.Set(sess)
	_ = s.a.store.TouchLogin(ctx, u.ID)
	s.a.audit(sess, "auth.login", u.Username, nil, "")
	s.a.poller.start()
	return userInfo(u), nil
}

// Login unlocks the vault with the user's password.
func (s *AuthService) Login(username, password string) (*UserInfo, error) {
	ctx := s.a.bg()
	username = strings.TrimSpace(username)
	if err := s.a.auth.CheckThrottle(username); err != nil {
		return nil, err
	}
	fail := func(reason string) (*UserInfo, error) {
		s.a.auth.RecordFailure(username)
		s.a.audit(nil, "auth.login_failed", username, errors.New(reason), "")
		return nil, errors.New("用户名或密码错误")
	}
	u, err := s.a.store.UserByName(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		// Spend comparable time so usernames cannot be probed by timing.
		_, _, _ = vault.Wrap(make([]byte, 32), password, vault.DefaultKDF)
		return fail("用户不存在")
	}
	if err != nil {
		return nil, err
	}
	dek, err := vault.Unwrap(u.WrappedKey, u.KDFSalt, password, u.KDF)
	if err != nil {
		return fail("密码错误")
	}
	if u.Disabled {
		vault.Zero(dek)
		s.a.audit(nil, "auth.login_failed", username, errors.New("账号已停用"), "")
		return nil, errors.New("该账号已被停用，请联系管理员")
	}
	s.a.auth.RecordSuccess(username)
	return s.startSession(ctx, u, dek)
}

// Logout ends the session and wipes the key from memory.
func (s *AuthService) Logout() {
	sess := s.a.auth.Current()
	s.a.poller.stop()
	s.a.scanner.cancel()
	if sess != nil {
		s.a.audit(sess, "auth.logout", sess.Username, nil, "")
	}
	s.a.auth.Clear()
}

// ChangePassword re-wraps the vault key with a new password.
func (s *AuthService) ChangePassword(oldPassword, newPassword string) error {
	ctx := s.a.bg()
	// Not Require(): a user on an initial password must be able to change it.
	sess := s.a.auth.Current()
	if sess == nil {
		return auth.ErrNotLoggedIn
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	if oldPassword == newPassword {
		return errors.New("新密码不能与原密码相同")
	}
	u, err := s.a.store.UserByID(ctx, sess.UserID)
	if err != nil {
		return err
	}
	dek, err := vault.Unwrap(u.WrappedKey, u.KDFSalt, oldPassword, u.KDF)
	if err != nil {
		s.a.audit(sess, "auth.password_change", u.Username, errors.New("原密码错误"), "")
		return errors.New("原密码错误")
	}
	defer vault.Zero(dek)
	salt, wrapped, err := vault.Wrap(dek, newPassword, vault.DefaultKDF)
	if err != nil {
		return err
	}
	err = s.a.store.UpdateUserKey(ctx, u.ID, salt, vault.DefaultKDF, wrapped, false)
	s.a.audit(sess, "auth.password_change", u.Username, err, "")
	if err == nil {
		sess.PasswordChanged()
	}
	return err
}
