//go:build darwin

package sysinfo

import "golang.org/x/sys/unix"

// osVersion, macOS ürün sürümüdür (ör. 27.2).
func osVersion() string {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return v
}
