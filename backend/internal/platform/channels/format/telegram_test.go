package format

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTelegramEntities(t *testing.T) {
	// The emoji is two UTF-16 units: offsets after it shift by one.
	r := Telegram("🎉 **kalın** ve *eğik* `kod` [ad](https://ex.com/a_b)\n```\nx*y\n```")
	if want := "🎉 kalın ve eğik kod ad\nx*y"; r.Text != want {
		t.Fatalf("text %q want %q", r.Text, want)
	}
	want := []Entity{
		{Type: "bold", Offset: 3, Length: 5},
		{Type: "italic", Offset: 12, Length: 4},
		{Type: "code", Offset: 17, Length: 3},
		{Type: "text_link", Offset: 21, Length: 2, URL: "https://ex.com/a_b"},
		{Type: "pre", Offset: 24, Length: 3},
	}
	if !reflect.DeepEqual(r.Entities, want) {
		t.Fatalf("entities %+v\nwant %+v", r.Entities, want)
	}
	units := utf16.Encode([]rune(r.Text))
	if got := string(utf16.Decode(units[3:8])); got != "kalın" {
		t.Fatalf("bold range is %q", got)
	}
}

// Code inside bold must not nest: Telegram refuses code within another
// entity, so bold covers only the text around it.
func TestTelegramCodeDoesNotNest(t *testing.T) {
	r := Telegram("**a `b` c** ve ***d***")
	for _, e := range r.Entities {
		if e.Type == "code" {
			for _, o := range r.Entities {
				if o.Type != "code" && o.Offset < e.Offset+e.Length && e.Offset < o.Offset+o.Length {
					t.Fatalf("%+v overlaps %+v", o, e)
				}
			}
		}
	}
	if !strings.HasPrefix(r.Text, "a b c ve ") {
		t.Fatalf("text %q", r.Text)
	}
	if want := []Entity{{Type: "bold", Offset: 0, Length: 2}, {Type: "code", Offset: 2, Length: 1}, {Type: "bold", Offset: 3, Length: 2}}; !reflect.DeepEqual(r.Entities[:3], want) {
		t.Fatalf("entities %+v", r.Entities)
	}
}

func TestTelegramSplitKeepsFormatting(t *testing.T) {
	// One bold run longer than the limit: every part must stay bold and no
	// part may exceed the limit or cut a surrogate pair.
	src := "**" + strings.Repeat("kalın 🎉 metin ", 40) + "son**\nnormal satır"
	r := Telegram(src)
	parts := r.Split(100)
	if len(parts) < 5 {
		t.Fatalf("%d parts", len(parts))
	}
	joined := ""
	for i, p := range parts {
		if p.Len() > 100 {
			t.Fatalf("part %d has %d units", i, p.Len())
		}
		if strings.ContainsRune(p.Text, '�') || p.Text != strings.TrimSpace(p.Text) {
			t.Fatalf("part %d is broken or untrimmed: %q", i, p.Text)
		}
		if strings.Contains(p.Text, "*") {
			t.Fatalf("part %d carries markup: %q", i, p.Text)
		}
		for _, e := range p.Entities {
			if e.Offset < 0 || e.Length <= 0 || e.Offset+e.Length > p.Len() {
				t.Fatalf("part %d entity %+v out of range", i, e)
			}
		}
		if i < len(parts)-1 && (len(p.Entities) != 1 || p.Entities[0].Type != "bold" || p.Entities[0].Offset != 0 || p.Entities[0].Length != p.Len()) {
			t.Fatalf("part %d is not fully bold: %+v", i, p.Entities)
		}
		joined += strings.ReplaceAll(p.Text, " ", "")
	}
	if want := strings.ReplaceAll(strings.ReplaceAll(r.Text, " ", ""), "\n", ""); strings.ReplaceAll(joined, "\n", "") != want {
		t.Fatal("split lost text")
	}
	last := parts[len(parts)-1]
	if !strings.HasSuffix(last.Text, "normal satır") {
		t.Fatalf("last part %q", last.Text)
	}
}

// A message that is only a code fence must not become an empty text.
func TestTelegramOnlyFence(t *testing.T) {
	if r := Telegram("```"); r.Text != "```" || len(r.Entities) != 0 {
		t.Fatalf("%+v", r)
	}
	r := Telegram("***ikisi***")
	if r.Text != "ikisi" || len(r.Entities) != 2 || r.Entities[0].Length != 5 || r.Entities[1].Length != 5 {
		t.Fatalf("bold italic: %+v", r)
	}
}

func TestTelegramSplitShortAndBlank(t *testing.T) {
	if parts := Telegram("**kısa**").Split(4096); len(parts) != 1 || parts[0].Text != "kısa" || len(parts[0].Entities) != 1 {
		t.Fatalf("%+v", parts)
	}
	if parts := Telegram(" \n ").Split(4096); len(parts) != 0 {
		t.Fatalf("blank text gave %+v", parts)
	}
}
