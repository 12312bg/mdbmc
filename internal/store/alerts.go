package store

import (
	"context"
	"strings"
)

// Alert is one entry of the alert inbox.
type Alert struct {
	ID         int64
	TS         int64
	DeviceID   int64
	DeviceName string
	Host       string
	Kind       string // offline, health, power
	Severity   string // critical, warning, ok (recovered)
	Title      string
	Detail     string
}

// AddAlert inserts a and sets its ID and time.
func (s *Store) AddAlert(ctx context.Context, a *Alert) error {
	if a.TS == 0 {
		a.TS = now()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO alerts (ts, device_id, device_name, host, kind, severity, title, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, a.TS, a.DeviceID, a.DeviceName, a.Host, a.Kind, a.Severity, a.Title, a.Detail)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

// PruneAlerts keeps the newest keep alerts.
func (s *Store) PruneAlerts(ctx context.Context, keep int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alerts WHERE id <= (SELECT id FROM alerts ORDER BY id DESC LIMIT 1 OFFSET ?)`, keep)
	return err
}

// AlertQuery filters the alert inbox.
type AlertQuery struct {
	Severity string
	Keyword  string
	Offset   int
	Limit    int
}

// ListAlerts returns matching alerts (newest first) and the total count.
func (s *Store) ListAlerts(ctx context.Context, q AlertQuery) ([]*Alert, int, error) {
	var where []string
	var args []any
	if q.Severity != "" {
		where = append(where, "severity = ?")
		args = append(args, q.Severity)
	}
	if q.Keyword != "" {
		where = append(where, "(device_name LIKE ? OR host LIKE ? OR title LIKE ? OR detail LIKE ?)")
		kw := "%" + q.Keyword + "%"
		args = append(args, kw, kw, kw, kw)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM alerts"+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, device_id, device_name, host, kind, severity, title, detail FROM alerts`+
		cond+" ORDER BY id DESC LIMIT ? OFFSET ?", append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.TS, &a.DeviceID, &a.DeviceName, &a.Host, &a.Kind, &a.Severity, &a.Title, &a.Detail); err != nil {
			return nil, 0, err
		}
		out = append(out, &a)
	}
	return out, total, rows.Err()
}

// AlertCounts returns the newest alert ID and how many alerts are newer
// than afterID.
func (s *Store) AlertCounts(ctx context.Context, afterID int64) (maxID int64, newer int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0), COUNT(CASE WHEN id > ? THEN 1 END) FROM alerts`, afterID).Scan(&maxID, &newer)
	return
}

// AlertPrefs returns a user's inbox preferences: whether new alerts pop up,
// and the newest alert ID the user has read.
func (s *Store) AlertPrefs(ctx context.Context, userID int64) (popup bool, readID int64, err error) {
	var p int
	err = s.db.QueryRowContext(ctx, `SELECT alert_popup, alert_read_id FROM users WHERE id = ?`, userID).Scan(&p, &readID)
	return p != 0, readID, err
}

// SetAlertPopup turns a user's alert pop-ups on or off.
func (s *Store) SetAlertPopup(ctx context.Context, userID int64, on bool) error {
	return s.execOne(ctx, `UPDATE users SET alert_popup = ? WHERE id = ?`, boolInt(on), userID)
}

// SetAlertReadID marks the alerts up to id as read for a user.
func (s *Store) SetAlertReadID(ctx context.Context, userID, id int64) error {
	return s.execOne(ctx, `UPDATE users SET alert_read_id = MAX(alert_read_id, ?) WHERE id = ?`, id, userID)
}
