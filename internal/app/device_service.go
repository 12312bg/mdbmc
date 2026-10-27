package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/store"
)

// DeviceService manages devices and groups.
type DeviceService struct{ a *App }

func (a *App) lookupNames(ctx context.Context) (map[int64]string, map[int64]string, error) {
	groups, err := a.store.ListGroups(ctx)
	if err != nil {
		return nil, nil, err
	}
	creds, err := a.store.ListCredentials(ctx)
	if err != nil {
		return nil, nil, err
	}
	g := map[int64]string{}
	for _, x := range groups {
		g[x.ID] = x.Name
	}
	c := map[int64]string{}
	for _, x := range creds {
		c[x.ID] = x.Name
	}
	return g, c, nil
}

func (a *App) deviceDTOs(ctx context.Context, devs []*store.Device) ([]DeviceDTO, error) {
	g, c, err := a.lookupNames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DeviceDTO, 0, len(devs))
	for _, d := range devs {
		out = append(out, deviceDTO(d, g, c))
	}
	return out, nil
}

func (a *App) oneDTO(ctx context.Context, id int64) (*DeviceDTO, error) {
	d, err := a.store.DeviceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	list, err := a.deviceDTOs(ctx, []*store.Device{d})
	if err != nil {
		return nil, err
	}
	return &list[0], nil
}

// List returns all devices.
func (s *DeviceService) List() ([]DeviceDTO, error) {
	if _, err := s.a.auth.Require(auth.PermDeviceView); err != nil {
		return nil, err
	}
	devs, err := s.a.store.ListDevices(s.a.bg())
	if err != nil {
		return nil, err
	}
	return s.a.deviceDTOs(s.a.bg(), devs)
}

// Get returns one device.
func (s *DeviceService) Get(id int64) (*DeviceDTO, error) {
	if _, err := s.a.auth.Require(auth.PermDeviceView); err != nil {
		return nil, err
	}
	return s.a.oneDTO(s.a.bg(), id)
}

// Vendors lists known vendors for filters.
func (s *DeviceService) Vendors() []bmc.VendorOption { return bmc.Vendors() }

func normalizeDeviceInput(in *DeviceInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	in.Note = strings.TrimSpace(in.Note)
	if in.Host == "" {
		return errors.New("请填写 BMC 地址")
	}
	if strings.ContainsAny(in.Host, " /\\?#") {
		return errors.New("BMC 地址格式不正确")
	}
	if net.ParseIP(in.Host) == nil && strings.Contains(in.Host, ":") {
		return errors.New("BMC 地址不应包含端口，请在端口字段中填写")
	}
	if in.Name == "" {
		in.Name = in.Host
	}
	if in.IPMIPort == 0 {
		in.IPMIPort = 623
	}
	if in.HTTPSPort == 0 {
		in.HTTPSPort = 443
	}
	if in.IPMIPort < 1 || in.IPMIPort > 65535 || in.HTTPSPort < 1 || in.HTTPSPort > 65535 {
		return errors.New("端口范围应为 1-65535")
	}
	if in.Protocol == "" {
		in.Protocol = string(bmc.ProtocolAuto)
	}
	if !bmc.Protocol(in.Protocol).Valid() {
		return errors.New("管理协议无效")
	}
	if in.CredentialID == 0 && in.Username == "" {
		return errors.New("请选择凭据或填写 BMC 用户名")
	}
	return nil
}

// Create adds a device and identifies it in the background.
func (s *DeviceService) Create(in DeviceInput) (*DeviceDTO, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return nil, err
	}
	if err := normalizeDeviceInput(&in); err != nil {
		return nil, err
	}
	d := &store.Device{
		Name: in.Name, Host: in.Host, IPMIPort: in.IPMIPort, HTTPSPort: in.HTTPSPort, Protocol: in.Protocol,
		GroupID: in.GroupID, CredentialID: in.CredentialID, Note: in.Note,
	}
	if in.CredentialID == 0 {
		d.Username = in.Username
		d.PasswordEnc = sess.Box().SealString(in.Password, purposeDevicePassword)
	}
	ctx := s.a.bg()
	err = s.a.store.CreateDevice(ctx, d)
	s.a.audit(sess, "device.create", deviceLabel(d), err, "")
	if err != nil {
		return nil, err
	}
	go s.a.identify(sess, []int64{d.ID})
	return s.a.oneDTO(ctx, d.ID)
}

// Update saves connection settings. An empty password keeps the stored one.
func (s *DeviceService) Update(in DeviceInput) (*DeviceDTO, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return nil, err
	}
	if err := normalizeDeviceInput(&in); err != nil {
		return nil, err
	}
	ctx := s.a.bg()
	d, err := s.a.store.DeviceByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	endpointChanged := d.Host != in.Host || d.HTTPSPort != in.HTTPSPort || d.IPMIPort != in.IPMIPort
	d.Name, d.Host, d.IPMIPort, d.HTTPSPort, d.Protocol = in.Name, in.Host, in.IPMIPort, in.HTTPSPort, in.Protocol
	d.GroupID, d.CredentialID, d.Note = in.GroupID, in.CredentialID, in.Note
	keep := true
	if in.CredentialID == 0 {
		d.Username = in.Username
		if in.Password != "" || len(d.PasswordEnc) == 0 {
			d.PasswordEnc = s.a.sealDevicePassword(sess, in.Password)
			keep = false
		}
	} else {
		d.Username, d.PasswordEnc, keep = "", nil, false
	}
	err = s.a.store.UpdateDeviceConfig(ctx, d, keep)
	s.a.audit(sess, "device.update", deviceLabel(d), err, "")
	if err != nil {
		return nil, err
	}
	if endpointChanged {
		// A different endpoint is a different certificate.
		_ = s.a.store.SetTLSFingerprint(ctx, d.ID, "")
	}
	go s.a.identify(sess, []int64{d.ID})
	return s.a.oneDTO(ctx, d.ID)
}

func (a *App) sealDevicePassword(sess *auth.Session, pw string) []byte {
	return sess.Box().SealString(pw, purposeDevicePassword)
}

// Delete removes devices.
func (s *DeviceService) Delete(ids []int64) (int, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return 0, err
	}
	ctx := s.a.bg()
	devs, _ := s.a.store.DevicesByIDs(ctx, ids)
	n, err := s.a.store.DeleteDevices(ctx, ids)
	var names []string
	for _, d := range devs {
		names = append(names, deviceLabel(d))
	}
	s.a.audit(sess, "device.delete", strings.Join(names, ", "), err, fmt.Sprintf("共 %d 台", n))
	return int(n), err
}

// MoveToGroup assigns devices to a group (0 = ungrouped).
func (s *DeviceService) MoveToGroup(ids []int64, groupID int64) error {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return err
	}
	err = s.a.store.SetDevicesGroup(s.a.bg(), ids, groupID)
	s.a.audit(sess, "device.move_group", fmt.Sprintf("%d 台设备", len(ids)), err, fmt.Sprintf("分组 ID %d", groupID))
	return err
}

// SetCredential points devices at a credential profile.
func (s *DeviceService) SetCredential(ids []int64, credentialID int64) error {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return err
	}
	if credentialID == 0 {
		return errors.New("请选择凭据")
	}
	err = s.a.store.SetDevicesCredential(s.a.bg(), ids, credentialID)
	s.a.audit(sess, "device.set_credential", fmt.Sprintf("%d 台设备", len(ids)), err, fmt.Sprintf("凭据 ID %d", credentialID))
	return err
}

// RevealPassword returns the BMC password a device uses.
func (s *DeviceService) RevealPassword(id int64) (string, error) {
	sess, err := s.a.auth.Require(auth.PermCredentialReveal)
	if err != nil {
		return "", err
	}
	ctx := s.a.bg()
	d, err := s.a.store.DeviceByID(ctx, id)
	if err != nil {
		return "", err
	}
	_, pass, err := s.a.credentialsFor(ctx, sess, d)
	s.a.audit(sess, "device.reveal_password", deviceLabel(d), err, "")
	return pass, err
}

// Identify re-detects protocols, vendor and identity.
func (s *DeviceService) Identify(id int64) (*DeviceDTO, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return nil, err
	}
	if err := s.a.identifyOne(s.a.bg(), sess, id); err != nil {
		return nil, err
	}
	return s.a.oneDTO(s.a.bg(), id)
}

// RefreshStatus polls the given devices now (all when ids is empty).
func (s *DeviceService) RefreshStatus(ids []int64) error {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return err
	}
	go s.a.poller.pollNow(sess, ids)
	return nil
}

// TrustCertificate pins the certificate the BMC presents now. It is used
// after a BMC certificate was legitimately replaced.
func (s *DeviceService) TrustCertificate(id int64) (string, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(s.a.bg(), 15*time.Second)
	defer cancel()
	d, err := s.a.store.DeviceByID(ctx, id)
	if err != nil {
		return "", err
	}
	fp, err := bmc.ProbeCertificate(ctx, d.Host, d.HTTPSPort, "")
	if err == nil {
		err = s.a.store.SetTLSFingerprint(ctx, id, fp)
	}
	s.a.audit(sess, "device.trust_cert", deviceLabel(d), err, "新指纹 "+fp)
	return fp, err
}

// ListGroups returns all groups.
func (s *DeviceService) ListGroups() ([]GroupDTO, error) {
	if _, err := s.a.auth.Require(auth.PermDeviceView); err != nil {
		return nil, err
	}
	groups, err := s.a.store.ListGroups(s.a.bg())
	if err != nil {
		return nil, err
	}
	out := make([]GroupDTO, 0, len(groups))
	for _, g := range groups {
		out = append(out, GroupDTO{ID: g.ID, Name: g.Name, DeviceCount: g.DeviceCount})
	}
	return out, nil
}

func groupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 40 {
		return "", errors.New("分组名称需为 1-40 个字符")
	}
	return name, nil
}

// CreateGroup adds a group.
func (s *DeviceService) CreateGroup(name string) (*GroupDTO, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return nil, err
	}
	if name, err = groupName(name); err != nil {
		return nil, err
	}
	id, err := s.a.store.CreateGroup(s.a.bg(), name)
	s.a.audit(sess, "group.create", name, err, "")
	if err != nil {
		return nil, err
	}
	return &GroupDTO{ID: id, Name: name}, nil
}

// RenameGroup renames a group.
func (s *DeviceService) RenameGroup(id int64, name string) error {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return err
	}
	if name, err = groupName(name); err != nil {
		return err
	}
	err = s.a.store.RenameGroup(s.a.bg(), id, name)
	s.a.audit(sess, "group.rename", name, err, "")
	return err
}

// DeleteGroup deletes a group; its devices become ungrouped.
func (s *DeviceService) DeleteGroup(id int64) error {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return err
	}
	err = s.a.store.DeleteGroup(s.a.bg(), id)
	s.a.audit(sess, "group.delete", fmt.Sprintf("ID %d", id), err, "")
	return err
}

// identify runs Identify for several devices in the background.
func (a *App) identify(sess *auth.Session, ids []int64) {
	for _, id := range ids {
		go func(id int64) {
			if err := a.identifyOne(a.bg(), sess, id); err != nil {
				a.emit("device:updated", id)
			}
		}(id)
	}
}

// identifyOne refreshes a device's identity and status from its BMC.
func (a *App) identifyOne(ctx context.Context, sess *auth.Session, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var ident *bmc.Identity
	d, err := a.withClient(ctx, sess, id, func(_ *store.Device, c *bmc.Client) error {
		var err error
		ident, err = c.Identify(ctx)
		return err
	})
	if d == nil {
		return err
	}
	now := time.Now().Unix()
	if err != nil {
		st := store.Status{PowerState: d.PowerState, Health: d.Health, Reachable: false, LastError: err.Error(), CheckedAt: now}
		_ = a.store.UpdateDeviceStatus(context.Background(), id, st)
		a.emitStatus(id, st)
		return err
	}
	_ = a.store.UpdateDeviceIdentity(context.Background(), id, store.Identity{
		Vendor: ident.Vendor, Manufacturer: ident.Manufacturer, Model: ident.Model, Serial: ident.Serial,
		BMCFirmware: ident.BMCFirmware, BIOSVersion: ident.BIOSVersion, HasIPMI: ident.HasIPMI, HasRedfish: ident.HasRedfish,
	})
	a.pinIfFirst(d, ident.TLSFingerprint)
	st := store.Status{PowerState: string(ident.PowerState), Health: string(ident.Health), Reachable: true, CheckedAt: now}
	_ = a.store.UpdateDeviceStatus(context.Background(), id, st)
	a.emit("device:updated", id)
	a.emitStatus(id, st)
	return nil
}

func (a *App) emitStatus(id int64, st store.Status) {
	a.emit("device:status", DeviceStatusEvent{
		ID: id, PowerState: st.PowerState, Health: st.Health, Reachable: st.Reachable,
		LastError: st.LastError, CheckedAt: st.CheckedAt,
	})
}
