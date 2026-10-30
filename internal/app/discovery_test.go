package app

import (
	"context"
	"testing"
)

func TestAddDiscoveredGroups(t *testing.T) {
	h := newHarness(t)
	_, err := h.authS.Setup("admin", "", "Admin@12345")
	must(t, err)
	ctx := context.Background()
	dell, err := h.app.store.CreateGroup(ctx, "Dell")
	must(t, err)
	lab, err := h.app.store.CreateGroup(ctx, "机房 A")
	must(t, err)

	disc := &DiscoveryService{a: h.app}
	res, err := disc.AddDiscovered(AddDiscoveredRequest{
		Hosts: []DiscoveredHost{
			{Host: "127.0.0.11", Vendor: "dell", Group: "dell"},             // existing group, any case
			{Host: "127.0.0.12", Vendor: "supermicro", Group: "Supermicro"}, // created
			{Host: "127.0.0.13", Vendor: "supermicro", Group: "Supermicro"}, // created once
			{Host: "127.0.0.14", Vendor: "generic"},                         // the request's group
		},
		IPMIPort: 1, HTTPSPort: 1, GroupID: lab, Username: "u", Password: "p",
	})
	must(t, err)
	if res.Added != 4 || len(res.Skipped) != 0 {
		t.Fatalf("result %+v", res)
	}
	groups, err := h.app.store.ListGroups(ctx)
	must(t, err)
	var sm int64
	for _, g := range groups {
		if g.Name == "Supermicro" {
			sm = g.ID
		}
	}
	if len(groups) != 3 || sm == 0 {
		t.Fatalf("groups %+v", groups)
	}
	devs, err := h.app.store.ListDevices(ctx)
	must(t, err)
	want := map[string]int64{"127.0.0.11": dell, "127.0.0.12": sm, "127.0.0.13": sm, "127.0.0.14": lab}
	for _, d := range devs {
		if d.GroupID != want[d.Host] {
			t.Errorf("%s in group %d, want %d", d.Host, d.GroupID, want[d.Host])
		}
	}
}
