package desktopreleases

import "testing"

func TestParseVersion(t *testing.T) {
	good := map[string]string{
		"0.1.0":             "0.1.0",
		"v1.2.3":            "1.2.3",
		"1.2.3-beta.1":      "1.2.3-beta.1",
		"1.2.3-rc.1+b.5":    "1.2.3-rc.1",
		"10.20.30":          "10.20.30",
		"1.0.0-alpha-a.b-c": "1.0.0-alpha-a.b-c",
	}
	for in, want := range good {
		v, err := ParseVersion(in)
		if err != nil || v.String() != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %s", in, v, err, want)
		}
	}
	for _, in := range []string{"", "1", "1.2", "1.2.3.4", "01.2.3", "1.02.3", "1.2.3-", "1.2.3-beta..1", "1.2.3-01", "1.2.3+", "a.b.c", "1.2.3-ü", "1.2.3/../x"} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) accepted", in)
		}
	}
}

func TestCompare(t *testing.T) {
	// Ascending by semver precedence (semver.org §11 example plus numeric cases).
	order := []string{
		"0.9.0", "0.10.0", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
		"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0",
	}
	for i := range order {
		for j := range order {
			a, _ := ParseVersion(order[i])
			b, _ := ParseVersion(order[j])
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := Compare(a, b); got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", order[i], order[j], got, want)
			}
		}
	}
	a, _ := ParseVersion("1.0.0+build.1")
	b, _ := ParseVersion("1.0.0+build.2")
	if Compare(a, b) != 0 {
		t.Error("build metadata must not affect precedence")
	}
}
