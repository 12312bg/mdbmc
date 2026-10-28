package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Device is a managed BMC.
type Device struct {
	ID           int64
	Name         string
	Host         string
	IPMIPort     int
	HTTPSPort    int
	Protocol     string // auto | ipmi | redfish
	GroupID      int64  // 0 = ungrouped
	CredentialID int64  // 0 = device-specific credentials
	Username     string
	PasswordEnc  []byte
	Note         string

	Vendor         string
	Manufacturer   string
	Model          string
	Serial         string
	BMCFirmware    string
	BIOSVersion    string
	HasIPMI        bool
	HasRedfish     bool
	TLSFingerprint string

	PowerState   string
	Health       string
	Reachable    bool
	LastError    string
	CheckedAt    int64
	IdentifiedAt int64
	CreatedAt    int64
	UpdatedAt    int64
}

const deviceCols = `id, name, host, ipmi_port, https_port, protocol, COALESCE(group_id, 0), COALESCE(credential_id, 0),
	username, password_enc, note, vendor, manufacturer, model, serial, bmc_firmware, bios_version,
	has_ipmi, has_redfish, tls_fingerprint, power_state, health, reachable, last_error,
	checked_at, identified_at, created_at, updated_at`

func scanDevice(row interface{ Scan(...any) error }) (*Device, error) {
	var d Device
	var hasIPMI, hasRF, reachable int
	err := row.Scan(&d.ID, &d.Name, &d.Host, &d.IPMIPort, &d.HTTPSPort, &d.Protocol, &d.GroupID, &d.CredentialID,
		&d.Username, &d.PasswordEnc, &d.Note, &d.Vendor, &d.Manufacturer, &d.Model, &d.Serial, &d.BMCFirmware, &d.BIOSVersion,
		&hasIPMI, &hasRF, &d.TLSFingerprint, &d.PowerState, &d.Health, &reachable, &d.LastError,
		&d.CheckedAt, &d.IdentifiedAt, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.HasIPMI, d.HasRedfish, d.Reachable = hasIPMI != 0, hasRF != 0, reachable != 0
	return &d, nil
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func deviceErr(err error) error {
	switch {
	case isUniqueErr(err):
		return errors.New("相同地址和端口的设备已存在")
	case isForeignKeyErr(err):
		return errors.New("所选分组或凭据不存在")
	}
	return err
}

// ListDevices returns all devices ordered by name.
func (s *Store) ListDevices(ctx context.Context) ([]*Device, error) {
	return s.queryDevices(ctx, "SELECT "+deviceCols+" FROM devices ORDER BY name COLLATE NOCASE, host")
}

// DevicesByIDs returns the devices with the given ids (missing ids are skipped).
func (s *Store) DevicesByIDs(ctx context.Context, ids []int64) ([]*Device, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.queryDevices(ctx, "SELECT "+deviceCols+" FROM devices WHERE id IN ("+placeholders(len(ids))+")", int64Args(ids)...)
}

func (s *Store) queryDevices(ctx context.Context, q string, args ...any) ([]*Device, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeviceByID returns one device.
func (s *Store) DeviceByID(ctx context.Context, id int64) (*Device, error) {
	return scanDevice(s.db.QueryRowContext(ctx, "SELECT "+deviceCols+" FROM devices WHERE id = ?", id))
}

// ManagedHosts maps lower-cased host to device id, used to flag hosts that
// discovery finds but are already managed.
func (s *Store) ManagedHosts(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, host FROM devices")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var host string
		if err := rows.Scan(&id, &host); err != nil {
			return nil, err
		}
		out[strings.ToLower(host)] = id
	}
	return out, rows.Err()
}

// CreateDevice inserts d and sets its ID.
func (s *Store) CreateDevice(ctx context.Context, d *Device) error {
	t := now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO devices
		(name, host, ipmi_port, https_port, protocol, group_id, credential_id, username, password_enc, note,
		 vendor, manufacturer, model, serial, has_ipmi, has_redfish, tls_fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Name, d.Host, d.IPMIPort, d.HTTPSPort, d.Protocol, nullID(d.GroupID), nullID(d.CredentialID),
		d.Username, d.PasswordEnc, d.Note, d.Vendor, d.Manufacturer, d.Model, d.Serial,
		boolInt(d.HasIPMI), boolInt(d.HasRedfish), d.TLSFingerprint, t, t)
	if err != nil {
		return deviceErr(err)
	}
	d.ID, _ = res.LastInsertId()
	d.CreatedAt, d.UpdatedAt = t, t
	if d.PowerState == "" {
		d.PowerState = "unknown"
	}
	if d.Health == "" {
		d.Health = "unknown"
	}
	return nil
}

// UpdateDeviceConfig saves user-editable connection settings. With
// keepPassword the stored device password is left untouched.
func (s *Store) UpdateDeviceConfig(ctx context.Context, d *Device, keepPassword bool) error {
	q := `UPDATE devices SET name = ?, host = ?, ipmi_port = ?, https_port = ?, protocol = ?, group_id = ?,
		credential_id = ?, username = ?, note = ?, updated_at = ?`
	args := []any{d.Name, d.Host, d.IPMIPort, d.HTTPSPort, d.Protocol, nullID(d.GroupID),
		nullID(d.CredentialID), d.Username, d.Note, now()}
	if !keepPassword {
		q += ", password_enc = ?"
		args = append(args, d.PasswordEnc)
	}
	q += " WHERE id = ?"
	args = append(args, d.ID)
	if err := s.execOne(ctx, q, args...); err != nil {
		return deviceErr(err)
	}
	return nil
}

// SetDevicesGroup moves devices into a group (0 = ungrouped).
func (s *Store) SetDevicesGroup(ctx context.Context, ids []int64, groupID int64) error {
	if len(ids) == 0 {
		return nil
	}
	args := append([]any{nullID(groupID), now()}, int64Args(ids)...)
	_, err := s.db.ExecContext(ctx, "UPDATE devices SET group_id = ?, updated_at = ? WHERE id IN ("+placeholders(len(ids))+")", args...)
	return deviceErr(err)
}

// SetDevicesCredential points devices at a credential profile.
func (s *Store) SetDevicesCredential(ctx context.Context, ids []int64, credID int64) error {
	if len(ids) == 0 {
		return nil
	}
	args := append([]any{nullID(credID), now()}, int64Args(ids)...)
	_, err := s.db.ExecContext(ctx, "UPDATE devices SET credential_id = ?, updated_at = ? WHERE id IN ("+placeholders(len(ids))+")", args...)
	return deviceErr(err)
}

// Identity is the hardware identity learned from the BMC.
type Identity struct {
	Vendor, Manufacturer, Model, Serial, BMCFirmware, BIOSVersion string
	HasIPMI, HasRedfish                                           bool
}

// UpdateDeviceIdentity stores detected identity fields.
func (s *Store) UpdateDeviceIdentity(ctx context.Context, id int64, ident Identity) error {
	return s.execOne(ctx, `UPDATE devices SET vendor = ?, manufacturer = ?, model = ?, serial = ?, bmc_firmware = ?,
		bios_version = ?, has_ipmi = ?, has_redfish = ?, identified_at = ? WHERE id = ?`,
		ident.Vendor, ident.Manufacturer, ident.Model, ident.Serial, ident.BMCFirmware, ident.BIOSVersion,
		boolInt(ident.HasIPMI), boolInt(ident.HasRedfish), now(), id)
}

// Status is the last polled device state.
type Status struct {
	PowerState string
	Health     string
	Reachable  bool
	LastError  string
	CheckedAt  int64
}

// UpdateDeviceStatus stores the last polled state.
func (s *Store) UpdateDeviceStatus(ctx context.Context, id int64, st Status) error {
	_, err := s.db.ExecContext(ctx, `UPDATE devices SET power_state = ?, health = ?, reachable = ?, last_error = ?, checked_at = ? WHERE id = ?`,
		st.PowerState, st.Health, boolInt(st.Reachable), st.LastError, st.CheckedAt, id)
	return err
}

// PinTLSFingerprint records fp only if the device has no pin yet and still
// points at host:port, so a slow operation against an old endpoint cannot pin
// that endpoint's certificate after the device was re-pointed.
func (s *Store) PinTLSFingerprint(ctx context.Context, id int64, host string, port int, fp string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE devices SET tls_fingerprint = ?
		WHERE id = ? AND tls_fingerprint = '' AND host = ? AND https_port = ?`, fp, id, host, port)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// SetTLSFingerprint pins (or clears, with "") the BMC certificate fingerprint.
func (s *Store) SetTLSFingerprint(ctx context.Context, id int64, fp string) error {
	return s.execOne(ctx, "UPDATE devices SET tls_fingerprint = ? WHERE id = ?", fp, id)
}

// DeleteDevices removes devices by id.
func (s *Store) DeleteDevices(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM devices WHERE id IN ("+placeholders(len(ids))+")", int64Args(ids)...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SaveInventory caches the JSON inventory of a device.
func (s *Store) SaveInventory(ctx context.Context, deviceID int64, data string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO inventory (device_id, data, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`, deviceID, data, now())
	return err
}

// Inventory returns the cached inventory JSON and its timestamp.
func (s *Store) Inventory(ctx context.Context, deviceID int64) (string, int64, error) {
	var data string
	var ts int64
	err := s.db.QueryRowContext(ctx, "SELECT data, updated_at FROM inventory WHERE device_id = ?", deviceID).Scan(&data, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return data, ts, err
}

// Group is a flat device group.
type Group struct {
	ID          int64
	Name        string
	DeviceCount int
}

// ListGroups returns all groups with their device counts.
func (s *Store) ListGroups(ctx context.Context) ([]*Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.name, COUNT(d.id) FROM device_groups g
		LEFT JOIN devices d ON d.group_id = g.id GROUP BY g.id ORDER BY g.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.DeviceCount); err != nil {
			return nil, err
		}
		out = append(out, &g)
	}
	return out, rows.Err()
}

// CreateGroup adds a group.
func (s *Store) CreateGroup(ctx context.Context, name string) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO device_groups (name, created_at) VALUES (?, ?)", name, now())
	if isUniqueErr(err) {
		return 0, fmt.Errorf("分组“%s”已存在", name)
	}
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RenameGroup renames a group.
func (s *Store) RenameGroup(ctx context.Context, id int64, name string) error {
	err := s.execOne(ctx, "UPDATE device_groups SET name = ? WHERE id = ?", name, id)
	if isUniqueErr(err) {
		return fmt.Errorf("分组“%s”已存在", name)
	}
	return err
}

// DeleteGroup deletes a group; its devices become ungrouped.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	return s.execOne(ctx, "DELETE FROM device_groups WHERE id = ?", id)
}
