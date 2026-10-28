package auth

import "testing"

func TestRoleMatrix(t *testing.T) {
	cases := []struct {
		role Role
		perm Permission
		want bool
	}{
		{RoleAdmin, PermUserManage, true},
		{RoleAdmin, PermAuditView, true},
		{RoleOperator, PermPowerControl, true},
		{RoleOperator, PermDeviceManage, true},
		{RoleOperator, PermCredentialReveal, true},
		{RoleOperator, PermSELClear, true},
		{RoleOperator, PermKVM, true},
		{RoleOperator, PermUserManage, false},
		{RoleOperator, PermSettingsManage, false},
		{RoleViewer, PermDeviceView, true},
		{RoleViewer, PermWebOpen, true},
		{RoleViewer, PermWebAutoLogin, false},
		{RoleViewer, PermKVM, false},
		{RoleViewer, PermPowerControl, false},
		{RoleViewer, PermCredentialReveal, false},
		{Role("bogus"), PermDeviceView, false},
	}
	for _, c := range cases {
		if got := c.role.Can(c.perm); got != c.want {
			t.Errorf("%s.Can(%s) = %v, want %v", c.role, c.perm, got, c.want)
		}
	}
}

func TestManagerRequireAndThrottle(t *testing.T) {
	m := NewManager()
	if _, err := m.Require(PermDeviceView); err != ErrNotLoggedIn {
		t.Fatalf("want ErrNotLoggedIn, got %v", err)
	}
	s, err := NewSession(1, "bob", "", RoleViewer, make([]byte, 32), false)
	if err != nil {
		t.Fatal(err)
	}
	m.Set(s)
	if _, err := m.Require(PermDeviceView); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Require(PermPowerControl); err != ErrForbidden {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	m.Clear()
	if m.Current() != nil {
		t.Fatal("session not cleared")
	}
	if _, err := s.DataKey(); err != ErrNotLoggedIn {
		t.Fatal("data key must not be readable after logout")
	}

	s2, _ := NewSession(2, "new", "", RoleAdmin, make([]byte, 32), true)
	m.Set(s2)
	if _, err := m.Require(); err != ErrMustChangePassword {
		t.Fatalf("want ErrMustChangePassword, got %v", err)
	}
	s2.PasswordChanged()
	if _, err := m.Require(PermUserManage); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < maxFailures; i++ {
		if err := m.CheckThrottle("Alice"); err != nil {
			t.Fatalf("throttled too early at %d", i)
		}
		m.RecordFailure("alice")
	}
	if err := m.CheckThrottle("ALICE"); err == nil {
		t.Fatal("expected throttle after max failures")
	}
	m.RecordSuccess("alice")
	if err := m.CheckThrottle("alice"); err != nil {
		t.Fatal("success should reset throttle")
	}
}
