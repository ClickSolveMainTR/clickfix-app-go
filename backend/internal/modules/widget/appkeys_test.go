package widget_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/google/uuid"
)

// callApp is call without an Origin: a native app sends its app key and
// package name instead (CF-201).
func (e *env) callApp(method, path, token, key, pkg string, body any) reply {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, "/v1/widget/"+e.inst.UUID.String()+path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set(handler.AppKeyHeader, key)
	}
	if pkg != "" {
		req.Header.Set(handler.AppPackageHeader, pkg)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return reply{Code: rec.Code, Data: env.Data, Err: env.Error, Hdr: rec.Header()}
}

// CF-201: a mobile app key replaces the origin allowlist; the package name
// must match, a revoked or foreign key fails, and the visitor flow behind
// it (user_hash, tokens) is unchanged.
func TestAppKeyFlow(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	cfgs := usecase.NewConfigs(e.q, convrepository.New(e.pool, e.q), e.ib.Channels)

	for _, bad := range []usecase.CreateAppKeyInput{
		{Platform: "web", PackageName: "com.example.shop"},
		{Platform: "android", PackageName: "shop"},
		{Platform: "ios", PackageName: "com.example.shop", Label: strings.Repeat("x", 101)},
	} {
		if _, err := cfgs.CreateAppKey(ctx, e.org, e.agent, e.inst.UUID, bad); err == nil {
			t.Fatalf("CreateAppKey(%+v) accepted", bad)
		}
	}
	created, err := cfgs.CreateAppKey(ctx, e.org, e.agent, e.inst.UUID, usecase.CreateAppKeyInput{Platform: "android", PackageName: "com.example.shop", Label: " Mağaza "})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Key, usecase.AppKeyPrefix) || created.KeyPrefix != created.Key[:12] || created.Label != "Mağaza" || created.RevokedAt != nil {
		t.Fatalf("created = %+v", created)
	}
	keys, err := cfgs.ListAppKeys(ctx, e.org, e.inst.UUID)
	if err != nil || len(keys) != 1 || keys[0].UUID != created.UUID {
		t.Fatalf("ListAppKeys = %v %v", keys, err)
	}
	if b, _ := json.Marshal(keys[0]); strings.Contains(string(b), created.Key) || strings.Contains(string(b), "hash") {
		t.Fatalf("list leaks the key: %s", b)
	}

	// No Origin and no key: the allowlist rejects (unchanged behaviour).
	if r := e.callApp(http.MethodPost, "/bootstrap", "", "", "", nil); r.Code != http.StatusForbidden || str(r.Err, "code") != handler.CodeOriginNotAllowed {
		t.Fatalf("no origin bootstrap = %d %v", r.Code, r.Err)
	}
	for name, c := range map[string][2]string{
		"wrong package":  {created.Key, "com.example.other"},
		"empty package":  {created.Key, ""},
		"wrong key":      {usecase.AppKeyPrefix + strings.Repeat("0", 40), "com.example.shop"},
		"foreign format": {"not-a-key", "com.example.shop"},
	} {
		if r := e.callApp(http.MethodPost, "/bootstrap", "", c[0], c[1], nil); r.Code != http.StatusForbidden || str(r.Err, "code") != handler.CodeAppKeyInvalid {
			t.Fatalf("%s bootstrap = %d %v", name, r.Code, r.Err)
		}
	}
	// A valid key: no Origin needed, package name compared case-insensitively.
	boot := e.callApp(http.MethodPost, "/bootstrap", "", created.Key, "COM.example.Shop", map[string]any{"sdk_version": "core-0.1.0"})
	if boot.Code != http.StatusOK || str(boot.Data, "widget", "settings", "color") != "#112233" {
		t.Fatalf("app bootstrap = %d %v", boot.Code, boot.Err)
	}
	token := str(boot.Data, "visitor", "token")
	if token == "" {
		t.Fatalf("no visitor session: %v", boot.Data)
	}
	// A foreign Origin alongside a valid key is ignored: the key decides.
	req := httptest.NewRequest(http.MethodGet, "/v1/widget/"+e.inst.UUID.String()+"/conversations", nil)
	req.Header.Set("Origin", "https://evil.test")
	req.Header.Set(handler.AppKeyHeader, created.Key)
	req.Header.Set(handler.AppPackageHeader, "com.example.shop")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("app conversations with foreign origin = %d %s", rec.Code, rec.Body)
	}
	// The install checker lists the app instead of a page origin.
	installs, err := cfgs.Installs(ctx, e.org, e.inst.UUID)
	if err != nil || len(installs) != 1 || installs[0].Origin != "app://android/com.example.shop" || installs[0].SDKVersion != "core-0.1.0" {
		t.Fatalf("installs = %+v %v", installs, err)
	}
	// user_hash is verified exactly as in a browser.
	if r := e.callApp(http.MethodPost, "/identify", token, created.Key, "com.example.shop", map[string]any{"user_id": "u-9", "user_hash": "00"}); r.Code != http.StatusUnauthorized {
		t.Fatalf("bad hash via app = %d %v", r.Code, r.Err)
	}
	user := e.callApp(http.MethodPost, "/identify", token, created.Key, "com.example.shop", map[string]any{
		"user_id": "u-9", "user_hash": widget.UserHash(identitySecret, "u-9"), "name": "Ayşe", "company": map[string]any{"id": "c-1", "name": "Örnek A.Ş."},
	})
	if user.Code != http.StatusOK || user.Data["verified"] != true {
		t.Fatalf("identify via app = %d %v %v", user.Code, user.Data, user.Err)
	}
	userToken := str(user.Data, "token")
	sent := e.callApp(http.MethodPost, "/conversations", userToken, created.Key, "com.example.shop", map[string]any{"text": "Uygulamadan merhaba", "client_id": uuid.NewString()})
	if sent.Code != http.StatusCreated && sent.Code != http.StatusOK {
		t.Fatalf("send via app = %d %v", sent.Code, sent.Err)
	}
	// The visitor token of an app session is not tied to the key: a browser
	// page on the allowlist may continue it (webview handover).
	if r := e.call(http.MethodGet, "/conversations", userToken, siteOrigin, nil); r.Code != http.StatusOK {
		t.Fatalf("handover to browser = %d %v", r.Code, r.Err)
	}

	// Revoking stops the key at once; revoking twice is not found; the row stays listed.
	revoked, err := cfgs.RevokeAppKey(ctx, e.org, e.inst.UUID, created.UUID)
	if err != nil || revoked.RevokedAt == nil {
		t.Fatalf("RevokeAppKey = %+v %v", revoked, err)
	}
	if _, err := cfgs.RevokeAppKey(ctx, e.org, e.inst.UUID, created.UUID); err != usecase.ErrNotFound {
		t.Fatalf("second revoke = %v", err)
	}
	if r := e.callApp(http.MethodGet, "/conversations", userToken, created.Key, "com.example.shop", nil); r.Code != http.StatusForbidden || str(r.Err, "code") != handler.CodeAppKeyInvalid {
		t.Fatalf("revoked key = %d %v", r.Code, r.Err)
	}
	if keys, _ := cfgs.ListAppKeys(ctx, e.org, e.inst.UUID); len(keys) != 1 || keys[0].RevokedAt == nil {
		t.Fatalf("revoked key not listed: %+v", keys)
	}

	// A key of another widget of the same organization is rejected here.
	other, err := e.ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeWidget, Name: "Second app",
		Settings: map[string]any{"color": "#445566"},
	})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := cfgs.CreateAppKey(ctx, e.org, e.agent, other.UUID, usecase.CreateAppKeyInput{Platform: "ios", PackageName: "com.example.shop"})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.callApp(http.MethodPost, "/bootstrap", "", foreign.Key, "com.example.shop", nil); r.Code != http.StatusForbidden || str(r.Err, "code") != handler.CodeAppKeyInvalid {
		t.Fatalf("other widget's key = %d %v", r.Code, r.Err)
	}
	// Another organization cannot see or revoke the key.
	if _, err := cfgs.RevokeAppKey(ctx, e.org+1, e.inst.UUID, foreign.UUID); err != usecase.ErrNotFound {
		t.Fatalf("cross-org revoke = %v", err)
	}
}
