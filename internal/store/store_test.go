package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDeviceGroupCredentialLifecycle(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	gid, err := s.CreateGroup(ctx, "机房A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGroup(ctx, "机房a"); err == nil {
		t.Fatal("group names should be unique case-insensitively")
	}
	cred := &Credential{Name: "default", Username: "ADMIN", PasswordEnc: []byte{1}}
	if err := s.CreateCredential(ctx, cred); err != nil {
		t.Fatal(err)
	}

	d := &Device{Name: "node1", Host: "10.0.0.1", IPMIPort: 623, HTTPSPort: 443, Protocol: "auto", GroupID: gid, CredentialID: cred.ID}
	if err := s.CreateDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	dup := *d
	if err := s.CreateDevice(ctx, &dup); err == nil {
		t.Fatal("duplicate endpoint should fail")
	}

	if err := s.DeleteCredential(ctx, cred.ID); err == nil {
		t.Fatal("deleting a credential in use should fail")
	}

	groups, err := s.ListGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0].DeviceCount != 1 {
		t.Fatalf("groups = %+v, err %v", groups, err)
	}
	if err := s.DeleteGroup(ctx, gid); err != nil {
		t.Fatal(err)
	}
	got, err := s.DeviceByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GroupID != 0 {
		t.Fatalf("device should be ungrouped after group delete, got %d", got.GroupID)
	}

	if err := s.UpdateDeviceStatus(ctx, d.ID, Status{PowerState: "on", Health: "ok", Reachable: true, CheckedAt: 5}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveInventory(ctx, d.ID, `{"a":1}`); err != nil {
		t.Fatal(err)
	}
	n, err := s.DeleteDevices(ctx, []int64{d.ID})
	if err != nil || n != 1 {
		t.Fatalf("delete: %d %v", n, err)
	}
	if _, _, err := s.Inventory(ctx, d.ID); err != ErrNotFound {
		t.Fatalf("inventory should cascade, got %v", err)
	}
	if err := s.DeleteCredential(ctx, cred.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAuditQuery(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	for i, a := range []string{"login", "power.on", "power.off"} {
		if err := s.AddAudit(ctx, &AuditEntry{TS: int64(100 + i), Username: "admin", Action: a, Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	rows, total, err := s.ListAudit(ctx, AuditQuery{Keyword: "power"})
	if err != nil || total != 2 || len(rows) != 2 || rows[0].Action != "power.off" {
		t.Fatalf("rows=%v total=%d err=%v", rows, total, err)
	}
}
