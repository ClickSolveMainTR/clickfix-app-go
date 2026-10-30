package mail

import (
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

func TestRenderLayoutEscapesUserInput(t *testing.T) {
	text, html, err := RenderLayout(Layout{
		Title:     `<script>alert(1)</script> Ali`,
		Body:      "Merhaba <b>Ali</b>,\n\nNot: <img src=x onerror=alert(1)>",
		ActionURL: "javascript:alert(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script>alert", "<b>Ali</b>", "<img src=x", "javascript:"} {
		if strings.Contains(html, bad) {
			t.Fatalf("html contains unescaped %q", bad)
		}
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("title not escaped")
	}
	if strings.Contains(html, "class=\"cf-btn\"") {
		t.Fatal("unsafe action URL rendered a button")
	}
	if !strings.Contains(text, "Merhaba <b>Ali</b>,") {
		t.Fatalf("plain text must keep the body as written: %q", text)
	}
}

func TestRenderLayoutBlocks(t *testing.T) {
	body := "Merhaba Ayşe,\n\nTür: Ürün demosu\nTarih: 4 Kasım 2026 14:00\nYer: Google Meet\n\n" +
		"Google Takvim: https://calendar.example.com/x?a=1&b=2\n\n" +
		"Giriş kodunuz: 482913\nKod 10 dakika geçerlidir.\n\nDetay için https://app.example.com/a."
	_, html, err := RenderLayout(Layout{Lang: "en", Eyebrow: "Appointment", Title: "Confirmed", Body: body,
		ActionURL: "https://app.example.com/manage", PreferencesURL: "https://app.example.com/profile"})
	if err != nil {
		t.Fatal(err)
	}
	checks := []string{
		`<td class="cf-softmuted" valign="top" width="130"`, // details table
		">Tür</td>", ">Google Meet</td>",
		`href="https://calendar.example.com/x?a=1&amp;b=2"`, ">Google Takvim</a>",
		">482913</div>",                    // code box
		`href="https://app.example.com/a"`, // trailing dot trimmed
		`class="cf-btn"`, ">Open</a>",      // default en label
		"Notification preferences", "cid:" + LogoCID,
		`lang="en"`, "<!--[if mso]>",
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
}

func TestLoneLabelValueStaysText(t *testing.T) {
	blocks := parseBody("Not: yarın gelin")
	if len(blocks) != 1 || blocks[0].Kind != "text" {
		t.Fatalf("got %+v", blocks)
	}
}

func TestBrandedMessageIsMultipart(t *testing.T) {
	msg, err := Branded([]string{"a@example.com"}, Layout{Title: "Merhaba", Body: "Satır bir\nSatır iki", ActionURL: "https://x.test/a"})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("Subject: x\r\nMIME-Version: 1.0\r\n")
	if err := writeBody(&b, msg); err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/related" {
		t.Fatalf("top type %q %v", mt, err)
	}
	related := multipart.NewReader(m.Body, params["boundary"])
	alt, err := related.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	_, altParams, _ := mime.ParseMediaType(alt.Header.Get("Content-Type"))
	parts := multipart.NewReader(alt, altParams["boundary"])
	var types []string
	var texts []string
	for {
		p, err := parts.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		types = append(types, ct)
		raw, _ := io.ReadAll(quotedprintable.NewReader(p))
		texts = append(texts, string(raw))
	}
	if strings.Join(types, ",") != "text/plain,text/html" {
		t.Fatalf("alternative parts %v", types)
	}
	if !strings.Contains(texts[0], "Satır bir\r\nSatır iki") || !strings.Contains(texts[1], "<h1") {
		t.Fatalf("decoded parts: %q", texts)
	}
	img, err := related.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if img.Header.Get("Content-ID") != "<"+LogoCID+">" || img.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("inline headers %v", img.Header)
	}
	for _, ln := range strings.Split(b.String(), "\r\n") {
		if len(ln) > 998 {
			t.Fatalf("line exceeds 998 octets (%d)", len(ln))
		}
	}
}

func TestPlainMessageUnchanged(t *testing.T) {
	var b strings.Builder
	if err := writeBody(&b, Message{Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	if b.String() != "Content-Type: text/plain; charset=UTF-8\r\n\r\nhi" {
		t.Fatalf("got %q", b.String())
	}
}
