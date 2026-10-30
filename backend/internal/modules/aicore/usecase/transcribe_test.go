package usecase

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
)

func TestMeteredTranscriptionReservationsAndMaskedTrace(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		status               int
		body                 string
		wantStatus           string
		wantCost, wantCredit int64
	}{
		{"success", 200, `{"text":"Email me at person@example.test","segments":[{"start":0,"end":1,"speaker":"person@example.test","text":"person@example.test"}]}`, "completed", 12000, 2},
		{"rejection", 429, `{}`, "failed", 0, 0},
		{"unknown", 200, `invalid`, "unknown", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, org, _ := aiFixture(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			speech := &MeteredTranscriber{Repo: svc.Repo, Raw: ai.NewOpenAI("test", srv.URL, ""), Model: "whisper-1", PricePerMinute: 6000}
			id := uuid.New()
			out, err := speech.Transcribe(context.Background(), org, 0, id, uuid.Nil, ai.AudioRequest{Audio: []byte("audio"), DurationSeconds: 61})
			if tc.name == "success" && (err != nil || strings.Contains(out.Text, "person@example.test") || strings.Contains(out.Segments[0].Speaker, "person@example.test")) {
				t.Fatalf("out=%+v err=%v", out, err)
			}
			run, e := svc.Repo.Get(context.Background(), org, id)
			if e != nil || run.Task != "transcribe" || run.Status != tc.wantStatus || run.CostMicros != tc.wantCost || run.ChargedCredits != tc.wantCredit || strings.Contains(run.Output, "person@example.test") {
				t.Fatalf("run=%+v err=%v", run, e)
			}
			// Reusing the same run ID can never reach the provider again.
			_, _ = speech.Transcribe(context.Background(), org, 0, id, uuid.Nil, ai.AudioRequest{Audio: []byte("audio"), DurationSeconds: 61})
			if calls != 1 {
				t.Fatalf("provider replayed %d times", calls)
			}
			speech.PricePerMinute = 0
			if _, e = speech.Transcribe(context.Background(), org, 0, uuid.New(), uuid.Nil, ai.AudioRequest{Audio: []byte("audio"), DurationSeconds: 61}); e != ai.ErrUnavailable {
				t.Fatal(e)
			}
		})
	}
}
