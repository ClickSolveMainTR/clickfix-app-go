package assistant_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	contactsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts"
	contactshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/handler"
	contactsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// CF-211 tenant isolation on the real routes: a member of organization A
// cannot read or change organization B's contact from the chat, even with
// B's uuid in hand; the organization comes from the caller, never from
// the model.
func TestCF211ToolsStayInTheCallersOrganization(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	other, err := orgusecase.New(orgrepository.NewPostgres(f.pool, f.q), nil).
		CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Diğer " + uuid.NewString()[:6], OwnerUserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		for _, stmt := range []string{"DELETE FROM contacts WHERE organization_id = ANY($1)", "DELETE FROM activity_events WHERE organization_id = ANY($1)",
			"DELETE FROM domain_event_log WHERE organization_id = ANY($1)", "DELETE FROM outbox_events WHERE organization_id = ANY($1)"} {
			_, _ = f.pool.Exec(c, stmt, []int64{f.scope.ID, other.ID})
		}
		_, _ = f.pool.Exec(c, "DELETE FROM organizations WHERE id = $1", other.ID)
	})
	var mine, theirs uuid.UUID
	if err := f.pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Ayşe A') RETURNING uuid", f.scope.ID).Scan(&mine); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Bora B') RETURNING uuid", other.ID).Scan(&theirs); err != nil {
		t.Fatal(err)
	}
	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantContactsRead, rbac.PermTenantContactsWrite}
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	contactsmodule.RegisterRoutes(mux, contactshandler.New(contactsusecase.New(convrepository.New(f.pool, f.q)), nil), tokens,
		wfLoader{authctx.Principal{UserID: userUUID, UserInternal: f.user}}, wfResolver{scope})
	bridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).Register(contactsmodule.Capabilities()...)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "web", Route: "/contacts"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	f.prov.steps = []ai.Result{
		{Calls: []ai.ToolCall{
			{ID: "a", Name: "contacts_list", Input: []byte(`{}`)},
			{ID: "b", Name: "contacts_get", Input: []byte(`{"contact_uuid":"` + theirs.String() + `"}`)},
			{ID: "c", Name: "contacts_update", Input: []byte(`{"summary":"x","params":{"contact_uuid":"` + theirs.String() + `","name":"Ele geçirildi"}}`)},
		}},
		{Text: `{"reply":"Tamam.","suggestions":[]}`},
	}
	res, err := f.svc.Send(ctx, c, th.UUID, "Kişileri göster, Bora'yı aç ve adını değiştir", pc)
	if err != nil {
		t.Fatal(err)
	}
	out := toolTexts(f.prov.reqs[1])
	if !strings.Contains(out, "Ayşe A") || strings.Contains(out, "Bora B") {
		t.Fatalf("another organization's contact reached the model: %s", out)
	}
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("reading another organization's contact did not fail: %s", out)
	}
	if len(res.Reply.Actions) != 1 {
		t.Fatalf("actions %v", res.Reply.Actions)
	}
	_, _ = f.svc.Confirm(ctx, c, th.UUID, res.Reply.Actions[0].ID, nil)
	var name string
	if err := f.pool.QueryRow(ctx, "SELECT name FROM contacts WHERE uuid = $1", theirs).Scan(&name); err != nil || name != "Bora B" {
		t.Fatalf("another organization's contact changed: %q %v", name, err)
	}
	// The same tool works on the caller's own contact.
	f.prov.steps = []ai.Result{call("d", "contacts_update", `{"summary":"x","params":{"contact_uuid":"`+mine.String()+`","name":"Ayşe Yılmaz"}}`), {Text: `{"reply":"Tamam.","suggestions":[]}`}}
	res, err = f.svc.Send(ctx, c, th.UUID, "Ayşe'nin soyadını Yılmaz yap", pc)
	if err != nil || len(res.Reply.Actions) != 1 {
		t.Fatalf("%v %v", err, res.Reply.Actions)
	}
	if _, err := f.svc.Confirm(ctx, c, th.UUID, res.Reply.Actions[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT name FROM contacts WHERE uuid = $1", mine).Scan(&name); err != nil || name != "Ayşe Yılmaz" {
		t.Fatalf("own contact not updated: %q %v", name, err)
	}
}
