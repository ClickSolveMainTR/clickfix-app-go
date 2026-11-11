package screenctx

import (
	"strings"
	"testing"
	"time"
)

func TestParseDropsQueryAndKeepsLastEvents(t *testing.T) {
	raw := []byte(`[{"name":"opened_invoice","properties":{"invoice":"F-1"}},{"name":"  "},{"name":"payment_failed"}]`)
	s := Parse("https://app.example.com/invoices/F-1?token=abc#x", raw, time.Unix(10, 0))
	if s == nil {
		t.Fatal("nil screen")
	}
	if s.PageURL != "https://app.example.com/invoices/F-1" || s.PagePath != "/invoices/F-1" {
		t.Fatalf("url not cleaned: %+v", s)
	}
	if s.LastEvent != "payment_failed" || len(s.Events) != 2 {
		t.Fatalf("events: %+v", s.Events)
	}
	c := s.Conditions()
	if c["page_path"] != "/invoices/F-1" || len(c["event_names"].([]any)) != 2 {
		t.Fatalf("conditions: %+v", c)
	}
	p := s.Prompt()
	if !strings.Contains(p, "untrusted") || strings.Contains(p, "token=abc") || !strings.Contains(p, "payment_failed") {
		t.Fatalf("prompt: %s", p)
	}
}

func TestParseEmptyAndUnsafeURL(t *testing.T) {
	if Parse("", nil, time.Time{}) != nil {
		t.Fatal("expected nil")
	}
	if s := Parse("javascript:alert(1)", []byte(`[{"name":"x"}]`), time.Time{}); s == nil || s.PageURL != "" {
		t.Fatalf("unsafe url kept: %+v", s)
	}
	var nilScreen *Screen
	if nilScreen.Prompt() != "" || len(nilScreen.Conditions()) != 0 {
		t.Fatal("nil screen should be empty")
	}
}
