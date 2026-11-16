package ipc

import (
	"context"
	"encoding/json"
	"runtime"
)

// Info, version yönteminin döndürdüğü sidecar bilgisidir.
type Info struct {
	Version string `json:"version"`
	Mode    string `json:"mode"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// NewInfo, çalışan ikilinin bilgisini döndürür.
func NewInfo(version, mode string) Info {
	return Info{Version: version, Mode: mode, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// PingResult, ping yönteminin yanıtıdır.
type PingResult struct {
	Pong bool `json:"pong"`
}

// RegisterBuiltins, her modda bulunan ping ve version yöntemlerini kaydeder.
func RegisterBuiltins(s *Server, info Info) {
	s.Handle("ping", func(context.Context, json.RawMessage) (any, error) {
		return PingResult{Pong: true}, nil
	})
	s.Handle("version", func(context.Context, json.RawMessage) (any, error) {
		return info, nil
	})
}
