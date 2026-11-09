package widget_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
)

// CF-195: the customer created a contact with its own user id and an
// agent wrote first, before the user ever opened the site. When the user
// signs in, identify lands on that contact (no second one), marks it
// verified and the visitor finds the agent's conversation.
func TestIdentifyAdoptsExternalContact(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, "INSERT INTO contacts (organization_id, name, external_id) VALUES ($1, 'Zeynep', 'u-500')", e.org); err != nil {
		t.Fatal(err)
	}
	conv, err := e.ib.Conversations.Create(ctx, e.org, convusecase.Actor{ID: e.agent}, convusecase.CreateInput{
		ContactExternalID: "u-500", Inbox: e.inst.UUID, Subject: "Hoş geldiniz",
	})
	if err != nil {
		t.Fatalf("agent-started conversation: %v", err)
	}
	var identities int
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM contact_identities WHERE organization_id = $1 AND external_id = 'user:u-500'", e.org).Scan(&identities); err != nil || identities != 1 {
		t.Fatalf("widget identity for the external id = %d %v", identities, err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	user := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{
		"user_id": "u-500", "user_hash": widget.UserHash(identitySecret, "u-500"), "name": "Zeynep K.",
	})
	if user.Code != http.StatusOK {
		t.Fatalf("identify = %d %v", user.Code, user.Err)
	}
	var contacts int
	var verified bool
	if err := e.pool.QueryRow(ctx, "SELECT count(*), bool_and(verified_at IS NOT NULL) FROM contacts WHERE organization_id = $1 AND external_id = 'u-500' AND deleted_at IS NULL", e.org).Scan(&contacts, &verified); err != nil || contacts != 1 || !verified {
		t.Fatalf("contacts with the id = %d verified=%v %v", contacts, verified, err)
	}
	list := e.call(http.MethodGet, "/conversations", str(user.Data, "token"), siteOrigin, nil)
	if list.Code != http.StatusOK || !strings.Contains(listUUIDs(list.Data), conv.Uuid.String()) {
		t.Fatalf("visitor should see the agent's conversation: %d %+v", list.Code, list.Data)
	}
	// The inbox filter and the conversation carry the identity (CF-195).
	verifiedList, _, err := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{IdentityState: "verified", Limit: 10})
	if err != nil || len(verifiedList) != 1 || verifiedList[0].UUID != conv.Uuid {
		t.Fatalf("verified filter = %d %v", len(verifiedList), err)
	}
	if c := verifiedList[0].Contact; c.ExternalID == nil || *c.ExternalID != "u-500" || c.IdentityState != "verified" {
		t.Fatalf("conversation contact = %+v", c)
	}
	if anon, _, _ := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{IdentityState: "anonymous", Limit: 10}); len(anon) != 0 {
		t.Fatalf("anonymous filter should not list the verified conversation: %d", len(anon))
	}
	var payload string
	if err := e.pool.QueryRow(ctx, "SELECT payload::text FROM outbox_events WHERE organization_id = $1 AND payload::text LIKE '%' || $2::text || '%' ORDER BY id LIMIT 1", e.org, conv.Uuid.String()).Scan(&payload); err != nil || !strings.Contains(payload, `"contact_external_id": "u-500"`) {
		t.Fatalf("event payload = %s %v", payload, err)
	}
	// A second, anonymous browser stays anonymous.
	other := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	if r := e.call(http.MethodPost, "/conversations", other, siteOrigin, map[string]any{"text": "merhaba"}); r.Code != http.StatusCreated {
		t.Fatalf("anonymous conversation = %d %v", r.Code, r.Err)
	}
	var unverified int
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM contacts WHERE organization_id = $1 AND verified_at IS NULL AND external_id IS NULL AND deleted_at IS NULL", e.org).Scan(&unverified); err != nil || unverified != 1 {
		t.Fatalf("anonymous contacts = %d %v", unverified, err)
	}
}

// CF-195: a signed-in user whose e-mail is on another user's contact gets
// a contact of its own; the other contact is neither taken over nor
// marked verified.
func TestIdentifyDoesNotTakeOverAnotherUsersContact(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, "INSERT INTO contacts (organization_id, name, email, external_id) VALUES ($1, 'Başka', 'ortak@example.com', 'u-1')", e.org); err != nil {
		t.Fatal(err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	user := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{
		"user_id": "u-2", "user_hash": widget.UserHash(identitySecret, "u-2"), "email": "ortak@example.com",
	})
	if user.Code != http.StatusOK {
		t.Fatalf("identify = %d %v", user.Code, user.Err)
	}
	var verified bool
	if err := e.pool.QueryRow(ctx, "SELECT verified_at IS NOT NULL FROM contacts WHERE organization_id = $1 AND external_id = 'u-1'", e.org).Scan(&verified); err != nil || verified {
		t.Fatalf("another user's contact verified=%v %v", verified, err)
	}
	var owner string
	if err := e.pool.QueryRow(ctx, "SELECT COALESCE(c.external_id, '') FROM contact_identities ci JOIN contacts c ON c.id = ci.contact_id WHERE ci.organization_id = $1 AND ci.external_id = 'user:u-2'", e.org).Scan(&owner); err != nil || owner != "u-2" {
		t.Fatalf("u-2 identity belongs to %q %v", owner, err)
	}
}

// CF-195: a widget whose identify calls are rejected, or that has no
// identity secret, warns on its channel health with the reason.
func TestWidgetIdentityHealthWarning(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	v, err := e.ib.Channels.Get(ctx, e.org, e.inst.UUID)
	if err != nil || v.Health.State == "warning" {
		t.Fatalf("healthy widget = %+v %v", v.Health, err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	if r := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{"user_id": "u-1", "user_hash": "00"}); r.Code != http.StatusUnauthorized {
		t.Fatalf("wrong hash = %d", r.Code)
	}
	v, _ = e.ib.Channels.Get(ctx, e.org, e.inst.UUID)
	if v.Health.State != "warning" || v.Health.Detail != "identity_rejected:WIDGET_IDENTITY_INVALID" {
		t.Fatalf("after a rejected identify = %+v", v.Health)
	}
	if r := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{"user_id": "u-1", "user_hash": widget.UserHash(identitySecret, "u-1")}); r.Code != http.StatusOK {
		t.Fatalf("right hash = %d", r.Code)
	}
	v, _ = e.ib.Channels.Get(ctx, e.org, e.inst.UUID)
	if v.Health.State == "warning" {
		t.Fatalf("a verified identify clears the warning: %+v", v.Health)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO channel_instances (organization_id, channel_type, name) VALUES ($1, 'widget', 'Anonim')", e.org); err != nil {
		t.Fatal(err)
	}
	list, err := e.ib.Channels.List(ctx, e.org, "widget")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range list {
		if it.Name == "Anonim" && (it.Health.State != "warning" || it.Health.Detail != "identity_secret_missing") {
			t.Fatalf("widget without secret = %+v", it.Health)
		}
	}
}

func listUUIDs(data map[string]any) string {
	var b strings.Builder
	items, _ := data["items"].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			s, _ := m["uuid"].(string)
			b.WriteString(s + " ")
		}
	}
	return b.String()
}
