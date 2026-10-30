package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
)

// AIRunner is the AI core (credit reservation, policy, entitlement, PII
// masking and output validation all happen inside RunAgent).
type AIRunner interface {
	RunAgent(ctx context.Context, org, actor int64, in aimodel.RunInput, options aiusecase.AgentOptions) (aiusecase.RunResult, error)
}

// Answers is the chat-like questionnaire of the discovery step.
type Answers struct {
	CompanyName  string   `json:"company_name"`
	Industry     string   `json:"industry"`
	TeamSize     string   `json:"team_size"`
	Channels     []string `json:"channels"`
	SupportHours string   `json:"support_hours"`
	Languages    []string `json:"languages"`
	RequestTypes []string `json:"request_types"`
	Tone         string   `json:"tone"`
	Notes        string   `json:"notes"`
}

// Questionnaire vocabularies (the UI offers these as chips).
var (
	answerIndustries = []string{"saas", "ecommerce", "agency", "it_helpdesk", "fintech", "education", "healthcare", "field_service", "other"}
	answerTeamSizes  = []string{"1", "2-5", "6-20", "21-50", "50+"}
	answerHours      = []string{"weekdays_9_18", "weekdays_8_20", "extended", "24_7"}
)

func (a *Answers) normalize() {
	a.CompanyName = clipTrim(a.CompanyName, 120)
	a.Industry = oneOf(a.Industry, answerIndustries, "other")
	a.TeamSize = oneOf(a.TeamSize, answerTeamSizes, "2-5")
	a.SupportHours = oneOf(a.SupportHours, answerHours, "weekdays_9_18")
	a.Tone = oneOf(a.Tone, planTones, "friendly")
	a.Channels = filterIn(a.Channels, planChannelTypes)
	a.Languages = cleanList(a.Languages, 6, 2)
	a.RequestTypes = cleanList(a.RequestTypes, 12, 60)
	a.Notes = clipTrim(a.Notes, 1000)
}

// hoursPreset turns the questionnaire answer into weekdays.
func hoursPreset(preset string) []PlanDay {
	days := make([]PlanDay, 0, len(planDays))
	for i, d := range planDays {
		day := PlanDay{Day: d, Open: "09:00", Close: "18:00"}
		switch preset {
		case "weekdays_8_20":
			day.Open, day.Close = "08:00", "20:00"
		case "extended":
			day.Open, day.Close = "08:00", "22:00"
			if i == 6 {
				day.Open, day.Close = "10:00", "18:00"
			}
		case "24_7":
			day.Open, day.Close = "00:00", "23:59"
		}
		day.Closed = preset != "24_7" && preset != "extended" && i >= 5
		days = append(days, day)
	}
	return days
}

const planInstructions = `You set up a customer-support workspace (ClickFix, a B2B help desk) for a new company.
From the company's public website summary and the owner's questionnaire answers, produce a SetupPlan that makes the
workspace ready on first login. Write every name, reply, article and rule in the language of "locale".
Be concrete and specific to this business (its products, customers and typical requests); never invent prices,
phone numbers, addresses or policies that are not in the input — write placeholders like "[iade süresi]" instead.
Keep it reviewable: 2-4 teams, 5-8 tags, 0-2 custom statuses (categories open|pending|snoozed|resolved|closed),
1-2 SLA policies (minutes; priorities low|normal|high|urgent), 4-6 canned replies (shortcut: lowercase a-z0-9_),
2-3 macros (action types reply|note|status|priority|tag; status values open|pending|resolved|closed), 1-3 custom fields
(entity conversation|contact; type text|textarea|number|date|boolean|select|multiselect|url|email|phone; options only for
select types), 1 form (fields: name|email|phone|subject|message|priority or "custom:<field key>"), 1-2 knowledge
collections with 2-3 short markdown articles each (public=true only for customer-facing answers), a widget greeting,
an AI agent persona with 2-5 guardrails and 1-3 procedures (intent: snake_case), 1-3 routing rules (team = a team key),
2-3 inbox views, a CSAT survey question, notification digest, appointment types only when the business takes
appointments or on-site sessions. Colors are #RRGGBB; prefer the website's brand color for branding and widget.`

// planSchema is the structured-output schema (strict: every key present).
// Array sizes and string lengths are not constrained here (one provider
// drops those keywords); SetupPlan.Normalize caps them instead.
func planSchema() map[string]any {
	str := map[string]any{"type": "string"}
	boolean := map[string]any{"type": "boolean"}
	integer := map[string]any{"type": "integer"}
	strs := map[string]any{"type": "array", "items": str}
	arr := func(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }
	enum := func(v ...string) map[string]any { return map[string]any{"type": "string", "enum": v} }
	prio := enum(planPriorities...)
	return obj(map[string]any{
		"summary": obj(map[string]any{
			"company_name": str, "industry": str, "description": str, "locale": enum(planLocales...),
			"languages": strs, "tone": enum(planTones...), "website": str,
		}),
		"branding": obj(map[string]any{"primary_color": str}),
		"teams":    arr(obj(map[string]any{"key": str, "name": str, "description": str, "color": str, "skills": strs})),
		"tags":     arr(obj(map[string]any{"name": str, "color": str})),
		"statuses": arr(obj(map[string]any{"name": str, "slug": str, "category": enum(planStatusCategory...), "color": str})),
		"business_hours": obj(map[string]any{
			"name": str, "timezone": str, "holiday_country": str,
			"days": arr(obj(map[string]any{"day": enum(planDays...), "closed": boolean, "open": str, "close": str})),
		}),
		"sla_policies": arr(obj(map[string]any{
			"name": str, "description": str, "priorities": arr(prio),
			"targets": arr(obj(map[string]any{"priority": prio, "first_response_minutes": integer, "next_response_minutes": integer, "resolution_minutes": integer})),
		})),
		"canned_replies": arr(obj(map[string]any{"title": str, "shortcut": str, "category": str, "content": str})),
		"macros": arr(obj(map[string]any{"name": str, "description": str,
			"actions": arr(obj(map[string]any{"type": enum(planMacroTypes...), "value": str}))})),
		"custom_fields": arr(obj(map[string]any{"entity": enum(planFieldEntities...), "key": str, "label": str, "type": enum(planFieldTypes...), "options": strs})),
		"forms": arr(obj(map[string]any{"name": str, "description": str,
			"fields": arr(obj(map[string]any{"field": str, "label": str, "required": boolean}))})),
		"knowledge": arr(obj(map[string]any{"slug": str, "name": str,
			"articles": arr(obj(map[string]any{"slug": str, "title": str, "body": str, "public": boolean}))})),
		"widget": obj(map[string]any{"greeting_title": str, "greeting_text": str, "color": str}),
		"ai_agent": obj(map[string]any{"persona": str, "tone": str, "guardrails": strs, "handoff_keywords": strs,
			"procedures": arr(obj(map[string]any{"intent": str, "title": str, "steps": strs}))}),
		"routing_rules": arr(obj(map[string]any{"name": str, "team": str, "priorities": arr(prio), "channel_types": arr(enum(planChannelTypes...))})),
		"views": arr(obj(map[string]any{"name": str, "icon": str, "statuses": arr(enum("open", "pending", "snoozed", "resolved", "closed")),
			"priorities": arr(prio), "unassigned": boolean})),
		"survey":            obj(map[string]any{"name": str, "question": str}),
		"appointment_types": arr(obj(map[string]any{"name": str, "duration_minutes": integer, "description": str})),
		"notifications":     obj(map[string]any{"email_digest": enum(planDigests...), "digest_hour": integer}),
	})
}

func obj(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	slices.Sort(required)
	return map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
}

// aiPlan asks the AI core for a plan. The website text and the answers are
// customer data: RunAgent wraps them as untrusted input and masks PII.
func aiPlan(ctx context.Context, runner AIRunner, org, actor int64, locale string, ans Answers, site *SiteSummary) (SetupPlan, uuid.UUID, error) {
	if runner == nil {
		return SetupPlan{}, uuid.Nil, errAIUnavailable
	}
	input := map[string]any{"locale": locale, "answers": ans}
	if site != nil {
		input["website"] = map[string]any{
			"url": site.URL, "title": site.Title, "description": site.Description, "site_name": site.SiteName,
			"theme_color": site.ThemeColor, "language": site.Language, "headings": site.Headings, "text": site.Text,
		}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return SetupPlan{}, uuid.Nil, err
	}
	res, err := runner.RunAgent(ctx, org, actor, aimodel.RunInput{
		Task: "reply", Input: clip(string(raw), 11000), Locale: locale, MaxTokens: 8192,
	}, aiusecase.AgentOptions{Instructions: planInstructions, Schema: planSchema()})
	if err != nil {
		return SetupPlan{}, uuid.Nil, err
	}
	var plan SetupPlan
	if err := json.Unmarshal([]byte(res.Text), &plan); err != nil {
		return SetupPlan{}, res.UUID, fmt.Errorf("onboarding: ai plan: %w", err)
	}
	return plan, res.UUID, nil
}

var errAIUnavailable = errors.New("ai is not available")

// fallbackPlan builds a plan without AI: the closest gallery template,
// personalised with the questionnaire answers and the website's brand.
func fallbackPlan(templates []Template, ans Answers, site *SiteSummary) (SetupPlan, Template) {
	text := ans.Notes + " " + strings.Join(ans.RequestTypes, " ")
	if site != nil {
		text += " " + site.Title + " " + site.Description + " " + strings.Join(site.Headings, " ")
	}
	tpl := closestTemplate(templates, ans.Industry, text)
	plan := clonePlan(tpl.Plan)
	// The template's content language wins (see FromTemplate).
	personalize(&plan, "", ans, site)
	return plan, tpl
}

// personalize applies what the owner told us on top of any plan.
func personalize(p *SetupPlan, locale string, ans Answers, site *SiteSummary) {
	if locale != "" {
		p.Summary.Locale = locale
	}
	if ans.CompanyName != "" {
		p.Summary.CompanyName = ans.CompanyName
	}
	if ans.Industry != "" && ans.Industry != "other" {
		p.Summary.Industry = ans.Industry
	}
	if len(ans.Languages) > 0 {
		p.Summary.Languages = ans.Languages
	}
	if ans.Tone != "" {
		p.Summary.Tone = ans.Tone
	}
	if ans.SupportHours != "" && p.BusinessHours != nil {
		p.BusinessHours.Days = hoursPreset(ans.SupportHours)
	}
	if site != nil {
		p.Summary.Website = site.URL
		if p.Summary.Description == "" {
			p.Summary.Description = site.Description
		}
		if site.ThemeColor != "" {
			p.Branding.PrimaryColor = site.ThemeColor
			if p.Widget != nil {
				p.Widget.Color = site.ThemeColor
			}
		}
		if p.Summary.CompanyName == "" {
			p.Summary.CompanyName = firstNonEmpty(site.SiteName, site.Title)
		}
	}
	// Routing rules for channels the business does not use are noise.
	if len(ans.Channels) > 0 {
		for i := range p.RoutingRules {
			r := &p.RoutingRules[i]
			if len(r.ChannelTypes) == 0 {
				continue
			}
			kept := filterIn(r.ChannelTypes, ans.Channels)
			if len(kept) == 0 {
				r.Name = ""
			}
			r.ChannelTypes = kept
		}
	}
}

func clonePlan(p SetupPlan) SetupPlan {
	var out SetupPlan
	raw, _ := json.Marshal(p)
	_ = json.Unmarshal(raw, &out)
	return out
}
