package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscribeHTTPFormatsAndSegments(t *testing.T) {
	for _, tc := range []struct{ model, format string }{{"whisper-1", "verbose_json"}, {"gpt-4o-transcribe", "json"}, {"gpt-4o-transcribe-diarize", "diarized_json"}} {
		t.Run(tc.model, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("wrong authenticated audio endpoint")
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer func() { _ = r.MultipartForm.RemoveAll() }()
				if r.FormValue("model") != tc.model || r.FormValue("response_format") != tc.format {
					t.Error("wrong format/model")
				}
				f, h, err := r.FormFile("file")
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = f.Close() }()
				data, _ := io.ReadAll(f)
				if string(data) != "audio" || h.Filename != "call.wav" {
					t.Error("audio upload changed")
				}
				_, _ = fmt.Fprint(w, `{"text":"Hello","duration":4.5,"segments":[{"start":0,"end":4.5,"speaker":"A","text":"Hello"}]}`)
			}))
			defer srv.Close()
			p := NewOpenAI("test-key", srv.URL, "")
			out, err := p.Transcribe(context.Background(), AudioRequest{Audio: []byte("audio"), Filename: "call.wav", Model: tc.model})
			if err != nil || len(out.Segments) != 1 || out.Segments[0].Speaker != "A" || out.Duration != 4.5 {
				t.Fatalf("%+v %v", out, err)
			}
		})
	}
}
func TestTranscribeHTTPRejectsWithoutRetryAndBounds(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(429) }))
	defer srv.Close()
	p := NewOpenAI("test", srv.URL, "")
	if _, err := p.Transcribe(context.Background(), AudioRequest{Audio: []byte("audio"), Model: "whisper-1"}); !NotAccepted(err) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	for _, raw := range []string{"", strings.Repeat("x", MaxAudioBytes+1)} {
		if _, err := p.Transcribe(context.Background(), AudioRequest{Audio: []byte(raw), Model: "whisper-1"}); err != ErrInvalidOutput {
			t.Fatal(err)
		}
	}
}
