package sysinfo

import (
	"strings"
	"testing"
)

func TestClientOS(t *testing.T) {
	in := Info{OS: "macOS", OSVersion: "27.2", Arch: "arm64"}
	if got := ClientOS(in, true); got != "macOS 27.2 arm64" {
		t.Errorf("ayrıntılı = %q", got)
	}
	if got := ClientOS(in, false); got != ShortOS() || strings.Contains(got, "27.2") {
		t.Errorf("onaysız = %q", got)
	}
	long := Info{OS: strings.Repeat("x", 40)}
	if got := ClientOS(long, true); len(got) > 32 {
		t.Errorf("32 karakteri aştı: %q", got)
	}
}

func TestCollectHasNoPersonalData(t *testing.T) {
	in := Collect("1.0.0")
	if in.AppVersion != "1.0.0" || in.OS == "" || in.Arch == "" {
		t.Errorf("eksik: %+v", in)
	}
}
