package capture

import "testing"

func TestOutSize(t *testing.T) {
	cases := []struct{ w, h, max, ow, oh int }{
		{2940, 1912, 1080, 1660, 1080},
		{1920, 1080, 1080, 1920, 1080},
		{1366, 767, 1080, 1366, 766},
		{3840, 2160, 1080, 1920, 1080},
		{3024, 1964, 0, 3024, 1964},
		{0, 100, 1080, 0, 0},
	}
	for _, c := range cases {
		if w, h := OutSize(c.w, c.h, c.max); w != c.ow || h != c.oh {
			t.Errorf("OutSize(%d,%d,%d) = %dx%d, beklenen %dx%d", c.w, c.h, c.max, w, h, c.ow, c.oh)
		}
	}
}

func TestBitrate(t *testing.T) {
	if b := Bitrate(1920, 1080, 30); b != 4976 {
		t.Errorf("1080p30 = %d", b)
	}
	if b := Bitrate(320, 240, 5); b != 1000 {
		t.Errorf("alt sınır = %d", b)
	}
	if b := Bitrate(7680, 4320, 60); b != 20000 {
		t.Errorf("üst sınır = %d", b)
	}
}
