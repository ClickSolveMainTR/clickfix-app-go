package googlechat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// CF-177: the message names its syntax (Markdown) instead of relying on
// the app's configured default; when Chat refuses it the same message goes
// out as plain text without a syntax.
func TestSendTranslatesFormatAndFallsBack(t *testing.T) {
	t.Parallel()
	var bodies []map[string]any
	reject := false
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "ya29.test", "token_type": "Bearer", "expires_in": 3599})
	})
	mux.HandleFunc("POST /v1/spaces/DM1/messages", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		if reject && b["markupSyntax"] != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"status":"INVALID_ARGUMENT"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "spaces/DM1/messages/R2"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	a := New(Config{Client: srv.Client(), BaseURL: srv.URL, TokenURL: srv.URL + "/token"})
	inst := instance(t, serviceAccountJSON(t, rsaKey(t)))
	msg := channels.OutboundMessage{MessageUUID: uuid.New(), ThreadID: "spaces/DM1", Text: "**Çözüldü** _şimdi_: [kayıt](https://ex.com/a_b)\n* bir"}

	if res, err := a.Send(context.Background(), inst, msg); err != nil || res.ExternalID != "spaces/DM1/messages/R2" {
		t.Fatalf("send = %+v, %v", res, err)
	}
	if b := bodies[0]; b["markupSyntax"] != "MARKUP_SYNTAX_MARKDOWN" || b["text"] != "**Çözüldü** *şimdi*: [kayıt](https://ex.com/a_b)\n- bir" {
		t.Fatalf("body = %+v", b)
	}

	reject, bodies = true, nil
	if res, err := a.Send(context.Background(), inst, msg); err != nil || res.ExternalID != "spaces/DM1/messages/R2" {
		t.Fatalf("fallback = %+v, %v", res, err)
	}
	if len(bodies) != 2 || bodies[1]["markupSyntax"] != nil || bodies[1]["text"] != "Çözüldü şimdi: kayıt (https://ex.com/a_b)\n* bir" {
		t.Fatalf("bodies = %+v", bodies)
	}
}
