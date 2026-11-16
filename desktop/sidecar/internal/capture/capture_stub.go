//go:build !windows && !(darwin && cgo)

package capture

// Linux (Destek uygulaması v1'de yok) ve cgo'suz macOS derlemeleri.

// ScreenPermission desteklenmiyor.
func ScreenPermission() Permission { return Unsupported }

// RequestScreenPermission desteklenmiyor.
func RequestScreenPermission() Permission { return Unsupported }

// MainDisplay desteklenmiyor.
func MainDisplay() (Display, error) { return Display{}, ErrUnsupported }

// Start desteklenmiyor.
func Start(Config, func(Frame), func(error)) (Stream, error) { return nil, ErrUnsupported }
