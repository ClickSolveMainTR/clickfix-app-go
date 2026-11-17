// Package sysinfo, destek kaydı için en az sistem bilgisini toplar:
// işletim sistemi, sürümü, mimari, uygulama sürümü, birincil ekran
// çözünürlüğü. Kullanıcı adı, bilgisayar adı, IP ya da donanım kimliği
// toplanmaz. Ayrıntılı bilgi API'ye yalnız müşteri onaylarsa gider
// (session.resolve share_system_info); onaysızken yalnız işletim sisteminin
// genel adı (macos, windows) gider.
package sysinfo

import (
	"runtime"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
)

// Info, sysinfo.get yanıtıdır.
type Info struct {
	OS         string           `json:"os"`
	OSVersion  string           `json:"os_version"`
	Arch       string           `json:"arch"`
	AppVersion string           `json:"app_version"`
	Screen     *capture.Display `json:"screen"`
}

// Collect bilgiyi toplar.
func Collect(appVersion string) Info {
	in := Info{OS: osName(runtime.GOOS), OSVersion: osVersion(), Arch: runtime.GOARCH, AppVersion: appVersion}
	if d, err := capture.MainDisplay(); err == nil {
		in.Screen = &d
	}
	return in
}

// ShortOS, onay olmadan gönderilen genel addır (kullanıcı ajanında da durur).
func ShortOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	default:
		return runtime.GOOS
	}
}

// ClientOS, API'nin client_os alanıdır (en çok 32 karakter). detailed
// yalnız müşteri onayıyla true olur.
func ClientOS(in Info, detailed bool) string {
	if !detailed {
		return ShortOS()
	}
	s := strings.TrimSpace(in.OS + " " + in.OSVersion + " " + in.Arch)
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

func osName(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return goos
}
