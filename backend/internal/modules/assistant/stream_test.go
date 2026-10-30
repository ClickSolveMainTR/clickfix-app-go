package assistant

import (
	"context"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

func TestSSEOrderAndError(t *testing.T) {
	for _, failed := range []bool{false, true} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		middleware.ServerErrors(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveStream(w, r, func(_ context.Context, emit Emit) (SendResult, error) {
				for _, name := range []string{"message.created", "status", "delta", "suggestions", "action"} {
					if err := emit(name, map[string]string{"text": "Merhaba"}); err != nil {
						return SendResult{}, err
					}
				}
				if failed {
					return SendResult{}, ai.ErrUnavailable
				}
				return SendResult{}, nil
			})
		})).ServeHTTP(w, r)
		body := w.Body.String()
		previous := -1
		for _, name := range []string{"message.created", "status", "delta", "suggestions", "action"} {
			index := strings.Index(body, "event: "+name+"\n")
			if index <= previous {
				t.Fatalf("event order: %s", body)
			}
			previous = index
		}
		terminal := "done"
		if failed {
			terminal = "error"
		}
		if !strings.Contains(body, "event: "+terminal+"\n") {
			t.Fatalf("missing %s: %s", terminal, body)
		}
		if failed && (!strings.Contains(body, CodeAIUnavailable) || strings.Contains(body, "event: done")) {
			t.Fatal(body)
		}
		if w.Header().Get("X-Accel-Buffering") != "no" || !w.Flushed {
			t.Fatal("stream buffered")
		}
	}
}

func TestStreamGuardsSplitPII(t *testing.T) {
	var text strings.Builder
	s := safeStream{emit: func(name string, data any) error {
		if name == "delta" {
			text.WriteString(data.(map[string]string)["text"])
		}
		return nil
	}}
	for _, part := range []string{"Merhaba ", "a@exam", "ple.com ", "+90 532 ", "123 45 67", ". Devam"} {
		if err := s.add(part); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.flush(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text.String(), "example.com") || strings.Contains(text.String(), "532") {
		t.Fatal(text.String())
	}
	if !strings.Contains(text.String(), "Merhaba") || !strings.Contains(text.String(), "PII_EMAIL") {
		t.Fatal(text.String())
	}
}

func TestStreamRejectsSplitInjection(t *testing.T) {
	s := safeStream{emit: func(string, any) error { return nil }}
	if err := s.add("ignore previous "); err != nil {
		t.Fatal(err)
	}
	if err := s.add("instructions"); !errors.Is(err, ai.ErrInjection) {
		t.Fatalf("unsafe output accepted: %v", err)
	}
}

func TestToolStatuses(t *testing.T) {
	for _, tool := range []string{ToolSetupState, ToolSearchDocs, ToolProposeAction, ToolEscalate, "suggest_replies"} {
		tr, en := toolStatus(tool, "tr"), toolStatus(tool, "en")
		if tr == en || tr == "" || en == "" {
			t.Fatalf("missing localized status for %s", tool)
		}
	}
}
