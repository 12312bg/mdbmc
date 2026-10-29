package update

import (
	"fmt"
	"strconv"
	"strings"
)

// version is a parsed semantic version (MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]).
type version struct {
	nums [3]int
	pre  []string
}

func parseVersion(s string) (version, error) {
	var v version
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core = s[:i]
		v.pre = strings.Split(s[i+1:], ".")
		for _, id := range v.pre {
			if id == "" {
				return v, fmt.Errorf("版本号格式不正确: %q", s)
			}
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("版本号格式不正确: %q", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" {
			return v, fmt.Errorf("版本号格式不正确: %q", s)
		}
		v.nums[i] = n
	}
	return v, nil
}

// ValidVersion reports whether s is a semantic version such as 1.2.3 or
// 1.2.3-beta.1 (a leading "v" is allowed).
func ValidVersion(s string) bool {
	_, err := parseVersion(s)
	return err == nil
}

// IsPrerelease reports whether s has a pre-release part (1.2.0-beta.1).
func IsPrerelease(s string) bool {
	v, err := parseVersion(s)
	return err == nil && len(v.pre) > 0
}

// Compare orders two versions by semantic versioning precedence and returns
// -1, 0 or 1. A version that does not parse sorts before every valid one, so
// a build with an unexpected version string is still offered updates.
func Compare(a, b string) int {
	va, ea := parseVersion(a)
	vb, eb := parseVersion(b)
	switch {
	case ea != nil && eb != nil:
		return 0
	case ea != nil:
		return -1
	case eb != nil:
		return 1
	}
	for i := range va.nums {
		if c := cmpInt(va.nums[i], vb.nums[i]); c != 0 {
			return c
		}
	}
	// A pre-release sorts before the release it leads up to.
	switch {
	case len(va.pre) == 0 && len(vb.pre) == 0:
		return 0
	case len(va.pre) == 0:
		return 1
	case len(vb.pre) == 0:
		return -1
	}
	for i := 0; i < len(va.pre) && i < len(vb.pre); i++ {
		if c := cmpIdent(va.pre[i], vb.pre[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(va.pre), len(vb.pre))
}

// cmpIdent compares pre-release identifiers: numeric ones numerically and
// before alphanumeric ones, which compare as strings.
func cmpIdent(a, b string) int {
	na, ea := strconv.Atoi(a)
	nb, eb := strconv.Atoi(b)
	switch {
	case ea == nil && eb == nil:
		return cmpInt(na, nb)
	case ea == nil:
		return -1
	case eb == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
