package metagraph

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

func TestSignature(t *testing.T) {
	body := []byte(`{"object":"page"}`)
	sig := Sign("s3cret", body)
	if !VerifySignature("s3cret", sig, body) {
		t.Fatal("valid signature rejected")
	}
	for _, c := range []struct{ secret, header string }{{"s3cret", "sha256=00"}, {"other", sig}, {"", sig}, {"s3cret", ""}} {
		if VerifySignature(c.secret, c.header, body) {
			t.Fatalf("accepted %+v", c)
		}
	}
}

func TestChallenge(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?hub.mode=subscribe&hub.verify_token=vt&hub.challenge=42", nil)
	if got, ok := Challenge(r, "vt"); !ok || got != "42" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := Challenge(r, "other"); ok {
		t.Fatal("wrong token accepted")
	}
	if _, ok := Challenge(r, ""); ok {
		t.Fatal("empty configured token accepted")
	}
	r = httptest.NewRequest(http.MethodGet, "/?hub.mode=unsubscribe&hub.verify_token=vt&hub.challenge=42", nil)
	if _, ok := Challenge(r, "vt"); ok {
		t.Fatal("wrong mode accepted")
	}
}

func graphErr(status int, body string) error {
	return parseError(status, []byte(body))
}

func TestSendErrorMapping(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		permanent bool
		window    bool
		code      string
	}{
		{"window messenger", graphErr(400, `{"error":{"message":"(#10) This message is sent outside of allowed window.","type":"OAuthException","code":10,"error_subcode":2018278}}`), true, true, "window_closed"},
		{"window instagram", graphErr(400, `{"error":{"message":"outside of allowed window","code":10,"error_subcode":2534022}}`), true, true, "window_closed"},
		{"token", graphErr(400, `{"error":{"message":"Error validating access token","type":"OAuthException","code":190,"error_subcode":463}}`), true, false, "token_invalid"},
		{"rate", graphErr(400, `{"error":{"message":"(#613) Calls to this api have exceeded the rate limit.","code":613}}`), false, false, "rate_limited"},
		{"5xx", graphErr(500, `{"error":{"message":"An unknown error has occurred.","code":1}}`), false, false, "unavailable"},
		{"502 html", graphErr(502, `<html>bad gateway</html>`), false, false, "unavailable"},
		{"bad param", graphErr(400, `{"error":{"message":"(#100) No matching user found","code":100,"error_subcode":2018001}}`), true, false, "100"},
		{"network", fmt.Errorf("metagraph: %w", errors.New("connection refused")), false, false, "network"},
	}
	for _, c := range cases {
		err := SendError(c.err)
		var se *channels.SendError
		if !errors.As(err, &se) {
			t.Fatalf("%s: not a SendError: %v", c.name, err)
		}
		if channels.IsPermanent(err) != c.permanent || errors.Is(err, channels.ErrWindowClosed) != c.window || se.Code != c.code {
			t.Fatalf("%s: permanent=%v window=%v code=%q", c.name, channels.IsPermanent(err), errors.Is(err, channels.ErrWindowClosed), se.Code)
		}
	}
	if SendError(nil) != nil {
		t.Fatal("nil error mapped")
	}
}

func TestInstagramOAuthErrorShape(t *testing.T) {
	err := graphErr(400, `{"error_type":"OAuthException","code":400,"error_message":"Invalid authorization code"}`)
	ge, ok := AsError(err)
	if !ok || ge.Message != "Invalid authorization code" || ge.Type != "OAuthException" {
		t.Fatalf("got %+v", ge)
	}
}

func TestDownloadCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			w.Header().Set("Content-Length", "100")
		}
		if r.URL.Path == "/chunked" {
			w.(http.Flusher).Flush() // no Content-Length
		}
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer srv.Close()
	ctx := context.Background()
	if _, err := Download(ctx, srv.Client(), srv.URL+"/big", 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("declared size: %v", err)
	}
	// Chunked answer without Content-Length: the reader stops at the cap.
	rc, err := Download(ctx, srv.Client(), srv.URL+"/chunked", 10)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.ReadAll(rc); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("stream: %v", err)
	}
	rc2, err := Download(ctx, srv.Client(), srv.URL+"/ok", 100)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(rc2)
	_ = rc2.Close()
	if err != nil || len(b) != 100 {
		t.Fatalf("exact cap: %d %v", len(b), err)
	}
	if _, err := Download(ctx, nil, "file:///etc/passwd", 10); err == nil {
		t.Fatal("file url accepted")
	}
}

func TestRecentMessages(t *testing.T) {
	since := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Query().Get("after") == "" {
			_, _ = fmt.Fprintf(w, `{"data":[{"updated_time":"2026-10-28T10:30:00+0000","messages":{"data":[
				{"id":"m3","created_time":"2026-10-28T10:30:00+0000","from":{"id":"111"},"message":"reply"},
				{"id":"m2","created_time":"2026-10-28T10:20:00+0000","from":{"id":"555"},"message":"new"},
				{"id":"m1","created_time":"2026-10-28T09:00:00+0000","from":{"id":"555"},"message":"old"}]}}],
				"paging":{"next":"%s/v21.0/111/conversations?after=x"}}`, srv.URL)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"updated_time":"2026-10-28T10:10:00+0000","messages":{"data":[
			{"id":"m4","created_time":"2026-10-28T10:05:00+0000","from":{"id":"666"},"message":"","attachments":{"data":[{"mime_type":"image/png","image_data":{"url":"https://cdn/x.png"}}]}},
			{"id":"m2","created_time":"2026-10-28T10:20:00+0000","from":{"id":"555"},"message":"dup"}]}},
			{"updated_time":"2026-10-28T08:00:00+0000","messages":{"data":[{"id":"m0","created_time":"2026-10-28T08:00:00+0000","from":{"id":"777"}}]}}],
			"paging":{"next":"https://evil.example/next"}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "", srv.Client())
	msgs, err := c.RecentMessages(context.Background(), "111", "messenger", "tok", since, "111")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "m4,m2" {
		t.Fatalf("got %v", ids)
	}
	if _, ok := msgs[0].Attachments.Data[0].Inbound(nil); !ok {
		t.Fatal("image attachment not readable")
	}
}
