package update

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.10.0", "1.9.9", 1},
		{"2.0.0", "10.0.0", -1},
		{"1.0.0-beta.1", "1.0.0", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-rc.1", "1.0.0-beta.11", 1},
		{"1.0.0+build.5", "1.0.0", 0},
		// CI builds of untagged commits: 0.1.1-<commit>.
		{"0.1.1-abc1234", "0.1.1", -1},
		{"0.1.1-abc1234", "0.1.0", 1},
		{"garbage", "0.0.1", -1},
		{"0.0.1", "garbage", 1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestValidVersion(t *testing.T) {
	for _, s := range []string{"1.2.3", "v0.1.0", "1.2.3-beta.1", "1.2.3+x"} {
		if !ValidVersion(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "1.2", "1.2.3.4", "1.x.3", "1.2.3-", "1.2.3-a..b", "-1.2.3"} {
		if ValidVersion(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
	if !IsPrerelease("1.2.0-beta.1") || IsPrerelease("1.2.0") {
		t.Error("IsPrerelease")
	}
}
