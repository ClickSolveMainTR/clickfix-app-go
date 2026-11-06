package model

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPreview(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"  Merhaba\n\n  dünya\t ": "Merhaba dünya",
		"tek":                     "tek",
		"satır\r\nsonu":           "satır sonu",
		// CF-177: formatting marks are dropped, plain text is kept.
		"**Kargonuz** *yolda*: [takip](https://ex.com/t_1), 2*3*4 ve dosya_adi.pdf": "Kargonuz yolda: takip, 2*3*4 ve dosya_adi.pdf",
	}
	for in, want := range cases {
		if got := Preview(in); got != want {
			t.Errorf("Preview(%q) = %q, want %q", in, got, want)
		}
	}
	long := Preview(strings.Repeat("ş", 500))
	if utf8.RuneCountInString(long) != PreviewLength || !strings.HasSuffix(long, "…") {
		t.Fatalf("long preview = %d runes", utf8.RuneCountInString(long))
	}
}

func TestValid(t *testing.T) {
	if !Valid(StatusSnoozed, Statuses) || Valid("solved", Statuses) {
		t.Fatal("status validation")
	}
}
