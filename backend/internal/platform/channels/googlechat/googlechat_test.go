package googlechat

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const project = "123456789012"

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func certPEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: Issuer},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func serviceAccountJSON(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "clickfix-test", "private_key_id": "sa-key-1",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "chat-bot@clickfix-test.iam.gserviceaccount.com",
		"token_uri":    "https://attacker.example/token",
	})
	return string(b)
}

func instance(t *testing.T, sa string, spaces ...string) channels.Instance {
	t.Helper()
	ids := make([]any, 0, len(spaces))
	for _, s := range spaces {
		ids = append(ids, s)
	}
	return channels.Instance{UUID: uuid.New(), OrganizationID: 7, Type: channels.TypeGoogleChat,
		Credentials: channels.Credentials{FieldServiceAccount: sa, FieldProjectNumber: project},
		Settings:    map[string]any{SettingSpaceIDs: ids}}
}

func TestVerifyWebhookJWT(t *testing.T) {
	t.Parallel()
	google := rsaKey(t)
	other := rsaKey(t)
	var certFetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		certFetches.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"kid1": certPEM(t, google)})
	}))
	defer srv.Close()
	a := New(Config{Client: srv.Client(), CertsURL: srv.URL})
	inst := instance(t, "{}")

	sign := func(key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = kid
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	now := time.Now()
	valid := func() jwt.MapClaims {
		return jwt.MapClaims{"iss": Issuer, "aud": project, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
	}
	with := func(k string, v any) jwt.MapClaims {
		c := valid()
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return c
	}
	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid", sign(google, "kid1", valid()), true},
		{"wrong audience", sign(google, "kid1", with("aud", "999")), false},
		{"wrong issuer", sign(google, "kid1", with("iss", "someone@example.iam.gserviceaccount.com")), false},
		{"expired", sign(google, "kid1", with("exp", now.Add(-10*time.Minute).Unix())), false},
		{"no expiry", sign(google, "kid1", with("exp", nil)), false},
		{"other key", sign(other, "kid1", valid()), false},
		{"unknown kid", sign(google, "kid2", valid()), false},
		{"no token", "", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/channels/google_chat/x", nil)
		if c.token != "" {
			r.Header.Set("Authorization", "Bearer "+c.token)
		}
		err := a.VerifyWebhook(r, nil, inst)
		if c.ok && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, channels.ErrSignature) {
			t.Errorf("%s: want ErrSignature, got %v", c.name, err)
		}
	}
	// One fetch serves every check; the unknown kid refetch is rate limited.
	if n := certFetches.Load(); n != 1 {
		t.Errorf("certificate fetches = %d, want 1 (cached)", n)
	}
	// A different project's instance never accepts this project's tokens.
	inst.Credentials[FieldProjectNumber] = "42"
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", "Bearer "+sign(google, "kid1", valid()))
	if err := a.VerifyWebhook(r, nil, inst); !errors.Is(err, channels.ErrSignature) {
		t.Errorf("other project accepted: %v", err)
	}
}

func parse(t *testing.T, a *Adapter, inst channels.Instance, ev map[string]any) []channels.InboundMessage {
	t.Helper()
	b, _ := json.Marshal(ev)
	res, err := a.ParseWebhook(context.Background(), nil, b, inst)
	if err != nil {
		t.Fatal(err)
	}
	return res.Messages
}

func TestParseWebhookNormalizes(t *testing.T) {
	t.Parallel()
	a := New(Config{Client: http.DefaultClient, BaseURL: "http://127.0.0.1:1"})
	inst := instance(t, "{}", "spaces/ROOM1")
	elif := map[string]any{"name": "users/111", "displayName": "Elif Kaya", "email": "elif@example.com", "type": "HUMAN"}

	dm := parse(t, a, inst, map[string]any{
		"type": "MESSAGE", "eventTime": "2026-10-28T10:00:00.5Z",
		"space": map[string]any{"name": "spaces/DM1", "spaceType": "DIRECT_MESSAGE", "singleUserBotDm": true},
		"user":  elif,
		"message": map[string]any{"name": "spaces/DM1/messages/M1", "text": "Merhaba", "createTime": "2026-10-28T10:00:00Z",
			"sender": elif, "thread": map[string]any{"name": "spaces/DM1/threads/T1"}},
	})
	if len(dm) != 1 {
		t.Fatalf("dm messages = %d", len(dm))
	}
	m := dm[0]
	if m.ExternalID != "spaces/DM1/messages/M1" || m.ThreadID != "spaces/DM1" || m.Text != "Merhaba" || m.Kind != "text" {
		t.Errorf("dm normalized wrong: %+v", m)
	}
	if m.From.ExternalID != "users/111" || !m.From.InstanceScoped || m.From.Name != "Elif Kaya" || m.From.Email != "elif@example.com" {
		t.Errorf("dm sender wrong: %+v", m.From)
	}
	if !m.At.Equal(time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("at = %v", m.At)
	}

	room := map[string]any{
		"type":  "MESSAGE",
		"space": map[string]any{"name": "spaces/ROOM1", "spaceType": "SPACE", "displayName": "Destek"},
		"user":  elif,
		"message": map[string]any{"name": "spaces/ROOM1/messages/M2", "text": "@ClickFix yazıcı bozuk",
			"argumentText": " yazıcı bozuk", "sender": map[string]any{"name": "users/111", "type": "HUMAN"},
			"thread": map[string]any{"name": "spaces/ROOM1/threads/T9"}},
	}
	rm := parse(t, a, inst, room)
	if len(rm) != 1 {
		t.Fatalf("room messages = %d", len(rm))
	}
	m = rm[0]
	if m.ThreadID != "spaces/ROOM1/threads/T9" || m.Text != "yazıcı bozuk" || m.From.ExternalID != "spaces/ROOM1" || m.From.Name != "Destek" {
		t.Errorf("room normalized wrong: %+v", m)
	}
	// The sender had no display name in this event: the cache from the DM fills it.
	if m.Attributes["author_name"] != "Elif Kaya" || m.Attributes["author_id"] != "users/111" {
		t.Errorf("author attributes wrong: %+v", m.Attributes)
	}

	// Rooms outside settings.space_ids are ignored.
	other := instance(t, "{}", "spaces/ELSE")
	if got := parse(t, a, other, room); len(got) != 0 {
		t.Errorf("unselected room ingested: %+v", got)
	}

	// Bots, lifecycle events and ADDED_TO_SPACE with the adding message.
	bot := map[string]any{"type": "MESSAGE", "space": map[string]any{"name": "spaces/DM1", "spaceType": "DIRECT_MESSAGE"},
		"message": map[string]any{"name": "spaces/DM1/messages/B", "text": "x", "sender": map[string]any{"name": "users/b", "type": "BOT"}}}
	if got := parse(t, a, inst, bot); len(got) != 0 {
		t.Errorf("bot ingested")
	}
	removed := map[string]any{"type": "REMOVED_FROM_SPACE", "space": map[string]any{"name": "spaces/ROOM1", "spaceType": "SPACE"}}
	if got := parse(t, a, inst, removed); len(got) != 0 {
		t.Errorf("removal produced messages")
	}
	added := map[string]any{"type": "ADDED_TO_SPACE", "space": map[string]any{"name": "spaces/ROOM1", "spaceType": "SPACE", "displayName": "Destek 2"}}
	if got := parse(t, a, inst, added); len(got) != 0 {
		t.Errorf("plain add produced messages")
	}
	room["type"] = "ADDED_TO_SPACE"
	delete(room["space"].(map[string]any), "displayName")
	got := parse(t, a, inst, room)
	if len(got) != 1 || got[0].From.Name != "Destek 2" {
		t.Errorf("add with message: %+v", got)
	}

	// A message name outside the event's space is refused.
	room["type"] = "MESSAGE"
	room["message"].(map[string]any)["name"] = "spaces/OTHER/messages/M"
	b, _ := json.Marshal(room)
	if _, err := a.ParseWebhook(context.Background(), nil, b, inst); err == nil {
		t.Error("foreign message name accepted")
	}
}

func TestSendRepliesInThreadWithServiceAccount(t *testing.T) {
	t.Parallel()
	saKey := rsaKey(t)
	var tokens, creates atomic.Int32
	var conflict atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		tokens.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		claims := jwt.MapClaims{}
		if _, err := jwt.ParseWithClaims(r.Form.Get("assertion"), claims, func(*jwt.Token) (any, error) { return &saKey.PublicKey, nil },
			jwt.WithValidMethods([]string{"RS256"})); err != nil {
			t.Errorf("assertion: %v", err)
		}
		if claims["iss"] != "chat-bot@clickfix-test.iam.gserviceaccount.com" || claims["scope"] != Scope || !strings.HasSuffix(claims["aud"].(string), "/token") {
			t.Errorf("assertion claims: %v", claims)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "ya29.test", "token_type": "Bearer", "expires_in": 3599})
	})
	mux.HandleFunc("POST /v1/spaces/ROOM1/messages", func(w http.ResponseWriter, r *http.Request) {
		creates.Add(1)
		if r.Header.Get("Authorization") != "Bearer ya29.test" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		q := r.URL.Query()
		if !strings.HasPrefix(q.Get("messageId"), "client-") || q.Get("messageReplyOption") != "REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD" {
			t.Errorf("query = %v", q)
		}
		var body struct {
			Text   string `json:"text"`
			Thread struct {
				Name string `json:"name"`
			} `json:"thread"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Text != "Bakıyoruz" || body.Thread.Name != "spaces/ROOM1/threads/T9" {
			t.Errorf("body = %+v", body)
		}
		if conflict.Load() {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"status":"ALREADY_EXISTS"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "spaces/ROOM1/messages/R1"})
	})
	mux.HandleFunc("GET /v1/spaces/ROOM1/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "spaces/ROOM1/messages/R1"})
	})
	mux.HandleFunc("POST /v1/spaces/DM1/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("messageReplyOption") {
			t.Error("direct message sent with a thread option")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "spaces/DM1/messages/R2"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	a := New(Config{Client: srv.Client(), BaseURL: srv.URL, TokenURL: srv.URL + "/token"})
	inst := instance(t, serviceAccountJSON(t, saKey), "spaces/ROOM1")
	ctx := context.Background()

	if ext, err := a.Connect(ctx, inst); err != nil || ext != project {
		t.Fatalf("connect = %q, %v", ext, err)
	}
	msg := channels.OutboundMessage{MessageUUID: uuid.New(), ThreadID: "spaces/ROOM1/threads/T9", Text: "Bakıyoruz"}
	res, err := a.Send(ctx, inst, msg)
	if err != nil || res.ExternalID != "spaces/ROOM1/messages/R1" {
		t.Fatalf("send = %+v, %v", res, err)
	}
	// A retried send that already went through reads the message back.
	conflict.Store(true)
	if res, err = a.Send(ctx, inst, msg); err != nil || res.ExternalID != "spaces/ROOM1/messages/R1" {
		t.Fatalf("retried send = %+v, %v", res, err)
	}
	if res, err = a.Send(ctx, inst, channels.OutboundMessage{MessageUUID: uuid.New(), ThreadID: "spaces/DM1", Text: "Merhaba"}); err != nil || res.ExternalID != "spaces/DM1/messages/R2" {
		t.Fatalf("dm send = %+v, %v", res, err)
	}
	if n := tokens.Load(); n != 1 {
		t.Errorf("token requests = %d, want 1 (cached)", n)
	}
	if n := creates.Load(); n != 2 {
		t.Errorf("room creates = %d", n)
	}

	for name, m := range map[string]channels.OutboundMessage{
		"unselected room": {ThreadID: "spaces/OTHER/threads/T1", Text: "x"},
		"bad thread":      {ThreadID: "spaces/ROOM1/../x", Text: "x"},
		"too long":        {ThreadID: "spaces/DM1", Text: strings.Repeat("a", MaxText+1)},
		"empty":           {ThreadID: "spaces/DM1"},
	} {
		if _, err := a.Send(ctx, inst, m); !channels.IsPermanent(err) {
			t.Errorf("%s: want permanent error, got %v", name, err)
		}
	}
}

func TestConnectRejectsBadCredentials(t *testing.T) {
	t.Parallel()
	a := New(Config{Client: http.DefaultClient, TokenURL: "http://127.0.0.1:1/token"})
	for name, creds := range map[string]channels.Credentials{
		"not json":       {FieldServiceAccount: "nope", FieldProjectNumber: project},
		"not sa":         {FieldServiceAccount: `{"type":"authorized_user"}`, FieldProjectNumber: project},
		"bad project":    {FieldServiceAccount: serviceAccountJSON(t, rsaKey(t)), FieldProjectNumber: "my-project"},
		"missing fields": {},
	} {
		if _, err := a.Connect(context.Background(), channels.Instance{Credentials: creds}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if f := a.Fields(); len(f) != 2 || !f[0].Secret || f[1].Secret {
		t.Errorf("fields = %+v", f)
	}
}

func TestSpacesListsRooms(t *testing.T) {
	t.Parallel()
	saKey := rsaKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "token_type": "Bearer", "expires_in": 3600})
	})
	mux.HandleFunc("GET /v1/spaces", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"nextPageToken": "p2", "spaces": []map[string]any{
				{"name": "spaces/ROOM1", "displayName": "Destek", "spaceType": "SPACE"},
				{"name": "spaces/DM1", "spaceType": "DIRECT_MESSAGE"},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []map[string]any{
			{"name": "spaces/GC1", "spaceType": "GROUP_CHAT"},
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	a := New(Config{Client: srv.Client(), BaseURL: srv.URL, TokenURL: srv.URL + "/token"})
	got, err := a.Spaces(context.Background(), instance(t, serviceAccountJSON(t, saKey)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "spaces/ROOM1" || got[0].DisplayName != "Destek" || got[1].Name != "spaces/GC1" {
		t.Errorf("spaces = %+v", got)
	}
}
