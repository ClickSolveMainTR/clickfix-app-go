package assistant_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

const (
	u1 = "74da01e6-edfd-4ac5-959c-e714ccac0ba2"
	u2 = "0b6f3c55-1d2e-4c7a-9a1b-2f3e4d5c6b7a"
)

// recAPI records /v1 calls and their bodies.
type recAPI struct {
	mu     sync.Mutex
	calls  []string
	bodies []map[string]any
	data   string
}

func (a *recAPI) Call(_ context.Context, method, path string, body any) (capability.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, method+" "+path)
	m := map[string]any{}
	if body != nil {
		raw, _ := json.Marshal(body)
		_ = json.Unmarshal(raw, &m)
	}
	a.bodies = append(a.bodies, m)
	data := a.data
	if data == "" {
		data = `{"items":[{"uuid":"` + u1 + `","name":"Örnek"}],"total":1}`
	}
	return capability.Response{OK: true, Status: 200, Data: json.RawMessage(data)}, nil
}

// cf211Case is one tool: sample params, the /v1 call it must make and,
// for writes, body fields it must send.
type cf211Case struct {
	params string
	call   string
	body   map[string]any
}

var cf211Cases = map[string]cf211Case{
	// Inbox.
	"conversations.search":       {`{"q":"iade","status":"open","assignee":"me"}`, "GET /v1/tenant/conversations?assignee=me&q=iade&status=open", nil},
	"conversations.counts":       {`{}`, "GET /v1/tenant/conversations/counts", nil},
	"conversations.get":          {`{"conversation_uuid":"` + u1 + `"}`, "GET /v1/tenant/conversations/" + u1, nil},
	"conversations.messages":     {`{"conversation_uuid":"` + u1 + `","limit":20}`, "GET /v1/tenant/conversations/" + u1 + "/messages?limit=20", nil},
	"conversations.sla":          {`{"conversation_uuid":"` + u1 + `"}`, "GET /v1/tenant/conversations/" + u1 + "/sla", nil},
	"conversations.agents":       {`{}`, "GET /v1/tenant/conversations/agents", nil},
	"tags.list":                  {`{}`, "GET /v1/tenant/tags", nil},
	"conversations.reply":        {`{"conversation_uuid":"` + u1 + `","content":"Merhaba"}`, "POST /v1/tenant/conversations/" + u1 + "/messages", map[string]any{"content": "Merhaba"}},
	"conversations.add_note":     {`{"conversation_uuid":"` + u1 + `","content":"İade onaylandı"}`, "POST /v1/tenant/conversations/" + u1 + "/messages", map[string]any{"private": true, "content": "İade onaylandı"}},
	"conversations.assign":       {`{"conversation_uuid":"` + u1 + `","user_uuid":"` + u2 + `"}`, "PATCH /v1/tenant/conversations/" + u1, map[string]any{"assignee": u2}},
	"conversations.set_status":   {`{"conversation_uuid":"` + u1 + `","status":"resolved"}`, "PATCH /v1/tenant/conversations/" + u1, map[string]any{"status": "resolved"}},
	"conversations.set_priority": {`{"conversation_uuid":"` + u1 + `","priority":"urgent"}`, "PATCH /v1/tenant/conversations/" + u1, map[string]any{"priority": "urgent"}},
	"conversations.add_tag":      {`{"conversation_uuid":"` + u1 + `","tag_uuid":"` + u2 + `"}`, "POST /v1/tenant/conversations/" + u1 + "/tags", map[string]any{"uuid": u2}},
	"conversations.remove_tag":   {`{"conversation_uuid":"` + u1 + `","tag_uuid":"` + u2 + `"}`, "DELETE /v1/tenant/conversations/" + u1 + "/tags/" + u2, nil},
	"conversations.apply_macro":  {`{"conversation_uuid":"` + u1 + `","macro_uuid":"` + u2 + `"}`, "POST /v1/tenant/conversations/" + u1 + "/macros/" + u2, nil},
	"conversations.remind":       {`{"conversation_uuid":"` + u1 + `","due_at":"2030-01-01T09:00:00Z"}`, "POST /v1/tenant/conversations/" + u1 + "/reminders", map[string]any{"due_at": "2030-01-01T09:00:00Z"}},
	"tags.create":                {`{"name":"VIP"}`, "POST /v1/tenant/tags", map[string]any{"name": "VIP"}},
	// Contacts and companies.
	"contacts.list":      {`{"q":"ayşe","limit":10}`, "GET /v1/tenant/contacts?limit=10&q=ay%C5%9Fe", nil},
	"contacts.get":       {`{"contact_uuid":"` + u1 + `"}`, "GET /v1/tenant/contacts/" + u1, nil},
	"contacts.create":    {`{"name":"Ayşe","email":"ayse@example.test"}`, "POST /v1/tenant/contacts", map[string]any{"name": "Ayşe"}},
	"contacts.update":    {`{"contact_uuid":"` + u1 + `","phone":"+905551112233"}`, "PATCH /v1/tenant/contacts/" + u1, map[string]any{"phone": "+905551112233"}},
	"contacts.delete":    {`{"contact_uuid":"` + u1 + `"}`, "DELETE /v1/tenant/contacts/" + u1, nil},
	"companies.list":     {`{"q":"acme"}`, "GET /v1/tenant/companies?q=acme", nil},
	"companies.get":      {`{"company_uuid":"` + u1 + `"}`, "GET /v1/tenant/companies/" + u1 + "/overview", nil},
	"companies.create":   {`{"name":"Acme","domain":"acme.test"}`, "POST /v1/tenant/companies", map[string]any{"domain": "acme.test"}},
	"companies.update":   {`{"company_uuid":"` + u1 + `","tier":"gold"}`, "PATCH /v1/tenant/companies/" + u1, map[string]any{"tier": "gold"}},
	"companies.add_note": {`{"company_uuid":"` + u1 + `","body":"Yenileme görüşmesi"}`, "POST /v1/tenant/companies/" + u1 + "/notes", map[string]any{"body": "Yenileme görüşmesi"}},
	// Macros and saved replies.
	"macros.list":             {`{}`, "GET /v1/tenant/macros", nil},
	"macros.create":           {`{"name":"İade","actions":[{"type":"tag","value":"iade"}]}`, "POST /v1/tenant/macros", map[string]any{"name": "İade"}},
	"macros.update":           {`{"macro_uuid":"` + u1 + `","name":"İade","actions":[{"type":"status","value":"resolved"}]}`, "PATCH /v1/tenant/macros/" + u1, map[string]any{"name": "İade"}},
	"macros.delete":           {`{"macro_uuid":"` + u1 + `"}`, "DELETE /v1/tenant/macros/" + u1, nil},
	"canned_responses.list":   {`{"q":"iade"}`, "GET /v1/tenant/canned-responses?q=iade", nil},
	"canned_responses.create": {`{"title":"İade","shortcut":"iade","content":"Merhaba {{contact.first_name}}"}`, "POST /v1/tenant/canned-responses", map[string]any{"shortcut": "iade"}},
	"canned_responses.update": {`{"canned_uuid":"` + u1 + `","title":"İade","shortcut":"iade","content":"Merhaba"}`, "PATCH /v1/tenant/canned-responses/" + u1, map[string]any{"content": "Merhaba"}},
	// Knowledge base.
	"knowledge.search":           {`{"q":"iade süresi"}`, "GET /v1/tenant/knowledge/search?q=iade+s%C3%BCresi", nil},
	"knowledge.articles.list":    {`{"state":"draft"}`, "GET /v1/tenant/knowledge/articles?state=draft", nil},
	"knowledge.articles.get":     {`{"article_uuid":"` + u1 + `"}`, "GET /v1/tenant/knowledge/articles/" + u1, nil},
	"knowledge.collections.list": {`{}`, "GET /v1/tenant/knowledge/collections", nil},
	"knowledge.reviews.list":     {`{}`, "GET /v1/tenant/knowledge/reviews", nil},
	"knowledge.articles.create":  {`{"title":"İade","slug":"iade","locale":"tr","visibility":"public","body":"14 gün"}`, "POST /v1/tenant/knowledge/articles", map[string]any{"slug": "iade"}},
	"knowledge.articles.update":  {`{"article_uuid":"` + u1 + `","version":2,"title":"İade","slug":"iade","locale":"tr","visibility":"public","body":"14 gün"}`, "PUT /v1/tenant/knowledge/articles/" + u1, map[string]any{"version": float64(2)}},
	"knowledge.articles.publish": {`{"article_uuid":"` + u1 + `","version":2,"publish":true}`, "POST /v1/tenant/knowledge/articles/" + u1 + "/publish", map[string]any{"publish": true}},
	// Appointments.
	"appointments.upcoming":   {`{"scope":"all"}`, "GET /v1/tenant/appointments/upcoming?scope=all", nil},
	"appointments.list":       {`{"from":"2030-01-01T00:00:00Z","to":"2030-01-08T00:00:00Z"}`, "GET /v1/tenant/appointments?from=2030-01-01T00%3A00%3A00Z&to=2030-01-08T00%3A00%3A00Z", nil},
	"appointments.types":      {`{}`, "GET /v1/tenant/appointment-types", nil},
	"appointments.slots":      {`{"type_uuid":"` + u1 + `","from":"2030-01-01","to":"2030-01-07"}`, "GET /v1/tenant/appointment-types/" + u1 + "/slots?from=2030-01-01&to=2030-01-07", nil},
	"appointments.create":     {`{"type_uuid":"` + u1 + `","starts_at":"2030-01-02T10:00:00Z"}`, "POST /v1/tenant/appointments", map[string]any{"type_uuid": u1}},
	"appointments.reschedule": {`{"appointment_uuid":"` + u1 + `","starts_at":"2030-01-03T10:00:00Z"}`, "POST /v1/tenant/appointments/" + u1 + "/reschedule", map[string]any{"starts_at": "2030-01-03T10:00:00Z"}},
	"appointments.set_status": {`{"appointment_uuid":"` + u1 + `","status":"cancelled","reason":"müşteri iptal etti"}`, "POST /v1/tenant/appointments/" + u1 + "/status", map[string]any{"status": "cancelled"}},
	// Reports.
	"analytics.dashboard": {`{"from":"2030-01-01T00:00:00Z","to":"2030-01-08T00:00:00Z","bucket":"day"}`, "GET /v1/tenant/analytics/dashboard?bucket=day&from=2030-01-01T00%3A00%3A00Z&to=2030-01-08T00%3A00%3A00Z", nil},
	"reports.list":        {`{}`, "GET /v1/tenant/reports", nil},
	"reports.run":         {`{"report_uuid":"` + u1 + `","from":"2030-01-01T00:00:00Z","to":"2030-01-08T00:00:00Z"}`, "POST /v1/tenant/reports/" + u1 + "/evaluate", map[string]any{"from": "2030-01-01T00:00:00Z"}},
	"surveys.results":     {`{"from":"2030-01-01T00:00:00Z","to":"2030-01-08T00:00:00Z"}`, "GET /v1/tenant/surveys/results?from=2030-01-01T00%3A00%3A00Z&to=2030-01-08T00%3A00%3A00Z", nil},
	// Billing (read only).
	"billing.subscription": {`{}`, "GET /v1/tenant/billing/subscription", nil},
	"billing.usage":        {`{}`, "GET /v1/tenant/billing/usage", nil},
	"billing.invoices":     {`{"limit":5}`, "GET /v1/tenant/billing/invoices?limit=5", nil},
	"billing.wallet":       {`{}`, "GET /v1/tenant/billing/wallet", nil},
	// Organization, roles, invitations.
	"organization.get":    {`{}`, "GET /v1/tenant/organization", nil},
	"organization.update": {`{"timezone":"Europe/Istanbul"}`, "PATCH /v1/tenant/organization", map[string]any{"timezone": "Europe/Istanbul"}},
	"roles.list":          {`{}`, "GET /v1/tenant/roles", nil},
	"members.set_roles":   {`{"user_uuid":"` + u1 + `","role_uuids":["` + u2 + `"]}`, "PATCH /v1/tenant/members/" + u1, map[string]any{"role_uuids": []any{u2}}},
	"invitations.resend":  {`{"invitation_uuid":"` + u1 + `"}`, "POST /v1/tenant/invitations/" + u1 + "/resend", nil},
	"invitations.revoke":  {`{"invitation_uuid":"` + u1 + `"}`, "POST /v1/tenant/invitations/" + u1 + "/revoke", nil},
	// Audit, custom fields, status page, AI reports.
	"audit.list":         {`{"actor":"` + u1 + `","from":"2030-01-01"}`, "GET /v1/tenant/audit?actor=" + u1 + "&from=2030-01-01", nil},
	"custom_fields.list": {`{"entity_type":"contact"}`, "GET /v1/tenant/custom-fields?entity_type=contact", nil},
	"status.incidents":   {`{}`, "GET /v1/tenant/status/incidents", nil},
	"ai.performance":     {`{}`, "GET /v1/tenant/ai/performance", nil},
	"ai.knowledge_gaps":  {`{"limit":10}`, "GET /v1/tenant/ai/knowledge-gaps?limit=10", nil},
	"ai.knowledge_panel": {`{"from":"2030-01-01T00:00:00Z"}`, "GET /v1/tenant/analytics/ai-knowledge?from=2030-01-01T00%3A00%3A00Z", nil},
}

func params(t *testing.T, raw string) capability.Params {
	t.Helper()
	p := capability.Params{}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// Every CF-211 tool makes exactly its /v1 call; reads go through the
// read-only API (only GET and declared simulations pass).
func TestCF211ToolsCallTheirRoute(t *testing.T) {
	reg := capability.NewRegistry().Register(cf211Capabilities()...)
	for _, cp := range reg.All() {
		tc, ok := cf211Cases[cp.Name]
		if !ok {
			t.Errorf("%s has no test case", cp.Name)
			continue
		}
		t.Run(cp.Name, func(t *testing.T) {
			p := params(t, tc.params)
			if err := cp.Check(p); err != nil {
				t.Fatalf("params rejected: %v", err)
			}
			api := &recAPI{}
			e := capability.Exec{API: api, Locale: "tr"}
			var err error
			if cp.Writes() {
				_, err = cp.Apply(context.Background(), e, p)
			} else {
				e.API = capability.ReadOnly(api, cp.Simulations...)
				_, err = cp.Run(context.Background(), e, p)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(api.calls) != 1 || api.calls[0] != tc.call {
				t.Fatalf("calls %v, want %s", api.calls, tc.call)
			}
			for k, want := range tc.body {
				got, _ := json.Marshal(api.bodies[0][k])
				exp, _ := json.Marshal(want)
				if string(got) != string(exp) {
					t.Fatalf("body %s = %s, want %s (%v)", k, got, exp, api.bodies[0])
				}
			}
		})
	}
}

// Every tool needs a permission, refuses unknown and path-breaking params
// and never takes an organization from the model (the organization is
// the caller's, fixed by the API factory).
func TestCF211ToolsAreGuarded(t *testing.T) {
	reg := capability.NewRegistry().Register(cf211Capabilities()...)
	none := func(string) bool { return false }
	for _, cp := range reg.All() {
		if cp.Permission == "" {
			t.Errorf("%s has no permission", cp.Name)
		}
		if cp.Allowed(none, capability.SurfaceWeb) || cp.Allowed(none, capability.SurfaceMCP) {
			t.Errorf("%s allowed without its permission", cp.Name)
		}
		if !cp.Allowed(func(s string) bool { return s == cp.Permission }, capability.SurfaceWeb) {
			t.Errorf("%s not allowed with %s", cp.Name, cp.Permission)
		}
		p := params(t, cf211Cases[cp.Name].params)
		for _, k := range []string{"organization_uuid", "organization_id", "org"} {
			bad := capability.Params{}
			for kk, v := range p {
				bad[kk] = v
			}
			bad[k] = u2
			if err := cp.Check(bad); !errors.Is(err, capability.ErrInvalid) {
				t.Errorf("%s accepted %s", cp.Name, k)
			}
		}
		for k, v := range p {
			if s, ok := v.(string); ok && s == u1 {
				bad := capability.Params{}
				for kk, vv := range p {
					bad[kk] = vv
				}
				bad[k] = "../../auth/me"
				if cp.Check(bad) == nil {
					t.Errorf("%s accepted a path in %s", cp.Name, k)
				}
			}
		}
	}
	sel := reg.Select(capability.Filter{Surface: capability.SurfaceWeb, Route: "/inbox", Has: none, Max: 5}, []string{"contacts"})
	if len(sel.Loaded)+len(sel.Other) != 0 {
		t.Fatal("a member without permissions was offered tools")
	}
}

// Billing stays read only; destructive tools are the deletes, revokes and
// role changes. MCP has no confirmation card, so destructive tools and the
// organization profile stay in the assistant; every other tool is on both.
func TestCF211Kinds(t *testing.T) {
	reg := capability.NewRegistry().Register(cf211Capabilities()...)
	assistantOnly := []string{"organization.update"}
	destructive := []string{}
	for _, cp := range reg.All() {
		if cp.Module == "billing" && cp.Writes() {
			t.Errorf("billing write %s", cp.Name)
		}
		if cp.Kind == capability.Destructive {
			destructive = append(destructive, cp.Name)
		}
		if strings.HasPrefix(tcMethod(cf211Cases[cp.Name].call), "DELETE") && cp.Kind == capability.Read {
			t.Errorf("%s deletes as a read", cp.Name)
		}
		if !cp.OnSurface(capability.SurfaceWeb) {
			t.Errorf("%s is not in the assistant", cp.Name)
		}
		offMCP := cp.Kind == capability.Destructive || slices.Contains(assistantOnly, cp.Name)
		if cp.OnSurface(capability.SurfaceMCP) == offMCP {
			t.Errorf("%s: on MCP = %v, want %v", cp.Name, cp.OnSurface(capability.SurfaceMCP), !offMCP)
		}
	}
	slices.Sort(destructive)
	want := []string{"contacts.delete", "invitations.revoke", "macros.delete", "members.set_roles"}
	if !slices.Equal(destructive, want) {
		t.Fatalf("destructive %v, want %v", destructive, want)
	}
	// The registry refuses a destructive capability on MCP outright.
	defer func() {
		if recover() == nil {
			t.Fatal("a destructive capability was registered on MCP")
		}
	}()
	bad := *reg.All()[0]
	bad.Name, bad.Kind, bad.Surfaces = "things.wipe", capability.Destructive, capability.ReadSurfaces
	bad.Apply = func(context.Context, capability.Exec, capability.Params) (capability.Result, error) {
		return capability.Result{}, nil
	}
	capability.NewRegistry().Register(&bad)
}

// A write never takes text a read clipped: an article read in full is
// clipped only when it does not fit, and then cannot be written back.
func TestCF211TruncatedTextIsNotWrittenBack(t *testing.T) {
	reg := capability.NewRegistry().Register(cf211Capabilities()...)
	get, _ := reg.Get("knowledge.articles.get")
	upd, _ := reg.Get("knowledge.articles.update")
	if !get.FullText {
		t.Fatal("articles.get is not a full-text read")
	}
	short, _ := capability.CompactFull(map[string]any{"body": strings.Repeat("a", 1500)}, 3000)
	if strings.Contains(short.Data.(map[string]any)["body"].(string), capability.TruncatedMarker) {
		t.Fatal("a body that fits was clipped")
	}
	long, ok := capability.CompactFull(map[string]any{"body": strings.Repeat("ç", 5000), "title": "İade"}, 3000)
	body := long.Data.(map[string]any)["body"].(string)
	if !ok || !strings.Contains(body, capability.TruncatedMarker) {
		t.Fatalf("long body: %v %q", ok, body[len(body)-80:])
	}
	p := params(t, cf211Cases["knowledge.articles.update"].params)
	p["body"] = body
	if err := upd.Check(p); err == nil || !strings.Contains(err.Error(), "shortened") {
		t.Fatalf("clipped body accepted: %v", err)
	}
	delete(p, "body")
	if err := upd.Check(p); err == nil {
		t.Fatal("a full-replace update without body was accepted")
	}
	reply, _ := reg.Get("conversations.reply")
	if reply.Check(capability.Params{"conversation_uuid": u1, "content": "x " + capability.TruncatedMarker + ": 400 of 900]"}) == nil {
		t.Fatal("reply took clipped text")
	}
}

func tcMethod(call string) string { m, _, _ := strings.Cut(call, " "); return m }
