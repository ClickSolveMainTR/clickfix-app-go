//go:build !darwin

package sysinfo

// osVersion: Windows sürümü (RtlGetVersion) Windows yakalamasıyla birlikte
// eklenecek; şimdilik boş.
func osVersion() string { return "" }
