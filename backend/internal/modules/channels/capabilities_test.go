package channels

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

// stubAPI answers the channel routes from a fixed list.
type stubAPI struct {
	items []map[string]any
	calls []string
}

func (s *stubAPI) Call(_ context.Context, method, path string, _ any) (capability.Response, error) {
	s.calls = append(s.calls, method+" "+path)
	if method == http.MethodGet && strings.HasPrefix(path, capChannelsPath+"?") || path == capChannelsPath {
		raw, _ := json.Marshal(map[string]any{"items": s.items})
		return capability.Response{OK: true, Data: raw}, nil
	}
	for _, it := range s.items {
		if strings.HasSuffix(path, "/"+it["uuid"].(string)) {
			raw, _ := json.Marshal(it)
			return capability.Response{OK: true, Data: raw}, nil
		}
	}
	return capability.Response{Status: 404, Code: "NOT_FOUND", Message: "Channel not found"}, nil
}

func byName(name string) *capability.Capability {
	r := capability.NewRegistry().Register(Capabilities()...)
	c, _ := r.Get(name)
	return c
}

var existing = []map[string]any{
	{"uuid": "11111111-1111-1111-1111-111111111111", "type": "api", "name": "Sipariş sistemi", "status": "active",
		"webhook_path": "/v1/webhooks/channels/api/x", "credential_hints": map[string]string{"webhook_url": "https://shop.example.com/hook", "signing_secret": "••••abcd"}},
	{"uuid": "22222222-2222-2222-2222-222222222222", "type": "email", "name": "Destek", "status": "active", "credential_hints": map[string]string{"password": "••••"}},
}

func TestCreateRejectsUnsafeWebhooks(t *testing.T) {
	c := byName("channels.api.create")
	for _, u := range []string{"http://shop.example.com/h", "https://localhost/h", "https://10.0.0.5/h", "https://169.254.169.254/latest", "https://a:b@shop.example.com/h", "https://x.internal/h", "ftp://shop.example.com"} {
		if err := c.Check(capability.Params{"name": "A", "webhook_url": u}); !errors.Is(err, capability.ErrInvalid) {
			t.Fatalf("%s accepted", u)
		}
	}
	if err := c.Check(capability.Params{"name": "A", "webhook_url": "https://shop.example.com/clickfix"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateDryRunFindsDuplicatesWithoutWriting(t *testing.T) {
	api := &stubAPI{items: existing}
	e := capability.Exec{API: capability.ReadOnly(api), Locale: "tr"}
	c := byName("channels.api.create")
	if _, err := c.DryRun(context.Background(), e, capability.Params{"name": "Yeni", "webhook_url": "https://shop.example.com/hook"}); err == nil || !strings.Contains(err.Error(), "güncelleyin") {
		t.Fatalf("duplicate address: %v", err)
	}
	res, err := c.DryRun(context.Background(), e, capability.Params{"name": "Yeni", "webhook_url": "https://other.example.com/hook"})
	if err != nil || !strings.Contains(res.Summary, "Yeni") {
		t.Fatalf("%+v %v", res, err)
	}
	for _, call := range api.calls {
		if !strings.HasPrefix(call, "GET ") {
			t.Fatalf("dry run wrote: %s", call)
		}
	}
}

func TestListHidesSecretsAndUpdateNeedsAnAPIChannel(t *testing.T) {
	api := &stubAPI{items: existing}
	e := capability.Exec{API: capability.ReadOnly(api)}
	res, err := byName("channels.list").Run(context.Background(), e, capability.Params{})
	raw, _ := json.Marshal(res.Data)
	if err != nil || strings.Contains(string(raw), "abcd") || strings.Contains(string(raw), "credential") || !strings.Contains(string(raw), "shop.example.com") {
		t.Fatalf("list %s %v", raw, err)
	}
	up := byName("channels.api.update")
	if err := up.Check(capability.Params{"channel_uuid": "11111111-1111-1111-1111-111111111111"}); err == nil {
		t.Fatal("update without a change accepted")
	}
	if _, err := up.DryRun(context.Background(), e, capability.Params{"channel_uuid": "22222222-2222-2222-2222-222222222222", "name": "X"}); err == nil {
		t.Fatal("e-mail channel updated as an API channel")
	}
	if _, err := up.DryRun(context.Background(), e, capability.Params{"channel_uuid": "11111111-1111-1111-1111-111111111111", "name": "Sipariş sistemi"}); err == nil {
		t.Fatal("no-op update accepted")
	}
}

func TestSampleRequestsUsePlaceholders(t *testing.T) {
	d := SampleRequests("https://api.clickfix.example/v1/webhooks/channels/api/x", "https://shop.example.com/hook", false)
	if len(d) != 3 || !strings.Contains(d[0].Code, "$CLICKFIX_SIGNING_SECRET") || !strings.Contains(d[0].Code, "X-ClickFix-Signature") || !strings.Contains(d[1].Code, "shop.example.com") {
		t.Fatalf("%+v", d)
	}
}
