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
		// A contact's marks are kept as typed.
		"_abc_ ve *x*": "_abc_ ve *x*",
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

// CF-177: marks are dropped only from what an agent or the AI wrote.
func TestPreviewOutbound(t *testing.T) {
	in := "**Kargonuz** *yolda*: [takip](https://ex.com/t_1), 2*3*4 ve dosya_adi.pdf"
	if got, want := PreviewOutbound(in), "Kargonuz yolda: takip, 2*3*4 ve dosya_adi.pdf"; got != want {
		t.Errorf("PreviewOutbound = %q, want %q", got, want)
	}
}

func TestValid(t *testing.T) {
	if !Valid(StatusSnoozed, Statuses) || Valid("solved", Statuses) {
		t.Fatal("status validation")
	}
}
