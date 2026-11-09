package assistant

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
)

func TestDocumentTypes(t *testing.T) {
	ok := map[string][2]string{
		"api.md":      {"", "# API\nPOST /orders"},
		"spec.yaml":   {"application/octet-stream", "openapi: 3.0.0"},
		"sample.json": {"application/json", `{"id":1}`},
		"page":        {"text/html; charset=utf-8", "<p>Merhaba</p>"},
		"notes.txt":   {"text/plain", "düz metin"},
	}
	for name, v := range ok {
		if _, err := documentType(name, v[0], []byte(v[1])); err != nil {
			t.Fatalf("%s refused: %v", name, err)
		}
	}
	bad := map[string][2]string{
		"text.pdf":   {"application/pdf", "not a pdf at all"},
		"pdf.txt":    {"text/plain", "%PDF-1.7\n1 0 obj"},
		"image.png":  {"image/png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"},
		"fake.txt":   {"text/plain", "MZ\x90\x00\x03\x00\x00\x00"},
		"latin1.txt": {"text/plain", "caf\xe9"},
		"run.sh":     {"application/x-sh", "#!/bin/sh\nrm -rf /"},
	}
	for name, v := range bad {
		if _, err := documentType(name, v[0], []byte(v[1])); !errors.Is(err, ErrDocumentType) {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestExtractMasksAndStrips(t *testing.T) {
	page := `<html><head><title>x</title><script>steal()</script></head><body><h1>Sipariş API</h1>
<p>Webhook: https://shop.example.com/clickfix</p><p>api_key=sk_live_ABCDEFGHIJKLMNOPQRSTUVWX ve destek@shop.example.com</p>
<style>.a{}</style></body></html>`
	text, truncated, masked, err := extract(context.Background(), "text/html", []byte(page))
	if err != nil || truncated || !masked {
		t.Fatalf("%v %v %v", err, truncated, masked)
	}
	for _, leak := range []string{"steal()", "sk_live_ABCDEFGHIJKLMNOPQRSTUVWX", "destek@shop.example.com", ".a{}"} {
		if strings.Contains(text, leak) {
			t.Fatalf("leaked %q in %q", leak, text)
		}
	}
	if !strings.Contains(text, "Sipariş API") || !strings.Contains(text, "https://shop.example.com/clickfix") {
		t.Fatalf("lost content: %q", text)
	}
	long := strings.Repeat("abc ", MaxDocumentChars)
	text, truncated, _, _ = extract(context.Background(), "text/plain", []byte(long))
	if !truncated || len([]rune(text)) != MaxDocumentChars {
		t.Fatal("not truncated")
	}
	if _, _, _, err := extract(context.Background(), "text/html", []byte("<script>x()</script>")); !errors.Is(err, ErrDocumentEmpty) {
		t.Fatal("empty page accepted")
	}
}

// routes answers by URL without any network.
type routes map[string]*http.Response

func (rt routes) RoundTrip(r *http.Request) (*http.Response, error) {
	if res, ok := rt[r.URL.String()]; ok {
		res.Request = r
		return res, nil
	}
	return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func resp(status int, typ, body string, headers ...string) *http.Response {
	h := http.Header{"Content-Type": {typ}}
	for i := 0; i+1 < len(headers); i += 2 {
		h.Set(headers[i], headers[i+1])
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

func TestFetcherRefusesPrivateTargetsAndRedirects(t *testing.T) {
	// The default client: refused before any connection.
	for _, u := range []string{"http://docs.example.com/api", "https://127.0.0.1/x", "https://localhost/x", "https://10.0.0.8/x",
		"https://169.254.169.254/latest/meta-data", "https://metadata.internal/x", "https://user:pw@docs.example.com/x", "file:///etc/passwd", "gopher://x"} {
		var fe *FetchError
		if _, _, _, err := (Fetcher{}).Fetch(context.Background(), u); !errors.As(err, &fe) {
			t.Fatalf("%s: %v", u, err)
		}
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: routes{
		"https://docs.example.com/api":     resp(301, "text/html", "", "Location", "/v2/api"),
		"https://docs.example.com/v2/api":  resp(200, "text/markdown", "# API v2"),
		"https://docs.example.com/inward":  resp(302, "text/html", "", "Location", "https://10.0.0.8/admin"),
		"https://docs.example.com/loop":    resp(302, "text/html", "", "Location", "https://docs.example.com/loop"),
		"https://docs.example.com/big":     resp(200, "text/plain", strings.Repeat("x", MaxDocumentBytes+10)),
		"https://docs.example.com/missing": resp(500, "text/plain", "secret stack trace"),
	}}
	f := Fetcher{Client: client}
	name, typ, data, err := f.Fetch(context.Background(), "https://docs.example.com/api")
	if err != nil || name != "docs.example.com/v2/api" || typ != "text/markdown" || string(data) != "# API v2" {
		t.Fatalf("%q %q %q %v", name, typ, data, err)
	}
	var fe *FetchError
	if _, _, _, err := f.Fetch(context.Background(), "https://docs.example.com/inward"); !errors.As(err, &fe) {
		t.Fatalf("redirect inward: %v", err)
	}
	if _, _, _, err := f.Fetch(context.Background(), "https://docs.example.com/loop"); !errors.As(err, &fe) || !strings.Contains(fe.Message, "redirect") {
		t.Fatalf("redirect loop: %v", err)
	}
	if _, _, _, err := f.Fetch(context.Background(), "https://docs.example.com/big"); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("size: %v", err)
	}
	if _, _, _, err := f.Fetch(context.Background(), "https://docs.example.com/missing"); !errors.As(err, &fe) || strings.Contains(fe.Message, "stack") {
		t.Fatalf("error page: %v", err)
	}
	if got := cleanURL("https://docs.example.com/api?token=abc#x"); got != "https://docs.example.com/api" {
		t.Fatalf("clean url %q", got)
	}
	if got := cleanName("../../etc/pass\x00wd"); got != "passwd" {
		t.Fatalf("clean name %q", got)
	}
}

func samplePDF(t *testing.T, lines ...string) []byte {
	t.Helper()
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	for _, l := range lines {
		doc.Cell(0, 10, l)
		doc.Ln(10)
	}
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPDFDocuments(t *testing.T) {
	data := samplePDF(t, "Order API", "Webhook: https://shop.example.com/clickfix", "api_key=sk_live_ABCDEFGHIJKLMNOPQRSTUVWX")
	typ, err := documentType("api.pdf", "application/pdf", data)
	if err != nil || typ != "application/pdf" {
		t.Fatalf("%q %v", typ, err)
	}
	text, _, masked, err := extract(context.Background(), typ, data)
	if err != nil || !masked || !strings.Contains(text, "shop.example.com/clickfix") || strings.Contains(text, "sk_live_") {
		t.Fatalf("%q %v %v", text, masked, err)
	}
	for _, bad := range [][]byte{[]byte("%PDF-1.7\ngarbage"), append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{0xff, 0x00}, 500)...)} {
		if _, _, _, err := extract(context.Background(), "application/pdf", bad); !errors.Is(err, ErrDocumentPDF) {
			t.Fatalf("damaged pdf: %v", err)
		}
	}
}

// bombPDF is a one-page PDF whose content stream inflates to n bytes of
// text operators (a compression bomb).
func bombPDF(t *testing.T, n int) []byte {
	t.Helper()
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	chunk := bytes.Repeat([]byte("BT /F1 12 Tf (aaaaaaaa) Tj ET\n"), 4096)
	for written := 0; written < n; written += len(chunk) {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	var b bytes.Buffer
	offsets := []int{}
	obj := func(body string, stream []byte) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s", len(offsets), body)
		if stream != nil {
			b.WriteString("\nstream\n")
			b.Write(stream)
			b.WriteString("\nendstream")
		}
		b.WriteString("\nendobj\n")
	}
	b.WriteString("%PDF-1.4\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>", nil)
	obj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>", nil)
	obj("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>", nil)
	obj(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>", z.Len()), z.Bytes())
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", nil)
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

// A compression bomb cannot hold the request past the budget.
func TestPDFBombStopsAtTheBudget(t *testing.T) {
	data := bombPDF(t, 64<<20)
	if len(data) > MaxDocumentBytes {
		t.Fatalf("bomb too large for the test: %d", len(data))
	}
	prev := pdfBudget
	pdfBudget = 100 * time.Millisecond
	t.Cleanup(func() { pdfBudget = prev })
	start := time.Now()
	_, err := pdfText(context.Background(), data)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("took %v", took)
	}
	if err != nil && !errors.Is(err, ErrDocumentPDF) {
		t.Fatalf("error %v", err)
	}
	if got := clipBytes("çağ", 4); got != "ça" {
		t.Fatalf("clip %q", got)
	}
}

// A self-closing skipped tag does not swallow the rest of the page.
func TestHTMLSelfClosingSkippedTag(t *testing.T) {
	text := htmlText([]byte(`<p>Önce</p><svg/><p>Sonra</p><object/><p>Orta</p><script>gizli()</script><p>Son</p>`))
	for _, want := range []string{"Önce", "Sonra", "Orta", "Son"} {
		if !strings.Contains(text, want) {
			t.Fatalf("lost %q: %q", want, text)
		}
	}
	if strings.Contains(text, "gizli") {
		t.Fatalf("script kept: %q", text)
	}
	// <iframe/> is raw text up to </iframe> (as in browsers): no markup leaks.
	if text := htmlText([]byte(`<p>A</p><iframe src="x"/><p>B</p>`)); strings.Contains(text, "<p>") {
		t.Fatalf("markup leaked: %q", text)
	}
}
