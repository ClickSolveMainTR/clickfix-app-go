package usecase_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/systemstest"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
)

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	p, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func ptr[T any](v T) *T { return &v }

// The accounting scenario of CF-197, step by step through the service:
// concept → endpoint → "Dene" → pick fields → action → playbook → the
// AI context for a customer known by external id.
func TestAccountingScenario(t *testing.T) {
	ctx := context.Background()
	p := pool(t)
	f := systemstest.Seed(t, ctx, p)
	acc := systemstest.NewAccounting(t)
	box, err := crypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(db.New(p), box, nil)
	svc.Caller = acc.Caller()
	svc.ValidateBaseURL = func(string) error { return nil }

	// Connection: the secret is stored encrypted and never returned.
	conn, err := svc.SaveConnection(ctx, f.Org, f.User, model.ConnectionInput{Name: "Muhasebe", BaseURL: acc.URL + "/api", AuthType: model.AuthBearer, Secret: systemstest.Token})
	if err != nil {
		t.Fatal(err)
	}
	if !conn.HasSecret {
		t.Fatal("secret not stored")
	}
	var stored string
	if err := p.QueryRow(ctx, "SELECT auth_enc FROM integration_connections WHERE organization_id=$1", f.Org).Scan(&stored); err != nil || stored == "" || strings.Contains(stored, systemstest.Token) {
		t.Fatalf("secret stored in clear: %v %q", err, stored)
	}
	// Re-saving without a secret keeps the stored one.
	if conn, err = svc.SaveConnection(ctx, f.Org, f.User, model.ConnectionInput{Name: "Muhasebe", BaseURL: acc.URL + "/api", AuthType: model.AuthBearer, KeepSecret: true}); err != nil || !conn.HasSecret {
		t.Fatalf("keep secret: %v %+v", err, conn)
	}

	// "Dene" with an unsaved path: the real answer and pickable paths.
	test, err := svc.TestSource(ctx, f.Org, f.User, f.ObjectTypeUUID, model.TestInput{ExternalID: "CUST-42", PathTemplate: ptr("/faturalar?musteri={external_id}"), ItemsPath: ptr("data.faturalar")})
	if err != nil {
		t.Fatal(err)
	}
	if !test.OK || test.StatusCode != 200 || len(test.Items) != 2 {
		t.Fatalf("test: %+v", test)
	}
	if strings.Contains(test.Request, systemstest.Token) {
		t.Fatalf("token in request: %s", test.Request)
	}
	picked := map[string]bool{}
	for _, opt := range test.Paths {
		picked[opt.Path] = true
	}
	if !picked["durum"] || !picked["hata.metin"] || !picked["no"] || !picked["data.faturalar"] {
		t.Fatalf("paths: %+v", test.Paths)
	}

	// Pick fields from the answer and save the source.
	if _, err := svc.SaveSource(ctx, f.Org, f.ObjectTypeUUID, model.SourceInput{PathTemplate: "/faturalar?musteri={company_external_id}", IdentityKey: model.KeyExternalID, Enabled: true}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("template without the identity placeholder accepted: %v", err)
	}
	if _, err := svc.SaveSource(ctx, f.Org, f.ObjectTypeUUID, model.SourceInput{PathTemplate: "/faturalar?musteri={external_id}", ItemsPath: "data.faturalar", Enabled: true,
		Fields: []model.Field{{Path: "no", Label: "Fatura no"}, {Path: "durum", Label: "Durum"}, {Path: "hata.metin", Label: "Hata metni"}}}); err != nil {
		t.Fatal(err)
	}

	// Action: AI may resend without approval; members too.
	if _, err := svc.SaveAction(ctx, f.Org, "fatura_tekrar_gonder", model.ActionInput{Title: "Faturayı tekrar gönder", ConceptKey: "fatura", Method: "POST",
		PathTemplate: "/faturalar/{fatura_no}/tekrar-gonder", Params: []model.Param{{Key: "fatura_no", Label: "Fatura no", Type: "string", Required: true}},
		AllowedFor: []string{model.ChannelAI, model.ChannelAgent}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// An approval-requiring refund that only members may run.
	if _, err := svc.SaveAction(ctx, f.Org, "iade_baslat", model.ActionInput{Title: "İade başlat", Method: "POST", PathTemplate: "/iadeler",
		RequiresApproval: true, AllowedFor: []string{model.ChannelAgent}, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// Playbook in plain words.
	if _, err := svc.SavePlaybook(ctx, f.Org, "fatura_gelmedi", model.PlaybookInput{Title: "Faturam gelmedi",
		Instructions: "Müşteri faturasının gitmediğini söylerse Fatura'ya bak; durum Hata ise hata metnini oku, faturayı tekrar gönder ve müşteriye bildir.",
		ConceptKeys:  []string{"fatura"}, ActionKeys: []string{"fatura_tekrar_gonder"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// Lookup by external id only and by contact: same customer.
	id, err := svc.ResolveIdentity(ctx, f.Org, nil, "CUST-42")
	if err != nil || id.ContactUUID == nil || *id.ContactUUID != f.ContactUUID || id.CompanyExternalID != "COMP-7" {
		t.Fatalf("identity: %v %+v", err, id)
	}
	l, err := svc.Lookup(ctx, f.Org, f.User, "fatura", id, model.ChannelAgent)
	if err != nil || l.Status != "ok" || len(l.Items) != 2 || l.Items[0]["Hata metni"] != "Alıcı e-posta adresi reddetti" || l.Items[0]["ic_kod"] != nil {
		t.Fatalf("lookup: %v %+v", err, l)
	}
	// A customer without an external id is skipped with the reason.
	if l, err = svc.Lookup(ctx, f.Org, f.User, "fatura", model.Identity{}, model.ChannelAgent); err != nil || l.Status != "skipped" || !strings.Contains(l.Reason, "IDENTITY_MISSING") {
		t.Fatalf("anonymous lookup: %v %+v", err, l)
	}

	// Action rules.
	if _, err := svc.Run(ctx, f.Org, f.User, "iade_baslat", model.ChannelAI, model.RunInput{ContactUUID: &f.ContactUUID}); !errors.Is(err, model.ErrNotAllowed) {
		t.Fatalf("ai ran an agent-only action: %v", err)
	}
	if _, err := svc.Run(ctx, f.Org, f.User, "iade_baslat", model.ChannelAgent, model.RunInput{ContactUUID: &f.ContactUUID}); !errors.Is(err, model.ErrNeedsApproval) {
		t.Fatalf("unapproved run: %v", err)
	}
	if _, err := svc.Run(ctx, f.Org, f.User, "fatura_tekrar_gonder", model.ChannelWorkflow, model.RunInput{ContactUUID: &f.ContactUUID, Params: map[string]any{"fatura_no": "F-2026-101"}}); !errors.Is(err, model.ErrNotAllowed) {
		t.Fatalf("workflow ran an action not allowed for workflows: %v", err)
	}
	if _, err := svc.Run(ctx, f.Org, f.User, "fatura_tekrar_gonder", model.ChannelAgent, model.RunInput{ContactUUID: &f.ContactUUID}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("missing param accepted: %v", err)
	}
	dry, err := svc.Run(ctx, f.Org, f.User, "fatura_tekrar_gonder", model.ChannelAgent, model.RunInput{ContactUUID: &f.ContactUUID, Params: map[string]any{"fatura_no": "F-2026-101"}, DryRun: true})
	if err != nil || !dry.DryRun || !strings.Contains(dry.Request, "/faturalar/F-2026-101/tekrar-gonder") || len(acc.Resent) != 0 {
		t.Fatalf("dry run: %v %+v resent=%v", err, dry, acc.Resent)
	}
	run, err := svc.Run(ctx, f.Org, f.User, "fatura_tekrar_gonder", model.ChannelAgent, model.RunInput{ConversationUUID: &f.ConvUUID, Params: map[string]any{"fatura_no": "F-2026-101"}})
	if err != nil || !run.OK || len(acc.Resent) != 1 || acc.Resent[0] != "F-2026-101" {
		t.Fatalf("run: %v %+v resent=%v", err, run, acc.Resent)
	}

	// The AI side: intents for triage, context for the conversation.
	intents, err := svc.Intents(ctx, f.Org)
	if err != nil || len(intents) != 1 || !strings.Contains(usecase.TriageHint(intents), `"fatura_gelmedi"`) {
		t.Fatalf("intents: %v %+v", err, intents)
	}
	ac, err := svc.Context(ctx, f.Org, f.Conversation, "fatura_gelmedi")
	if err != nil || !ac.Grounded || !strings.Contains(ac.Text, "F-2026-101") || !strings.Contains(ac.Text, "untrusted") || strings.Contains(ac.Text, "x-991") {
		t.Fatalf("ai context: %v %+v", err, ac)
	}
	tools, err := svc.Tools(ctx, f.Org, ac.Playbook.ActionKeys)
	if err != nil || len(tools) != 1 || tools[0].Name != "system_fatura_tekrar_gonder" {
		t.Fatalf("tools: %v %+v", err, tools)
	}

	// "Bu müşteri olarak dene": checks, screen and the AI context.
	pv, err := svc.Preview(ctx, f.Org, model.PreviewInput{ExternalID: "CUST-42", Attributes: map[string]any{"plan": "Pro", "nested": map[string]any{"x": 1}},
		PageURL: "https://app.example.com/faturalar/F-2026-101?session=abc", Intent: "fatura_gelmedi"})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, c := range pv.Checks {
		status[c.Key] = c.Status
	}
	if status["external_id"] != "accepted" || status["attributes.plan"] != "accepted" || status["attributes.nested"] != "rejected" || status["page_url"] != "accepted" {
		t.Fatalf("checks: %+v", pv.Checks)
	}
	if pv.Screen == nil || pv.Screen.PagePath != "/faturalar/F-2026-101" || !strings.Contains(pv.AIContext, "F-2026-101") || strings.Contains(pv.AIContext, "session=abc") {
		t.Fatalf("preview: %+v", pv)
	}
	anon, err := svc.Preview(ctx, f.Org, model.PreviewInput{Intent: "fatura_gelmedi"})
	if err != nil || anon.Checks[0].Status != "rejected" || anon.Lookups[0].Status != "skipped" {
		t.Fatalf("anonymous preview: %v %+v", err, anon)
	}

	// Calls are logged with masked paths and no bodies.
	ov, err := svc.Overview(ctx, f.Org)
	if err != nil || len(ov.Calls) == 0 || len(ov.Concepts) != 1 || ov.Concepts[0].Source == nil || len(ov.Actions) != 2 || len(ov.Playbooks) != 1 {
		t.Fatalf("overview: %v %+v", err, ov)
	}
	for _, c := range ov.Calls {
		if strings.Contains(c.MaskedPath, systemstest.Token) {
			t.Fatalf("token logged: %+v", c)
		}
	}
}
