package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

func TestNormalizeDropsBrokenItems(t *testing.T) {
	p := SetupPlan{
		Teams: []PlanTeam{{Name: "Destek Ekibi", Color: "red"}, {Name: "destek ekibi"}, {Name: ""}},
		Tags:  []PlanTag{{Name: "Fatura", Color: "#ABCDEF"}, {Name: "fatura"}},
		Statuses: []PlanStatus{
			{Name: "Bekliyor", Category: "pending"}, {Name: "Yanlış", Category: "nope"},
		},
		CannedReplies: []PlanCannedReply{{Title: "Şifre sıfırlama", Content: "x"}, {Title: "Boş"}},
		Macros: []PlanMacro{
			{Name: "Kapat", Actions: []PlanMacroAction{{Type: "status", Value: "snoozed"}, {Type: "status", Value: "resolved"}}},
			{Name: "Geçersiz", Actions: []PlanMacroAction{{Type: "assign", Value: "me"}}},
		},
		CustomFields: []PlanCustomField{
			{Key: "Sipariş No", Label: "Sipariş", Type: "text", Options: []string{"a"}},
			{Label: "Seçim", Type: "select"},
		},
		Forms: []PlanForm{{Name: "Form", Fields: []PlanFormField{{Field: "email"}, {Field: "custom:siparis_no"}, {Field: "custom:yok"}, {Field: "ssn"}}}},
		RoutingRules: []PlanRoutingRule{
			{Name: "Acil", Team: "destek-ekibi", Priorities: []string{"urgent", "bogus"}},
			{Name: "Yetim", Team: "olmayan"},
		},
		BusinessHours: &PlanBusinessHours{Timezone: "Mars/Olympus", Days: []PlanDay{{Day: "mon", Open: "18:00", Close: "09:00"}, {Day: "tue", Open: "09:00", Close: "17:00"}}},
		Widget:        &PlanWidget{},
	}
	if err := p.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(p.Teams) != 1 || p.Teams[0].Key != "destek-ekibi" || p.Teams[0].Color != "" {
		t.Fatalf("teams %+v", p.Teams)
	}
	if len(p.Tags) != 1 || p.Tags[0].Color != "#abcdef" {
		t.Fatalf("tags %+v", p.Tags)
	}
	if len(p.Statuses) != 1 || p.Statuses[0].Slug != "bekliyor" {
		t.Fatalf("statuses %+v", p.Statuses)
	}
	if len(p.CannedReplies) != 1 || p.CannedReplies[0].Shortcut != "sifre_sifirlama" {
		t.Fatalf("canned %+v", p.CannedReplies)
	}
	if len(p.Macros) != 1 || len(p.Macros[0].Actions) != 1 {
		t.Fatalf("macros %+v", p.Macros)
	}
	if len(p.CustomFields) != 1 || p.CustomFields[0].Key != "siparis_no" || len(p.CustomFields[0].Options) != 0 {
		t.Fatalf("fields %+v", p.CustomFields)
	}
	if got := p.Forms[0].Fields; len(got) != 2 || got[1].Field != "custom:siparis_no" {
		t.Fatalf("form fields %+v", got)
	}
	if len(p.RoutingRules) != 1 || len(p.RoutingRules[0].Priorities) != 1 {
		t.Fatalf("rules %+v", p.RoutingRules)
	}
	bh := p.BusinessHours
	if bh.Timezone != "Europe/Istanbul" || len(bh.Days) != 7 || !bh.Days[0].Closed || bh.Days[1].Closed {
		t.Fatalf("hours %+v", bh)
	}
	if p.Widget != nil {
		t.Fatal("empty widget must be dropped")
	}
	if p.Summary.Locale != "tr" || p.Version != PlanVersion {
		t.Fatalf("summary %+v", p.Summary)
	}
}

func TestNormalizeRejectsEmptyPlan(t *testing.T) {
	p := SetupPlan{Teams: []PlanTeam{{Name: ""}}}
	if err := p.Normalize(); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("got %v", err)
	}
}

func TestItemsAreOrderedAndParented(t *testing.T) {
	p := SetupPlan{
		Teams:        []PlanTeam{{Name: "L1"}},
		RoutingRules: []PlanRoutingRule{{Name: "Hepsi", Team: "l1"}},
		Knowledge:    []PlanCollection{{Name: "SSS", Articles: []PlanArticle{{Title: "Nasıl?", Body: "Böyle."}}}},
	}
	if err := p.Normalize(); err != nil {
		t.Fatal(err)
	}
	items := p.Items()
	idx := map[string]int{}
	for i, it := range items {
		idx[it.Key] = i
	}
	if idx["team:l1"] > idx["routing_rule:hepsi"] || idx["kb_collection:sss"] > idx["kb_article:nasil"] {
		t.Fatalf("order %+v", items)
	}
	for _, it := range items {
		if it.Kind == KindArticle && it.Parent != "kb_collection:sss" {
			t.Fatalf("article parent %q", it.Parent)
		}
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Müşteri Hizmetleri": "musteri-hizmetleri",
		"İade & Değişim!":    "iade-degisim",
		"  ---  ":            "",
		"Çağrı Merkezi 7/24": "cagri-merkezi-7-24",
	} {
		if got := Slugify(in, 60); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeWebsite(t *testing.T) {
	ok := map[string]string{
		"acme.com":                 "https://acme.com/",
		"http://acme.com/about?x":  "https://acme.com/about",
		"https://www.acme.com.tr/": "https://www.acme.com.tr/",
	}
	for in, want := range ok {
		got, err := NormalizeWebsite(in)
		if err != nil || got != want {
			t.Errorf("NormalizeWebsite(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{
		"", "https://localhost/", "https://127.0.0.1/", "https://10.0.0.5/", "https://[::1]/",
		"https://169.254.169.254/latest/meta-data", "https://intranet.internal/", "https://acme.com:8443/",
		"https://user:pass@acme.com/", "ftp://acme.com/", "https://printer.local/",
	} {
		if _, err := NormalizeWebsite(bad); !errors.Is(err, ErrWebsite) {
			t.Errorf("NormalizeWebsite(%q) must be refused, got %v", bad, err)
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func htmlResponse(r *http.Request, status int, body string, header map[string]string) *http.Response {
	h := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	for k, v := range header {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestWebsiteFetcherSummarizes(t *testing.T) {
	pages := map[string]string{
		"/": `<html lang="tr"><head><title>Acme Yazılım</title>
			<meta name="description" content="Bulut muhasebe yazılımı">
			<meta name="theme-color" content="#0F766E"><meta property="og:site_name" content="Acme">
			<script>var secret = 1</script></head>
			<body><nav><a href="/hakkimizda">Hakkımızda</a><a href="https://evil.example/about">x</a></nav>
			<h1>Muhasebeyi kolaylaştırın</h1><p>KOBİ'ler için e-fatura.</p></body></html>`,
		"/hakkimizda": `<html><body><h2>Biz kimiz</h2><p>2015'ten beri.</p></body></html>`,
	}
	var visited []string
	f := &WebsiteFetcher{
		Validate: validateSiteURL,
		Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			visited = append(visited, r.URL.String())
			if r.URL.Path == "/old" {
				return htmlResponse(r, http.StatusMovedPermanently, "", map[string]string{"Location": "https://acme.com/"}), nil
			}
			body, ok := pages[r.URL.Path]
			if !ok {
				return htmlResponse(r, http.StatusNotFound, "", nil), nil
			}
			return htmlResponse(r, http.StatusOK, body, nil), nil
		}), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	sum, err := f.Summarize(context.Background(), "acme.com/old")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Title != "Acme Yazılım" || sum.Description != "Bulut muhasebe yazılımı" || sum.ThemeColor != "#0f766e" || sum.SiteName != "Acme" || sum.Language != "tr" {
		t.Fatalf("summary %+v", sum)
	}
	if len(sum.Pages) != 2 || strings.Contains(sum.Text, "secret") || !strings.Contains(sum.Text, "2015") {
		t.Fatalf("pages %v text %q", sum.Pages, sum.Text)
	}
	for _, v := range visited {
		if u, _ := url.Parse(v); u.Host != "acme.com" {
			t.Fatalf("followed a foreign link: %s", v)
		}
	}
}

func TestWebsiteFetcherRefusesPrivateRedirect(t *testing.T) {
	f := &WebsiteFetcher{
		Validate: validateSiteURL,
		Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			return htmlResponse(r, http.StatusFound, "", map[string]string{"Location": "https://169.254.169.254/"}), nil
		})},
	}
	if _, err := f.Summarize(context.Background(), "https://acme.com/"); !errors.Is(err, ErrWebsite) {
		t.Fatalf("got %v", err)
	}
}

func TestPlanSchemaIsValidAndAcceptsTemplates(t *testing.T) {
	schema := planSchema()
	if err := ai.ValidateSchema(schema); err != nil {
		t.Fatal(err)
	}
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	// A template, filled to the strict shape, is a valid AI answer.
	plan := templates[0].Plan
	if plan.BusinessHours == nil || plan.Widget == nil || plan.AIAgent == nil || plan.Survey == nil || plan.Notifications == nil {
		t.Fatal("first template must be complete")
	}
	raw, _ := json.Marshal(plan)
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	delete(doc, "version")
	fillNulls(doc)
	raw, _ = json.Marshal(doc)
	if err := ai.ValidateJSON(schema, raw); err != nil {
		t.Fatal(err)
	}
}

// fillNulls turns JSON nulls of empty slices into [] (Go marshals nil
// slices as null; the model always sends arrays).
func fillNulls(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if val == nil {
				t[k] = []any{}
				continue
			}
			fillNulls(val)
		}
	case []any:
		for _, val := range t {
			fillNulls(val)
		}
	}
}

type fakeRunner struct {
	text string
	err  error
	in   aimodel.RunInput
}

func (f *fakeRunner) RunAgent(_ context.Context, _, _ int64, in aimodel.RunInput, _ aiusecase.AgentOptions) (aiusecase.RunResult, error) {
	f.in = in
	if f.err != nil {
		return aiusecase.RunResult{}, f.err
	}
	res := aiusecase.RunResult{}
	res.Text = f.text
	return res, nil
}

func TestAIPlanPassesSiteAsInput(t *testing.T) {
	runner := &fakeRunner{text: `{"teams":[{"key":"l1","name":"L1"}]}`}
	plan, _, err := aiPlan(context.Background(), runner, 1, 2, "en", Answers{Industry: "saas"}, &SiteSummary{URL: "https://acme.com/", Title: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Teams) != 1 || !strings.Contains(runner.in.Input, "Acme") || runner.in.Locale != "en" {
		t.Fatalf("plan %+v input %q", plan, runner.in.Input)
	}
	if len(runner.in.Input) > 24000 {
		t.Fatal("input over the AI core limit")
	}
}

func TestAINotice(t *testing.T) {
	for err, want := range map[error]string{
		aimodel.ErrCredits:  "ai_credits",
		aimodel.ErrDisabled: "ai_not_allowed",
		aimodel.ErrDaily:    "ai_limit",
		errAIUnavailable:    "ai_unavailable",
		ai.ErrUnavailable:   "ai_unavailable",
	} {
		if got := aiNotice(err); got != want {
			t.Errorf("aiNotice(%v) = %s, want %s", err, got, want)
		}
	}
}

func TestFallbackPicksClosestTemplate(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	plan, tpl := fallbackPlan(templates, Answers{Industry: "other", Notes: "online mağaza işletiyoruz, kargo soruları çok"},
		&SiteSummary{URL: "https://shop.example/", ThemeColor: "#123456"})
	if tpl.Key != "e-commerce" {
		t.Fatalf("picked %s", tpl.Key)
	}
	if plan.Branding.PrimaryColor != "#123456" || plan.Widget.Color != "#123456" || plan.Summary.Website != "https://shop.example/" {
		t.Fatalf("site not applied: %+v", plan.Branding)
	}
	if templates[1].Plan.Branding.PrimaryColor == "#123456" {
		t.Fatal("fallback must not modify the template")
	}
}
