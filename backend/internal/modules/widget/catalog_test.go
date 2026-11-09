package widget_test

import (
	"context"
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	helpusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/usecase"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	kbrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"net/http"
	"testing"

	cfmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/model"
	cfrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/repository"
	cfusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

// CF-196: defining a field is the only step. The widget writes it by key
// (no attribute mapping, no publish); a key no field defines is recorded as
// a warning with its value, and defining it later applies that value.
func TestDefineSendDone(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	fields := cfusecase.New(cfrepository.New(e.pool, e.q))
	e.svc.WithFields(fields)

	if _, err := fields.CreateDefinition(ctx, e.org, cfmodel.CreateDefinitionInput{
		EntityType: customfields.EntityContact, Key: "paket", Label: "Paket", Type: customfields.TypeSelect,
		Options:      []customfields.Option{{Value: "pro", Label: "Pro Paket"}, {Value: "basic", Label: "Temel"}},
		WidgetAccess: customfields.WidgetAccessAnonymous,
	}); err != nil {
		t.Fatal(err)
	}
	// Verified-only by default: an anonymous visitor cannot write it.
	if _, err := fields.CreateDefinition(ctx, e.org, cfmodel.CreateDefinitionInput{
		EntityType: customfields.EntityContact, Key: "kredi_limiti", Label: "Kredi limiti", Type: customfields.TypeNumber,
	}); err != nil {
		t.Fatal(err)
	}

	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil)
	token := str(boot.Data, "visitor", "token")
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "selam", "name": "Ada"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", created.Code, created.Err)
	}
	r := e.call(http.MethodPatch, "/attributes", token, siteOrigin, map[string]any{"attributes": map[string]any{
		"paket": "Pro Paket", "kredi_limiti": 5000, "lisans_bitis": "2026-12-01",
	}})
	if r.Code != http.StatusOK {
		t.Fatalf("attributes = %d %v", r.Code, r.Err)
	}
	if got := r.Data["applied"].([]any); len(got) != 1 || got[0] != "paket" {
		t.Fatalf("applied = %v", r.Data)
	}
	if got := r.Data["unknown"].([]any); len(got) != 1 || got[0] != "lisans_bitis" {
		t.Fatalf("unknown = %v", r.Data)
	}

	var contactUUID uuid.UUID
	var custom []byte
	if err := e.pool.QueryRow(ctx, "SELECT ct.uuid, ct.custom_fields FROM conversations c JOIN contacts ct ON ct.id = c.contact_id WHERE c.uuid = $1",
		str(created.Data, "conversation_uuid")).Scan(&contactUUID, &custom); err != nil {
		t.Fatal(err)
	}
	var cf map[string]any
	_ = json.Unmarshal(custom, &cf)
	if cf["paket"] != "pro" || cf["kredi_limiti"] != nil {
		t.Fatalf("custom fields = %v (the label must be stored as the option id)", cf)
	}

	// The warning shows up organization-wide, for the widget and on the contact.
	keys, err := fields.ListUnknownKeys(ctx, e.org, &e.inst.UUID)
	if err != nil || len(keys) != 1 || keys[0].Key != "lisans_bitis" || keys[0].ValueKind != "date" {
		t.Fatalf("unknown keys = %+v %v", keys, err)
	}
	values, err := fields.EntityValues(ctx, e.org, customfields.EntityContact, contactUUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(values.Unknown) != 1 || values.Unknown[0].Key != "lisans_bitis" || values.Unknown[0].Value != "2026-12-01" {
		t.Fatalf("contact unknown = %+v", values.Unknown)
	}
	var paket *cfmodel.EntityFieldValue
	for i := range values.Fields {
		if values.Fields[i].Key == "paket" {
			paket = &values.Fields[i]
		}
	}
	if paket == nil || paket.Value != "pro" || paket.Display != "Pro Paket" || !paket.Visibility.AI {
		t.Fatalf("paket value = %+v", paket)
	}

	// One click: define it from the warning; the received value is applied.
	def, err := fields.CreateDefinition(ctx, e.org, cfmodel.CreateDefinitionInput{
		EntityType: customfields.EntityContact, Key: keys[0].SuggestedKey, Label: "Lisans bitiş", Type: customfields.TypeDate,
		WidgetAccess: customfields.WidgetAccessAnonymous, FromUnknownKey: keys[0].Key,
	})
	if err != nil {
		t.Fatal(err)
	}
	adopted, err := fields.Adopt(ctx, e.org, def, keys[0].Key)
	if err != nil || adopted != 1 {
		t.Fatalf("adopted = %d %v", adopted, err)
	}
	if keys, _ := fields.ListUnknownKeys(ctx, e.org, nil); len(keys) != 0 {
		t.Fatalf("warning still open: %+v", keys)
	}
	if err := e.pool.QueryRow(ctx, "SELECT custom_fields FROM contacts WHERE uuid = $1", contactUUID).Scan(&custom); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(custom, &cf)
	if cf["lisans_bitis"] != "2026-12-01" {
		t.Fatalf("adopted value = %v", cf)
	}
	// From now on the widget writes it directly.
	if r := e.call(http.MethodPatch, "/attributes", token, siteOrigin, map[string]any{"attributes": map[string]any{"lisans_bitis": "2027-01-15"}}); len(r.Data["applied"].([]any)) != 1 {
		t.Fatalf("defined key = %v", r.Data)
	}
}

// CF-196: an article's audience picks catalog options by id; the widget
// offers it only to visitors whose contact holds one of them.
func TestHelpArticleAudience(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	fields := cfusecase.New(cfrepository.New(e.pool, e.q))
	e.svc.WithFields(fields)
	if _, err := fields.CreateDefinition(ctx, e.org, cfmodel.CreateDefinitionInput{
		EntityType: customfields.EntityContact, Key: "paket", Label: "Paket", Type: customfields.TypeSelect,
		Options:      []customfields.Option{{Value: "pro", Label: "Pro Paket"}, {Value: "basic", Label: "Temel"}},
		WidgetAccess: customfields.WidgetAccessAnonymous,
	}); err != nil {
		t.Fatal(err)
	}
	kb := kbusecase.New(kbrepo.New(e.pool))
	kb.Fields = fields
	e.h.WithHelp(helpusecase.New(db.New(e.pool), nil, nil, "secret"), "https://app.clickfix.test/")
	add := func(slug, title string) uuid.UUID {
		a, err := kb.SaveArticle(ctx, e.org, e.agent, uuid.New(), kbmodel.ArticleInput{Slug: slug, Locale: "tr", Title: title, Body: "Kurulum", Visibility: "public"}, true)
		if err != nil {
			t.Fatal(err)
		}
		if err = kb.Publish(ctx, e.org, a.Uuid, a.Version, true); err != nil {
			t.Fatal(err)
		}
		return a.Uuid
	}
	add("genel-kurulum", "Genel kurulum")
	pro := add("pro-kurulum", "Pro kurulum")
	aud, err := kb.SetArticleAudience(ctx, e.org, pro, kbusecase.Audience{Conditions: []customfields.FieldMatch{
		{Entity: customfields.EntityContact, Key: "paket", Options: []string{"Pro Paket"}},
	}})
	if err != nil || aud.Conditions[0].Options[0] != "pro" {
		t.Fatalf("audience = %+v %v", aud, err)
	}
	titles := func(token string) []string {
		t.Helper()
		r := e.call(http.MethodGet, "/help/search?q=kurulum", token, siteOrigin, nil)
		out := []string{}
		for _, it := range r.Data["items"].([]any) {
			out = append(out, it.(map[string]any)["title"].(string))
		}
		return out
	}
	if got := titles(""); len(got) != 1 || got[0] != "Genel kurulum" {
		t.Fatalf("anonymous visitor sees %v", got)
	}
	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil)
	token := str(boot.Data, "visitor", "token")
	if r := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "selam", "attributes": map[string]any{"paket": "pro"}}); r.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", r.Code, r.Err)
	}
	if got := titles(token); len(got) != 2 {
		t.Fatalf("pro visitor sees %v", got)
	}
}

// CF-196: the greeting renders {{contact.<key>}} with the option label of
// the catalog; sensitive fields and visitors without a contact render empty.
func TestGreetingUsesCatalogLabels(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	fields := cfusecase.New(cfrepository.New(e.pool, e.q))
	e.svc.WithFields(fields)
	for _, in := range []cfmodel.CreateDefinitionInput{
		{EntityType: customfields.EntityContact, Key: "paket", Label: "Paket", Type: customfields.TypeSelect,
			Options: []customfields.Option{{Value: "pro", Label: "Pro Paket"}}, WidgetAccess: customfields.WidgetAccessAnonymous},
		{EntityType: customfields.EntityContact, Key: "tc", Label: "TC", Type: customfields.TypeText, Sensitive: true, WidgetAccess: customfields.WidgetAccessAnonymous},
	} {
		if _, err := fields.CreateDefinition(ctx, e.org, in); err != nil {
			t.Fatal(err)
		}
	}
	cfgs := usecase.NewConfigs(e.q, convrepository.New(e.pool, e.q), e.ib.Channels)
	settings := map[string]any{
		"allowed_domains": []any{"shop.example.com"},
		"greeting_text":   "Merhaba {{contact.first_name}}, {{contact.paket}} paketiniz hazır. {{contact.tc}}",
	}
	if issues := widget.Validate(settings); len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
	if _, err := cfgs.Publish(ctx, e.org, e.agent, e.inst.UUID, usecase.PublishInput{Settings: settings}); err != nil {
		t.Fatal(err)
	}
	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil)
	if got := str(boot.Data, "widget", "settings", "greeting_text"); got != "Merhaba, paketiniz hazır." {
		t.Fatalf("anonymous greeting = %q", got)
	}
	token := str(boot.Data, "visitor", "token")
	if r := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{
		"text": "selam", "name": "Ada Yılmaz", "attributes": map[string]any{"paket": "pro", "tc": "12345678901"},
	}); r.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", r.Code, r.Err)
	}
	again := e.call(http.MethodPost, "/bootstrap", token, siteOrigin, nil)
	if got := str(again.Data, "widget", "settings", "greeting_text"); got != "Merhaba Ada, Pro Paket paketiniz hazır." {
		t.Fatalf("greeting = %q", got)
	}
	// The cached settings keep the template for the next visitor.
	if got := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "widget", "settings", "greeting_text"); got != "Merhaba, paketiniz hazır." {
		t.Fatalf("template leaked between visitors: %q", got)
	}
}
