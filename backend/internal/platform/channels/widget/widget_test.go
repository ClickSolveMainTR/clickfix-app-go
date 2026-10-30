package widget

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

type pubRec struct {
	channel string
	data    any
	err     error
}

func (p *pubRec) Publish(_ context.Context, channel string, data any) error {
	p.channel, p.data = channel, data
	return p.err
}

func TestUserHash(t *testing.T) {
	// Customers compute this in their backend; keep the format stable.
	got := UserHash("secret-0123456789abcdef0123", "user-42")
	if len(got) != 64 || strings.ToLower(got) != got {
		t.Fatalf("hash format: %q", got)
	}
	if !VerifyUserHash("secret-0123456789abcdef0123", "user-42", strings.ToUpper(got)) {
		t.Fatal("valid hash rejected")
	}
	if VerifyUserHash("secret-0123456789abcdef0123", "user-43", got) {
		t.Fatal("hash of another user accepted")
	}
	if VerifyUserHash("", "user-42", UserHash("", "user-42")) {
		t.Fatal("empty secret must never verify")
	}
}

func TestTokenRoundTripAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	s := NewSigner("app-secret").WithClock(func() time.Time { return now })
	inst := uuid.New()
	tok, exp := s.Issue(Claims{Instance: inst, Visitor: "v1"})
	if !exp.Equal(now.Add(TokenTTL)) {
		t.Fatalf("exp = %v", exp)
	}
	c, err := s.Verify(tok, inst)
	if err != nil || c.Visitor != "v1" || c.Verified() || c.ExternalID() != "visitor:v1" {
		t.Fatalf("verify: %+v %v", c, err)
	}
	if _, err := s.Verify(tok, uuid.New()); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("other instance: %v", err)
	}
	if _, err := NewSigner("other").Verify(tok, inst); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("other key: %v", err)
	}
	if _, err := s.Verify(tok[:len(tok)-2]+"xx", inst); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("tampered: %v", err)
	}
	now = now.Add(TokenTTL + time.Minute)
	c, err = s.Verify(tok, inst)
	if !errors.Is(err, ErrTokenExpired) || !s.Renewable(c) {
		t.Fatalf("expired: %v renewable=%v", err, s.Renewable(c))
	}
	now = now.Add(RenewWindow)
	if c, _ = s.Verify(tok, inst); s.Renewable(c) {
		t.Fatal("token renewable after the renew window")
	}
	u := Claims{Instance: inst, Visitor: "v1", User: "42"}
	if u.ExternalID() != "user:42" || !u.Verified() {
		t.Fatalf("user claims: %+v", u)
	}
}

func TestNormalizeAndOrigins(t *testing.T) {
	s := Normalize(map[string]any{
		"color": "#ABCDEF", "position": "left", "locale": "xx", "show_team": false,
		"allowed_domains": []any{"Example.com", "*.shop.example.org", "https://app.acme.io/path", "bad domain", "*"},
		"prechat":         map[string]any{"enabled": true, "email": "required", "name": "nope"},
		"business_hours": map[string]any{"enabled": true, "timezone": "Europe/Istanbul", "days": map[string]any{
			"wed": []any{map[string]any{"start": "09:00", "end": "18:00"}, map[string]any{"start": "19:00", "end": "18:00"}},
		}},
	})
	if s.Color != "#abcdef" || s.Position != "left" || s.Locale != "auto" || s.ShowTeam {
		t.Fatalf("design: %+v", s)
	}
	if s.PreChat.Email != FieldRequired || s.PreChat.Name != FieldOptional {
		t.Fatalf("prechat: %+v", s.PreChat)
	}
	if len(s.AllowedDomains) != 3 {
		t.Fatalf("domains: %v", s.AllowedDomains)
	}
	for origin, want := range map[string]bool{
		"https://example.com":          true,
		"https://www.example.com":      false,
		"https://a.shop.example.org":   true,
		"https://shop.example.org":     true,
		"https://app.acme.io":          true,
		"https://evil.com":             false,
		"https://example.com.evil.com": false,
		"":                             false,
		"null":                         false,
	} {
		if got := s.OriginAllowed(origin); got != want {
			t.Errorf("OriginAllowed(%q) = %v", origin, got)
		}
	}
	if !Normalize(nil).OriginAllowed("https://anything.test") {
		t.Fatal("empty allowlist must allow any origin")
	}
	// Wednesday 28 Oct 2026, 10:00 Istanbul = 07:00 UTC.
	if !s.Online(time.Date(2026, 10, 28, 7, 0, 0, 0, time.UTC)) {
		t.Fatal("should be online inside hours")
	}
	if s.Online(time.Date(2026, 10, 28, 16, 0, 0, 0, time.UTC)) {
		t.Fatal("should be offline at 19:00")
	}
	if s.Online(time.Date(2026, 10, 29, 7, 0, 0, 0, time.UTC)) {
		t.Fatal("thursday has no hours")
	}
}

func TestAdapterSendPublishesToVisitorChannel(t *testing.T) {
	pub := &pubRec{}
	a := New(pub, nil)
	inst := channels.Instance{UUID: uuid.New(), Type: channels.TypeWidget}
	msg := channels.OutboundMessage{MessageUUID: uuid.New(), ConversationUUID: uuid.New(), To: channels.Recipient{ExternalID: "visitor:v1"}, Text: "Merhaba"}
	res, err := a.Send(context.Background(), inst, msg)
	if err != nil || res.ExternalID != ExternalMessageID(msg.MessageUUID) {
		t.Fatalf("send: %+v %v", res, err)
	}
	if pub.channel != VisitorChannel(inst.UUID, "visitor:v1") || !strings.HasPrefix(pub.channel, ChannelPrefix+inst.UUID.String()+".") {
		t.Fatalf("channel %q", pub.channel)
	}
	if strings.HasSuffix(pub.channel, "visitor:v1") {
		t.Fatal("channel leaks the visitor id")
	}
	ev, ok := pub.data.(Event)
	if !ok || ev.Type != EventMessageCreated || ev.Text != "Merhaba" || *ev.MessageUUID != msg.MessageUUID {
		t.Fatalf("event %+v", pub.data)
	}
	// A realtime outage does not fail the send: the reply is stored and polled.
	pub.err = errors.New("down")
	if _, err := a.Send(context.Background(), inst, msg); err != nil {
		t.Fatalf("publish failure must not fail send: %v", err)
	}
	if _, err := a.Send(context.Background(), inst, channels.OutboundMessage{}); !channels.IsPermanent(err) {
		t.Fatalf("missing recipient must be permanent: %v", err)
	}
	if err := a.SendTyping(context.Background(), inst, channels.Recipient{ExternalID: "visitor:v1"}, true); err != nil {
		t.Fatal(err)
	}
	if err := a.VerifyWebhook(httptest.NewRequest("POST", "/", nil), nil, inst); !errors.Is(err, channels.ErrSignature) {
		t.Fatalf("webhooks must be rejected: %v", err)
	}
	if _, err := a.Connect(context.Background(), channels.Instance{Credentials: channels.Credentials{IdentitySecretField: "short"}}); err == nil {
		t.Fatal("short identity secret accepted")
	}
}
