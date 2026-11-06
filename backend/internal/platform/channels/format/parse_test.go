package format

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// tree converts blocks to the widget's token shape (markdown.ts), as JSON
// decodes it.
func tree(blocks []Block) []any {
	out := []any{}
	for _, b := range blocks {
		switch b.T {
		case Paragraph:
			out = append(out, map[string]any{"t": "p", "c": inlines(b.C)})
		case Pre:
			out = append(out, map[string]any{"t": "pre", "v": b.V})
		case List:
			items := []any{}
			for _, it := range b.Items {
				items = append(items, inlines(it))
			}
			out = append(out, map[string]any{"t": "list", "ordered": b.Ordered, "items": items})
		}
	}
	return out
}

func inlines(nodes []Inline) []any {
	out := []any{}
	for _, n := range nodes {
		switch n.T {
		case Text, Code:
			out = append(out, map[string]any{"t": n.T, "v": n.V})
		case Break:
			out = append(out, map[string]any{"t": "br"})
		case Link:
			out = append(out, map[string]any{"t": "link", "href": n.Href, "c": inlines(n.C)})
		default:
			out = append(out, map[string]any{"t": n.T, "c": inlines(n.C)})
		}
	}
	return out
}

// The widget (frontend/widget-sdk/tests/markdown-cases.test.mjs) asserts
// the same file against markdown.ts.
func TestParseMatchesWidget(t *testing.T) {
	raw, err := os.ReadFile("testdata/markdown_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string `json:"name"`
		Src    string `json:"src"`
		Blocks []any  `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 30 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, c := range cases {
		if got := tree(Parse(c.Src)); !reflect.DeepEqual(got, c.Blocks) {
			g, _ := json.Marshal(got)
			w, _ := json.Marshal(c.Blocks)
			t.Errorf("%s\n got: %s\nwant: %s", c.Name, g, w)
		}
	}
}

// FuzzParse checks the parser and every renderer never panic and that
// entity ranges stay inside the text.
func FuzzParse(f *testing.F) {
	for _, s := range []string{sample, "**", "*_`[](", "```", "- \n1. ", "[a](https://x)", "😀*a*😀"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, fn := range []func(string) string{Plain, Strip, Speech, WhatsApp, Slack, GoogleChat, Teams, HTML} {
			fn(s)
		}
		r := Telegram(s)
		for _, part := range append(r.Split(64), r) {
			for _, e := range part.Entities {
				if e.Offset < 0 || e.Length <= 0 || e.Offset+e.Length > part.Len() {
					t.Fatalf("entity %+v outside %q", e, part.Text)
				}
			}
		}
	})
}
