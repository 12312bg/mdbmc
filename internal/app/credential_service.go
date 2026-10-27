package app

import (
	"errors"
	"strings"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/store"
)

// CredentialService manages reusable BMC credential profiles.
type CredentialService struct{ a *App }

// List returns all credential profiles (without passwords).
func (s *CredentialService) List() ([]CredentialDTO, error) {
	if _, err := s.a.auth.Require(auth.PermDeviceView); err != nil {
		return nil, err
	}
	creds, err := s.a.store.ListCredentials(s.a.bg())
	if err != nil {
		return nil, err
	}
	out := make([]CredentialDTO, 0, len(creds))
	for _, c := range creds {
		out = append(out, CredentialDTO{ID: c.ID, Name: c.Name, Username: c.Username, Note: c.Note, DeviceCount: c.DeviceCount, UpdatedAt: c.UpdatedAt})
	}
	return out, nil
}

func normalizeCredential(in *CredentialInput, requirePassword bool) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.TrimSpace(in.Username)
	in.Note = strings.TrimSpace(in.Note)
	if in.Name == "" || len([]rune(in.Name)) > 40 {
		return errors.New("凭据名称需为 1-40 个字符")
	}
	if in.Username == "" {
		return errors.New("请填写 BMC 用户名")
	}
	if requirePassword && in.Password == "" {
		return errors.New("请填写 BMC 密码")
	}
	return nil
}

// Create adds a credential profile.
func (s *CredentialService) Create(in CredentialInput) error {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return err
	}
	if err := normalizeCredential(&in, true); err != nil {
		return err
	}
	c := &store.Credential{Name: in.Name, Username: in.Username, Note: in.Note,
		PasswordEnc: sess.Box().SealString(in.Password, purposeCredentialPassword)}
	err = s.a.store.CreateCredential(s.a.bg(), c)
	s.a.audit(sess, "credential.create", in.Name, err, "")
	return err
}

// Update edits a profile; an empty password keeps the stored one.
func (s *CredentialService) Update(in CredentialInput) error {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return err
	}
	if err := normalizeCredential(&in, false); err != nil {
		return err
	}
	c := &store.Credential{ID: in.ID, Name: in.Name, Username: in.Username, Note: in.Note}
	keep := in.Password == ""
	if !keep {
		c.PasswordEnc = sess.Box().SealString(in.Password, purposeCredentialPassword)
	}
	err = s.a.store.UpdateCredential(s.a.bg(), c, keep)
	s.a.audit(sess, "credential.update", in.Name, err, "")
	return err
}

// Delete removes an unused profile.
func (s *CredentialService) Delete(id int64) error {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return err
	}
	c, err := s.a.store.CredentialByID(s.a.bg(), id)
	if err != nil {
		return err
	}
	err = s.a.store.DeleteCredential(s.a.bg(), id)
	s.a.audit(sess, "credential.delete", c.Name, err, "")
	return err
}

// Reveal returns a profile's password in clear text.
func (s *CredentialService) Reveal(id int64) (string, error) {
	sess, err := s.a.auth.Require(auth.PermCredentialReveal)
	if err != nil {
		return "", err
	}
	c, err := s.a.store.CredentialByID(s.a.bg(), id)
	if err != nil {
		return "", err
	}
	pass, err := sess.Box().OpenString(c.PasswordEnc, purposeCredentialPassword)
	s.a.audit(sess, "credential.reveal", c.Name, err, "")
	return pass, err
}
