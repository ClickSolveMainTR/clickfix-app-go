package usecase

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"rapor.pdf":                     "rapor.pdf",
		"../../etc/passwd":              "passwd",
		`C:\Users\ali\foto.jpg`:         "foto.jpg",
		"a\r\nb\".txt":                  "ab.txt",
		"  spaced name.txt  ":           "spaced name.txt",
		"..":                            "",
		"":                              "",
		"/":                             "",
		"türkçe-şğüöç-İ.docx":           "türkçe-şğüöç-İ.docx",
		strings.Repeat("a", 400) + ".x": strings.Repeat("a", maxFilenameLen),
	}
	for in, want := range cases {
		got := sanitizeFilename(in)
		if in == strings.Repeat("a", 400)+".x" {
			if len(got) > maxFilenameLen {
				t.Fatalf("long name not capped: %d", len(got))
			}
			continue
		}
		if got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveContentType(t *testing.T) {
	t.Parallel()
	zip := []byte("PK\x03\x04rest")
	cases := []struct {
		name     string
		filename string
		declared string
		head     []byte
		want     string
	}{
		{"sniff beats declared", "a.pdf", "application/pdf", []byte("\x89PNG\r\n\x1a\n"), "image/png"},
		{"plain text uses declared csv", "a.csv", "text/csv", []byte("a,b\n1,2"), "text/csv"},
		{"plain text keeps text type over binary claim", "a.bin", "application/pdf", []byte("just text"), "text/plain"},
		{"octet falls back to extension", "a.pdf", "", []byte("%PDF-1.7"), "application/pdf"},
		{"docx zip keeps office type", "a.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", zip, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"docx zip by extension", "a.docx", "application/octet-stream", zip, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"plain zip", "a.zip", "application/zip", zip, "application/zip"},
	}
	for _, tc := range cases {
		if got := resolveContentType(tc.filename, tc.declared, tc.head); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestIsAllowedTypeAndInlineSafety(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"image/png", "image/webp", "video/mp4", "audio/mpeg", "application/pdf", "text/plain", "text/csv", "application/zip"} {
		if !isAllowedType(ct) {
			t.Errorf("%s should be allowed", ct)
		}
	}
	for _, ct := range []string{"text/html", "application/x-msdownload", "application/octet-stream", "application/javascript", "text/xml", ""} {
		if isAllowedType(ct) {
			t.Errorf("%s must not be allowed", ct)
		}
	}
	for ct, want := range map[string]bool{
		"image/png": true, "application/pdf": true, "text/plain; charset=utf-8": true,
		"image/svg+xml": false, "text/html": false, "application/zip": false, "": false,
	} {
		if got := IsInlineSafe(ct); got != want {
			t.Errorf("IsInlineSafe(%q) = %v, want %v", ct, got, want)
		}
	}
}

func TestLooksExecutable(t *testing.T) {
	t.Parallel()
	for name, head := range map[string][]byte{
		"pe": []byte("MZ\x90\x00"), "elf": []byte("\x7fELF\x02"), "macho64": {0xcf, 0xfa, 0xed, 0xfe},
	} {
		if !looksExecutable(head) {
			t.Errorf("%s not detected", name)
		}
	}
	if looksExecutable([]byte("%PDF-1.4")) || looksExecutable(nil) {
		t.Error("false positive")
	}
}

func TestTokenSigner(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	s := newTokenSigner("secret-a", func() time.Time { return now })
	claims := tokenClaims{Org: 5, Item: uuid.New(), Variant: "original", Exp: now.Add(time.Minute).Unix()}
	token := s.sign(claims)

	got, err := s.verify(token)
	if err != nil || got != claims {
		t.Fatalf("verify = %+v, %v", got, err)
	}
	if _, err := newTokenSigner("secret-b", func() time.Time { return now }).verify(token); err == nil {
		t.Fatal("token verified under another secret")
	}
	parts := strings.Split(token, ".")
	if _, err := s.verify(parts[0] + "." + parts[1] + "." + parts[1]); err == nil {
		t.Fatal("bad signature accepted")
	}
	for _, bad := range []string{"", "v1", "v2.a.b", "v1.a", token + ".extra"} {
		if _, err := s.verify(bad); err == nil {
			t.Fatalf("malformed token %q accepted", bad)
		}
	}
	late := newTokenSigner("secret-a", func() time.Time { return now.Add(time.Minute) })
	if _, err := late.verify(token); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestBuildThumbnail(t *testing.T) {
	t.Parallel()
	small := pngFixture(40, 20)
	out := buildThumbnail("image/png", small)
	if out == nil {
		t.Fatal("small png: no thumbnail")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 40 || cfg.Height != 20 {
		t.Fatalf("small image must keep its size: %v %+v", err, cfg)
	}

	tall := buildThumbnail("image/png", pngFixture(100, 1000))
	cfg, _ = jpeg.DecodeConfig(bytes.NewReader(tall))
	if cfg.Height != thumbMaxEdge || cfg.Width != 32 {
		t.Fatalf("tall image thumb = %dx%d", cfg.Width, cfg.Height)
	}

	if buildThumbnail("application/pdf", small) != nil {
		t.Fatal("pdf must not be thumbnailed")
	}
	if buildThumbnail("image/png", []byte("not an image")) != nil {
		t.Fatal("garbage must not be thumbnailed")
	}
}

func TestThumbnailFlattensTransparencyOnWhite(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4)) // fully transparent
	dst := scaleToFit(img, thumbMaxEdge)
	if c := dst.RGBAAt(1, 1); c != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("transparent pixel = %+v, want opaque white", c)
	}
}

func pngFixture(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: 10, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = encodePNG(&buf, img)
	return buf.Bytes()
}
