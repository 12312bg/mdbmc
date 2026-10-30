package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/store"
)

// fakeDialogs answers the file dialogs with fixed paths.
type fakeDialogs struct{ save, open string }

func (f fakeDialogs) SaveFile(string, string, string, string) (string, error) { return f.save, nil }
func (f fakeDialogs) OpenFile(string, string, string) (string, error)         { return f.open, nil }

func TestExportAssets(t *testing.T) {
	h := newHarness(t)
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	ctx := context.Background()
	gid, err := h.app.store.CreateGroup(ctx, "机房 A")
	must(t, err)
	_, err = h.devices.Create(DeviceInput{Name: "web01", Host: "127.0.0.1", IPMIPort: 1, HTTPSPort: 1, GroupID: gid, Username: "ADMIN", Password: "SuperSecret-Pw"})
	must(t, err)
	devs, _ := h.app.store.ListDevices(ctx)
	inv, _ := json.Marshal(bmc.Inventory{
		Memory: []bmc.MemoryDIMM{{Slot: "DIMM_A1", CapacityMiB: 32768, Type: "DDR5", Serial: "M123"}},
		Drives: []bmc.Drive{{Name: "Disk 0", Model: "SAMSUNG", CapacityBytes: 960197124096, Serial: "S456"}},
	})
	must(t, h.app.store.SaveInventory(ctx, devs[0].ID, string(inv)))

	path := filepath.Join(t.TempDir(), "assets.xlsx")
	h.app.SetFileDialogs(fakeDialogs{save: path})
	got, err := h.devices.ExportAssets()
	must(t, err)
	if got != path {
		t.Fatalf("saved to %q", got)
	}
	f, err := excelize.OpenFile(path)
	must(t, err)
	defer f.Close()
	if sheets := f.GetSheetList(); strings.Join(sheets, ",") != "设备,内存,磁盘" {
		t.Fatalf("sheets %v", sheets)
	}
	rows, _ := f.GetRows("设备")
	if len(rows) != 2 || rows[0][0] != "名称" || rows[1][0] != "web01" || rows[1][5] != "机房 A" || rows[1][15] != "ADMIN" {
		t.Fatalf("device rows %q", rows)
	}
	mem, _ := f.GetRows("内存")
	disk, _ := f.GetRows("磁盘")
	if mem[1][3] != "32" || disk[1][4] != "960.2" || disk[1][8] != "S456" {
		t.Fatalf("inventory rows %q %q", mem, disk)
	}
	for _, sh := range f.GetSheetList() {
		rows, _ := f.GetRows(sh)
		for _, r := range rows {
			if strings.Contains(strings.Join(r, "|"), "SuperSecret") {
				t.Fatal("password exported")
			}
		}
	}
	page, _ := h.admin.ListAudit(AuditQuery{Keyword: "device.export"})
	if page.Total != 1 {
		t.Fatal("export not audited")
	}

	// Cancelling the dialog writes nothing.
	h.app.SetFileDialogs(fakeDialogs{})
	if got, err := h.devices.ExportAssets(); err != nil || got != "" {
		t.Fatalf("cancelled export: %q %v", got, err)
	}
}

func TestImportDevices(t *testing.T) {
	h := newHarness(t)
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	must(t, h.admin.CreateUser(UserInput{Username: "ro", Role: "viewer", Password: "Viewer@123"}))
	must(t, h.creds.Create(CredentialInput{Name: "默认", Username: "ADMIN", Password: "tpl-pass"}))
	_, err = h.devices.Create(DeviceInput{Name: "old", Host: "127.0.0.2", IPMIPort: 1, HTTPSPort: 1, Username: "u"})
	must(t, err)

	dir := t.TempDir()
	tpl := filepath.Join(dir, "template.xlsx")
	h.app.SetFileDialogs(fakeDialogs{save: tpl})
	_, err = h.devices.ImportTemplate()
	must(t, err)

	// Fill in the template like a user would.
	f, err := excelize.OpenFile(tpl)
	must(t, err)
	for i, r := range [][]any{
		{"web01", "127.0.0.3", 1, 1, "自动", "机房 B", "", "ADMIN", "pw-web01", "前端"},
		{"", "127.0.0.4", 1, 1, "仅 IPMI", "", "默认", "", "", ""},
		{"dup", "127.0.0.2", 1, 1, "", "", "", "u", "p", ""},          // already managed
		{"again", "127.0.0.3", 1, 1, "", "", "", "u", "p", ""},        // same as row 2
		{"nocred", "127.0.0.5", 1, 1, "", "", "不存在", "", "", ""},      // unknown template
		{"badport", "127.0.0.6", "abc", 1, "", "", "", "u", "p", ""},  // not a number
		{"badproto", "127.0.0.7", 1, 1, "SNMP", "", "", "u", "p", ""}, // unknown protocol
		{"nouser", "127.0.0.8", 1, 1, "", "", "", "", "", ""},         // no credentials
		{"", "", nil, nil, nil, nil, nil, nil, nil, nil},              // empty row: ignored
		{"withport", "127.0.0.9:443", 1, 1, "", "", "", "u", "p", ""}, // port in the address
	} {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		row := r
		must(t, f.SetSheetRow(importSheet, cell, &row))
	}
	data := filepath.Join(dir, "devices.xlsx")
	must(t, f.SaveAs(data))
	f.Close()

	h.app.SetFileDialogs(fakeDialogs{open: data})
	p, err := h.devices.PreviewImport()
	must(t, err)
	status := map[int]string{}
	for _, r := range p.Rows {
		status[r.Row] = r.Status
		if r.Row == 3 && (r.Name != "127.0.0.4" || r.Protocol != "ipmi" || r.Credential != "默认") {
			t.Fatalf("row 3 %+v", r)
		}
	}
	want := map[int]string{2: "ok", 3: "ok", 4: "skip", 5: "error", 6: "error", 7: "error", 8: "error", 9: "error", 11: "error"}
	for row, s := range want {
		if status[row] != s {
			t.Errorf("row %d: %s, want %s (%+v)", row, status[row], s, p.Rows)
		}
	}
	if p.OK != 2 || p.Skip != 1 || p.Errors != 6 || len(p.Rows) != 9 || strings.Join(p.NewGroups, ",") != "机房 B" {
		t.Fatalf("preview %+v", p)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "pw-web01") {
		t.Fatal("password sent to the window")
	}

	if _, err := h.devices.Import("wrong"); err == nil {
		t.Fatal("import with a wrong token")
	}
	p, err = h.devices.PreviewImport() // the wrong token did not consume it, but start over anyway
	must(t, err)
	res, err := h.devices.Import(p.Token)
	must(t, err)
	if res.Added != 2 || len(res.Failed) != 0 {
		t.Fatalf("result %+v", res)
	}
	if _, err := h.devices.Import(p.Token); err == nil {
		t.Fatal("same preview imported twice")
	}
	ctx := context.Background()
	devs, _ := h.app.store.ListDevices(ctx)
	byHost := map[string]*store.Device{}
	for _, d := range devs {
		byHost[d.Host] = d
	}
	web, ipmiOnly := byHost["127.0.0.3"], byHost["127.0.0.4"]
	if web == nil || ipmiOnly == nil || len(devs) != 3 {
		t.Fatalf("devices %+v", devs)
	}
	sess := h.app.auth.Current()
	if pw, err := sess.Box().OpenString(web.PasswordEnc, purposeDevicePassword); err != nil || pw != "pw-web01" || web.Note != "前端" {
		t.Fatalf("web01: %q %v", pw, err)
	}
	groups, _ := h.app.store.ListGroups(ctx)
	if len(groups) != 1 || groups[0].Name != "机房 B" || web.GroupID != groups[0].ID {
		t.Fatalf("groups %+v", groups)
	}
	if ipmiOnly.CredentialID == 0 || ipmiOnly.Protocol != "ipmi" || len(ipmiOnly.PasswordEnc) != 0 {
		t.Fatalf("templated device %+v", ipmiOnly)
	}

	// Viewers export but do not import.
	h.authS.Logout()
	_, err = h.authS.Login("ro", "Viewer@123")
	must(t, err)
	must(t, h.authS.ChangePassword("Viewer@123", "Viewer@456"))
	if _, err := h.devices.PreviewImport(); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer previewed an import: %v", err)
	}
	if _, err := h.devices.ImportTemplate(); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer saved the template: %v", err)
	}
	h.app.SetFileDialogs(fakeDialogs{save: filepath.Join(dir, "viewer.xlsx")})
	if _, err := h.devices.ExportAssets(); err != nil {
		t.Fatalf("viewer export: %v", err)
	}
}
