//go:build windows

package capture

// Windows'ta ekran yakalama henüz yok. CF-235 Windows'u ölçmedi (makine
// yok); Windows Graphics Capture + Media Foundation H.264 yolu ayrı bir işte
// denenecek. O zamana kadar her çağrı açıkça "desteklenmiyor" der ki arayüz
// izin ya da ağ sorunu sanmasın.

// ScreenPermission: Windows'ta ekran kaydı izni yoktur, ama yakalama da
// yoktur.
func ScreenPermission() Permission { return Unsupported }

// RequestScreenPermission, ScreenPermission ile aynıdır.
func RequestScreenPermission() Permission { return Unsupported }

// MainDisplay desteklenmiyor.
func MainDisplay() (Display, error) { return Display{}, ErrUnsupported }

// Start desteklenmiyor.
func Start(Config, func(Frame), func(error)) (Stream, error) { return nil, ErrUnsupported }
