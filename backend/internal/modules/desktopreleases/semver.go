package desktopreleases

import (
	"errors"
	"strconv"
	"strings"
)

// Version is a semantic version (semver 2.0.0). Build metadata (+...) is
// accepted and ignored, as semver precedence requires.
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string
}

// ErrBadVersion is returned for a string that is not semver.
var ErrBadVersion = errors.New("desktopreleases: invalid semantic version")

// ParseVersion parses "1.2.3", "1.2.3-beta.1" or "v1.2.3+build.5".
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" || len(s) > 64 {
		return Version{}, ErrBadVersion
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		if !validIdents(s[i+1:], false) {
			return Version{}, ErrBadVersion
		}
		s = s[:i]
	}
	var pre []string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		if !validIdents(s[i+1:], true) {
			return Version{}, ErrBadVersion
		}
		pre = strings.Split(s[i+1:], ".")
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, ErrBadVersion
	}
	var nums [3]uint64
	for i, p := range parts {
		n, ok := numeric(p)
		if !ok {
			return Version{}, ErrBadVersion
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: pre}, nil
}

// String renders the version without build metadata.
func (v Version) String() string {
	s := strconv.FormatUint(v.Major, 10) + "." + strconv.FormatUint(v.Minor, 10) + "." + strconv.FormatUint(v.Patch, 10)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}

// Compare returns -1, 0 or 1 by semver precedence: a pre-release sorts
// before its release (1.0.0-beta.2 < 1.0.0) and numeric identifiers compare
// as numbers (beta.10 > beta.9).
func Compare(a, b Version) int {
	for _, d := range [3][2]uint64{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if d[0] != d[1] {
			return cmp(d[0] < d[1])
		}
	}
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}
	for i := 0; i < len(a.Pre) && i < len(b.Pre); i++ {
		x, y := a.Pre[i], b.Pre[i]
		if x == y {
			continue
		}
		xn, xok := numeric(x)
		yn, yok := numeric(y)
		switch {
		case xok && yok:
			return cmp(xn < yn)
		case xok:
			return -1 // numeric identifiers sort before alphanumeric ones
		case yok:
			return 1
		default:
			return cmp(x < y)
		}
	}
	switch {
	case len(a.Pre) < len(b.Pre):
		return -1
	case len(a.Pre) > len(b.Pre):
		return 1
	}
	return 0
}

func cmp(less bool) int {
	if less {
		return -1
	}
	return 1
}

// numeric parses a semver numeric identifier (no leading zeros).
func numeric(s string) (uint64, bool) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

func validIdents(s string, pre bool) bool {
	if s == "" {
		return false
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		allDigits := true
		for _, c := range id {
			switch {
			case c >= '0' && c <= '9':
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '-':
				allDigits = false
			default:
				return false
			}
		}
		if pre && allDigits && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}
