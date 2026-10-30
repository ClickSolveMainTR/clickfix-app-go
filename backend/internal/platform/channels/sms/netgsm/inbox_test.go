package netgsm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

func TestInboxExplicitWindowAndStableID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sms/rest/v2/inbox" || r.URL.Query().Get("startdate") == "" || r.URL.Query().Get("stopdate") == "" {
			t.Error("destructive inbox read")
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "fixture" || p != "fixture" {
			t.Error("missing auth")
		}
		_, _ = fmt.Fprint(w,
			`{"code":"00","messages":[{"id":121386491,"gorevId":"120210000","kayittar":"1770000000000","mesaj":"Mesaj metni","operator":"Netgsm","gonderen":"5551112233"}]}`)
	}))
	defer server.Close()
	p := New(server.URL, server.Client())
	inst := channels.Instance{Credentials: channels.Credentials{"usercode": "fixture", "password": "fixture"}}
	r, e := p.Inbox(context.Background(), inst, time.Now().Add(-time.Hour), time.Now())
	if e != nil || len(r.Messages) != 1 || r.Messages[0].ExternalID != "120210000" || !strings.HasSuffix(r.Messages[0].From.Phone,
		"5551112233") {
		t.Fatalf("%+v %v", r, e)
	}
}
