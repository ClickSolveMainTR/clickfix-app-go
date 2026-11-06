package widget_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

func TestEditorDraftPublishRollback(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	cfgs := usecase.NewConfigs(e.q, convrepository.New(e.pool, e.q), e.ib.Channels)

	cfg, err := cfgs.Config(ctx, e.org, e.inst.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 0 || cfg.Draft != nil || cfg.Published["color"] != "#112233" || cfg.SchemaVersion != widget.SchemaVersion {
		t.Fatalf("config = %+v", cfg)
	}
	// Another organization never sees the widget.
	if _, err := cfgs.Config(ctx, e.org+100000, e.inst.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign org = %v", err)
	}

	// A draft keeps invalid values and reports them.
	bad := cloneMap(cfg.Published)
	bad["appearance"].(map[string]any)["radius"] = float64(99)
	draft, err := cfgs.SaveDraft(ctx, e.org, e.agent, e.inst.UUID, bad, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Issues) != 1 || draft.Issues[0].Path != "appearance.radius" {
		t.Fatalf("draft issues = %+v", draft.Issues)
	}
	var se *usecase.SettingsError
	if _, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{}); !errors.As(err, &se) {
		t.Fatalf("publishing an invalid draft = %v", err)
	}

	good := cloneMap(cfg.Published)
	good["appearance"].(map[string]any)["radius"] = float64(4)
	good["greeting_title"] = "Selam"
	if _, err := cfgs.SaveDraft(ctx, e.org, e.agent, e.inst.UUID, good, 0); err != nil {
		t.Fatal(err)
	}
	v1, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{BaseVersion: 0, Note: "first"})
	if err != nil || v1.Version != 1 {
		t.Fatalf("publish = %+v %v", v1, err)
	}
	if cfg, _ = cfgs.Config(ctx, e.org, e.inst.UUID); cfg.Draft != nil || cfg.Version != 1 {
		t.Fatalf("after publish draft=%v version=%d", cfg.Draft, cfg.Version)
	}
	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil)
	if str(boot.Data, "widget", "settings", "greeting_title") != "Selam" {
		t.Fatalf("visitor sees old settings: %v", boot.Data["widget"])
	}
	// A stale editor cannot overwrite the newer version.
	if _, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{Settings: good, BaseVersion: 0}); !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale publish = %v", err)
	}
	good["greeting_title"] = "İkinci"
	if _, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{Settings: good, BaseVersion: 1}); err != nil {
		t.Fatal(err)
	}
	rb, err := cfgs.Rollback(ctx, e.org, e.agent, e.inst.UUID, 1, 2)
	if err != nil || rb.Version != 3 || rb.RollbackOf == nil || *rb.RollbackOf != 1 {
		t.Fatalf("rollback = %+v %v", rb, err)
	}
	versions, err := cfgs.Versions(ctx, e.org, e.inst.UUID)
	if err != nil || len(versions) != 3 || versions[0].Version != 3 || versions[2].Note != "first" || versions[2].CreatedByName != "Elif Şahin" {
		t.Fatalf("versions = %+v %v", versions, err)
	}
	if str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "widget", "settings", "greeting_title") != "Selam" {
		t.Fatal("rollback did not restore v1")
	}
	// Session replay settings survive publishing (replay.go reads them raw).
	withReplay := cloneMap(good)
	withReplay["replay"] = map[string]any{"replay_enabled": true}
	if _, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{Settings: withReplay, BaseVersion: 3}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = cfgs.Config(ctx, e.org, e.inst.UUID); cfg.Published["replay"] == nil {
		t.Fatal("replay block dropped")
	}

	dup, err := cfgs.Duplicate(ctx, e.org, e.agent, e.inst.UUID, "")
	if err != nil || dup.UUID == e.inst.UUID || dup.Name != "Site (2)" || dup.Settings["greeting_title"] != "İkinci" {
		t.Fatalf("duplicate = %+v %v", dup, err)
	}
	if len(dup.CredentialHints) != 0 {
		t.Fatalf("duplicate copied credentials: %v", dup.CredentialHints)
	}
	if err := cfgs.DiscardDraft(ctx, e.org, e.inst.UUID); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapContextAndInstalls(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	cfgs := usecase.NewConfigs(e.q, convrepository.New(e.pool, e.q), e.ib.Channels)
	settings := map[string]any{
		"allowed_domains": []any{"shop.example.com"},
		"targeting":       map[string]any{"hide_on": []any{map[string]any{"op": "starts_with", "value": "/checkout"}}},
		"triggers": []any{
			map[string]any{"id": "pricing", "message": map[string]any{"tr": "Yardım?"}, "conditions": map[string]any{"url": []any{map[string]any{"op": "contains", "value": "/pricing"}}}},
			map[string]any{"id": "back", "message": "Tekrar hoş geldiniz", "conditions": map[string]any{"visitor": "returning"}},
		},
		"reply_time": map[string]any{"mode": "manual", "value": "hours"},
	}
	if _, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{Settings: settings}); err != nil {
		t.Fatal(err)
	}
	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, map[string]any{"page_url": siteOrigin + "/pricing?ref=x", "sdk_version": "2.0.0"})
	c, _ := boot.Data["context"].(map[string]any)
	if boot.Code != http.StatusOK || c["visible"] != true || len(c["eligible_triggers"].([]any)) != 1 || c["eligible_triggers"].([]any)[0] != "pricing" {
		t.Fatalf("context = %d %v", boot.Code, c)
	}
	if str(boot.Data, "widget", "reply", "value") != "hours" {
		t.Fatalf("reply = %v", boot.Data["widget"].(map[string]any)["reply"])
	}
	token := str(boot.Data, "visitor", "token")
	again := e.call(http.MethodPost, "/bootstrap", token, siteOrigin, map[string]any{"page_url": siteOrigin + "/checkout/pay"})
	c = again.Data["context"].(map[string]any)
	if c["visible"] != false || c["hidden_reason"] != "hidden_page" || c["returning"] != true {
		t.Fatalf("checkout context = %v", c)
	}
	installs, err := cfgs.Installs(ctx, e.org, e.inst.UUID)
	if err != nil || len(installs) != 1 || installs[0].Origin != siteOrigin || installs[0].LastPageURL != siteOrigin+"/pricing" || installs[0].SDKVersion != "2.0.0" {
		t.Fatalf("installs = %+v %v", installs, err)
	}
	// Unknown body fields are rejected like every other widget call.
	if r := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, map[string]any{"nope": 1}); r.Code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", r.Code)
	}
}

type fakeFields struct{}

func (fakeFields) Definitions(context.Context, int64, string, *int64) ([]customfields.Definition, error) {
	return []customfields.Definition{
		{Key: "plan", Label: "Plan", Type: customfields.TypeSelect, Options: []customfields.Option{{Value: "pro", Label: "Pro"}, {Value: "free", Label: "Free"}}},
		{Key: "seats", Label: "Seats", Type: customfields.TypeNumber},
	}, nil
}

func TestPreChatConsentAttributesAndAI(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.svc.WithFields(fakeFields{})
	cfgs := usecase.NewConfigs(e.q, convrepository.New(e.pool, e.q), e.ib.Channels)
	settings := map[string]any{
		"allowed_domains": []any{"shop.example.com"},
		"prechat": map[string]any{
			"enabled": true,
			"fields":  []any{map[string]any{"key": "company_size", "label": "Ekip", "type": "select", "options": []any{"1-10", "11-50"}, "required": true, "attribute": "seats_hint"}},
			"consent": map[string]any{"enabled": true, "required": true, "text": "KVKK metnini okudum"},
		},
		"attribute_mapping": []any{
			map[string]any{"key": "plan", "target": "custom:plan"},
			map[string]any{"key": "seats", "target": "custom:seats", "anonymous": true},
			map[string]any{"key": "seats_hint", "target": "custom:plan"},
		},
		"ai":          map[string]any{"enabled": false},
		"attachments": map[string]any{"types": []any{"image"}},
	}
	if issues := widget.Validate(settings); len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
	if _, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{Settings: settings}); err != nil {
		t.Fatal(err)
	}
	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil)
	token := str(boot.Data, "visitor", "token")
	if r := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "selam", "prechat": map[string]any{"company_size": "1-10"}}); r.Code != http.StatusBadRequest || str(r.Err, "code") != handlerCodeConsent {
		t.Fatalf("no consent = %d %v", r.Code, r.Err)
	}
	if r := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "selam", "consent": true}); r.Code != http.StatusBadRequest {
		t.Fatalf("missing required field = %d", r.Code)
	}
	// Attributes before the first message are pending.
	if r := e.call(http.MethodPatch, "/attributes", token, siteOrigin, map[string]any{"attributes": map[string]any{"seats": 5}}); r.Data["pending"] != true {
		t.Fatalf("attributes before contact = %v", r.Data)
	}
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{
		"text": "selam", "consent": true, "prechat": map[string]any{"company_size": "1-10"}, "attributes": map[string]any{"seats": 5},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", created.Code, created.Err)
	}
	var attrs []byte
	if err := e.pool.QueryRow(ctx, "SELECT m.content_attributes FROM messages m JOIN conversations c ON c.id = m.conversation_id WHERE c.uuid = $1", str(created.Data, "conversation_uuid")).Scan(&attrs); err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	_ = json.Unmarshal(attrs, &stored)
	if stored["ai_disabled"] != true || stored["prechat"].(map[string]any)["company_size"] != "1-10" || stored["consent"] == nil {
		t.Fatalf("message attributes = %v", stored)
	}
	// Anonymous visitors may only set attributes marked anonymous.
	r := e.call(http.MethodPatch, "/attributes", token, siteOrigin, map[string]any{"attributes": map[string]any{"seats": 12, "plan": "pro", "unknown": 1}})
	if r.Code != http.StatusOK || len(r.Data["applied"].([]any)) != 1 || len(r.Data["ignored"].([]any)) != 2 {
		t.Fatalf("anonymous attributes = %v %v", r.Data, r.Err)
	}
	var custom []byte
	if err := e.pool.QueryRow(ctx, "SELECT ct.custom_fields FROM conversations c JOIN contacts ct ON ct.id = c.contact_id WHERE c.uuid = $1", str(created.Data, "conversation_uuid")).Scan(&custom); err != nil {
		t.Fatal(err)
	}
	var cf map[string]any
	_ = json.Unmarshal(custom, &cf)
	if cf["seats"] != float64(12) || cf["plan"] != nil {
		t.Fatalf("custom fields = %v", cf)
	}
	// Disallowed file types are refused.
	if _, err := e.svc.Send(ctx, mustOpen(t, e), mustClaims(t, e, token), usecase.SendInput{
		ConversationUUID: ptr(uuid.MustParse(str(created.Data, "conversation_uuid"))), File: &usecase.Upload{Filename: "a.pdf", ContentType: "application/pdf", Data: []byte("x")},
	}); !errors.Is(err, usecase.ErrFileType) {
		t.Fatalf("pdf on image-only widget = %v", err)
	}
}

const handlerCodeConsent = "WIDGET_CONSENT_REQUIRED"

func mustOpen(t *testing.T, e *env) usecase.Widget {
	t.Helper()
	w, err := e.svc.Open(context.Background(), e.inst.UUID, siteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func mustClaims(t *testing.T, e *env, token string) widget.Claims {
	t.Helper()
	c, err := e.svc.Authenticate(mustOpen(t, e), token)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func ptr[T any](v T) *T { return &v }

func cloneMap(m map[string]any) map[string]any {
	raw, _ := json.Marshal(m)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// CF-176: a rejected identify call is remembered per widget (fixed code and
// time only) for the editor, and a verified identify clears it.
func TestIdentityRejectionIsRecordedAndCleared(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	cfgs := usecase.NewConfigs(e.q, convrepository.New(e.pool, e.q), e.ib.Channels)
	// A widget whose identity secret was never generated.
	bare, err := e.ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeWidget, Name: "No secret",
		Settings: map[string]any{"allowed_domains": []any{"shop.example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.inst = bare
	if got, err := cfgs.IdentityRejection(ctx, e.org, bare.UUID); err != nil || got != nil {
		t.Fatalf("fresh widget rejection = %+v %v", got, err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	const hash = "5ca1ab1e5ca1ab1e5ca1ab1e5ca1ab1e5ca1ab1e5ca1ab1e5ca1ab1e5ca1ab1e"
	body := map[string]any{"user_id": "user-176", "user_hash": hash}

	// No secret: the request is still rejected, and the reason is recorded.
	if r := e.call(http.MethodPost, "/identify", token, siteOrigin, body); r.Code != http.StatusForbidden || str(r.Err, "code") != "WIDGET_IDENTITY_DISABLED" {
		t.Fatalf("identify without secret = %d %v", r.Code, r.Err)
	}
	first, err := cfgs.IdentityRejection(ctx, e.org, bare.UUID)
	if err != nil || first == nil || first.Code != usecase.RejectionIdentityDisabled || first.RejectedAt.IsZero() {
		t.Fatalf("rejection = %+v %v", first, err)
	}
	// Nothing of the request is stored.
	var row string
	if err := e.pool.QueryRow(ctx, "SELECT row_to_json(r)::text FROM widget_identity_rejections r WHERE organization_id = $1", e.org).Scan(&row); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(row, hash) || strings.Contains(row, "user-176") {
		t.Fatalf("rejection row leaks the request: %s", row)
	}
	// Repeats inside a minute do not turn into writes.
	e.call(http.MethodPost, "/identify", token, siteOrigin, body)
	var rows int
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM widget_identity_rejections WHERE organization_id = $1", e.org).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	again, _ := cfgs.IdentityRejection(ctx, e.org, bare.UUID)
	if rows != 1 || again == nil || !again.RejectedAt.Equal(first.RejectedAt) {
		t.Fatalf("throttle: rows=%d first=%v again=%+v", rows, first.RejectedAt, again)
	}

	// With a secret, a wrong hash is its own code (and replaces the old one).
	if _, err := e.ib.Channels.Update(ctx, e.org, bare.UUID, channelsusecase.UpdateInput{
		Credentials: channels.Credentials{widget.IdentitySecretField: identitySecret},
	}); err != nil {
		t.Fatal(err)
	}
	if r := e.call(http.MethodPost, "/identify", token, siteOrigin, body); r.Code != http.StatusUnauthorized || str(r.Err, "code") != "WIDGET_IDENTITY_INVALID" {
		t.Fatalf("identify with wrong hash = %d %v", r.Code, r.Err)
	}
	if got, err := cfgs.IdentityRejection(ctx, e.org, bare.UUID); err != nil || got == nil || got.Code != usecase.RejectionIdentityInvalid {
		t.Fatalf("invalid rejection = %+v %v", got, err)
	}

	// A verified user clears the warning.
	ok := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{"user_id": "user-176", "user_hash": widget.UserHash(identitySecret, "user-176")})
	if ok.Code != http.StatusOK || ok.Data["verified"] != true {
		t.Fatalf("verified identify = %d %v", ok.Code, ok.Err)
	}
	if got, err := cfgs.IdentityRejection(ctx, e.org, bare.UUID); err != nil || got != nil {
		t.Fatalf("rejection after verified identify = %+v %v", got, err)
	}
	// Another organization cannot read it.
	if _, err := cfgs.IdentityRejection(ctx, e.org+1_000_000, bare.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross-organization read = %v", err)
	}
}
