package onboarding

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// SetupPlan (CF-140) describes a ready workspace: what the AI proposes from
// the company website and the questionnaire, and what every gallery
// template ships as JSON. The same schema feeds the preview, the apply job
// and undo. Version bumps when a field changes meaning.
type SetupPlan struct {
	Version       int                `json:"version"`
	Summary       PlanSummary        `json:"summary"`
	Branding      PlanBranding       `json:"branding"`
	Teams         []PlanTeam         `json:"teams"`
	Tags          []PlanTag          `json:"tags"`
	Statuses      []PlanStatus       `json:"statuses"`
	BusinessHours *PlanBusinessHours `json:"business_hours"`
	SLAPolicies   []PlanSLAPolicy    `json:"sla_policies"`
	CannedReplies []PlanCannedReply  `json:"canned_replies"`
	Macros        []PlanMacro        `json:"macros"`
	CustomFields  []PlanCustomField  `json:"custom_fields"`
	Forms         []PlanForm         `json:"forms"`
	Knowledge     []PlanCollection   `json:"knowledge"`
	Widget        *PlanWidget        `json:"widget"`
	AIAgent       *PlanAIAgent       `json:"ai_agent"`
	RoutingRules  []PlanRoutingRule  `json:"routing_rules"`
	Views         []PlanView         `json:"views"`
	Survey        *PlanSurvey        `json:"survey"`
	Appointments  []PlanAppointment  `json:"appointment_types"`
	Notifications *PlanNotifications `json:"notifications"`
}

// PlanVersion is the current SetupPlan schema version.
const PlanVersion = 1

// PlanSummary is what the plan knows about the business (shown on the
// preview; also the AI's "this is what I understood").
type PlanSummary struct {
	CompanyName string   `json:"company_name"`
	Industry    string   `json:"industry"`
	Description string   `json:"description"`
	Locale      string   `json:"locale"`
	Languages   []string `json:"languages"`
	Tone        string   `json:"tone"`
	Website     string   `json:"website"`
}

// PlanBranding colors the help center, e-mails and the widget.
type PlanBranding struct {
	PrimaryColor string `json:"primary_color"`
}

type PlanTeam struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Color       string   `json:"color"`
	Skills      []string `json:"skills"`
}

type PlanTag struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type PlanStatus struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Category string `json:"category"`
	Color    string `json:"color"`
}

// PlanDay is one weekday; Closed days ignore Open/Close ("HH:MM").
type PlanDay struct {
	Day    string `json:"day"`
	Closed bool   `json:"closed"`
	Open   string `json:"open"`
	Close  string `json:"close"`
}

type PlanBusinessHours struct {
	Name           string    `json:"name"`
	Timezone       string    `json:"timezone"`
	HolidayCountry string    `json:"holiday_country"`
	Days           []PlanDay `json:"days"`
}

// PlanSLATarget are minutes; 0 means no target.
type PlanSLATarget struct {
	Priority      string `json:"priority"`
	FirstResponse int    `json:"first_response_minutes"`
	NextResponse  int    `json:"next_response_minutes"`
	Resolution    int    `json:"resolution_minutes"`
}

type PlanSLAPolicy struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Priorities  []string        `json:"priorities"`
	Targets     []PlanSLATarget `json:"targets"`
}

type PlanCannedReply struct {
	Title    string `json:"title"`
	Shortcut string `json:"shortcut"`
	Category string `json:"category"`
	Content  string `json:"content"`
}

// PlanMacroAction mirrors the macros module: reply, note, status, priority, tag.
type PlanMacroAction struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type PlanMacro struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Actions     []PlanMacroAction `json:"actions"`
}

type PlanCustomField struct {
	Entity  string   `json:"entity"`
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	Options []string `json:"options"`
}

// PlanFormField is a system field (name, email, phone, subject, message,
// priority) or a custom field of this plan ("custom:<key>").
type PlanFormField struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type PlanForm struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Fields      []PlanFormField `json:"fields"`
}

type PlanArticle struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Public bool   `json:"public"`
}

type PlanCollection struct {
	Slug     string        `json:"slug"`
	Name     string        `json:"name"`
	Articles []PlanArticle `json:"articles"`
}

type PlanWidget struct {
	GreetingTitle string `json:"greeting_title"`
	GreetingText  string `json:"greeting_text"`
	Color         string `json:"color"`
}

type PlanProcedure struct {
	Intent string   `json:"intent"`
	Title  string   `json:"title"`
	Steps  []string `json:"steps"`
}

// PlanAIAgent: persona (ai_agent_configs), guardrails and tone (business
// context sections) and draft procedures. The agent's mode is never
// changed: the owner turns autonomy on themselves.
type PlanAIAgent struct {
	Persona         string          `json:"persona"`
	Tone            string          `json:"tone"`
	Guardrails      []string        `json:"guardrails"`
	HandoffKeywords []string        `json:"handoff_keywords"`
	Procedures      []PlanProcedure `json:"procedures"`
}

// PlanRoutingRule sends matching conversations to a team of this plan.
type PlanRoutingRule struct {
	Name         string   `json:"name"`
	Team         string   `json:"team"`
	Priorities   []string `json:"priorities"`
	ChannelTypes []string `json:"channel_types"`
}

// PlanView is a shared inbox view.
type PlanView struct {
	Name       string   `json:"name"`
	Icon       string   `json:"icon"`
	Statuses   []string `json:"statuses"`
	Priorities []string `json:"priorities"`
	Unassigned bool     `json:"unassigned"`
}

type PlanSurvey struct {
	Name     string `json:"name"`
	Question string `json:"question"`
}

// PlanAppointment is an appointment type (CF-138 appointments module).
type PlanAppointment struct {
	Name            string `json:"name"`
	DurationMinutes int    `json:"duration_minutes"`
	Description     string `json:"description"`
}

// PlanNotifications are the owner's notification defaults.
type PlanNotifications struct {
	EmailDigest string `json:"email_digest"`
	DigestHour  int    `json:"digest_hour"`
}

// Fixed vocabularies of the target modules.
var (
	planPriorities     = []string{"low", "normal", "high", "urgent"}
	planStatusCategory = []string{"open", "pending", "snoozed", "resolved", "closed"}
	planMacroTypes     = []string{"reply", "note", "status", "priority", "tag"}
	planMacroStatuses  = []string{"open", "pending", "resolved", "closed"}
	planFieldTypes     = []string{"text", "textarea", "number", "date", "boolean", "select", "multiselect", "url", "email", "phone"}
	planFieldEntities  = []string{"conversation", "contact"}
	planSystemFields   = []string{"name", "email", "phone", "subject", "message", "priority"}
	planChannelTypes   = []string{"widget", "email", "whatsapp", "instagram", "messenger", "telegram", "voice"}
	planDays           = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	planTones          = []string{"friendly", "formal", "neutral"}
	planDigests        = []string{"off", "hourly", "daily"}
	planLocales        = []string{"tr", "en"}
)

// Caps keep one plan (AI or template) to a reviewable size.
const (
	maxTeams       = 8
	maxTags        = 20
	maxStatuses    = 6
	maxPolicies    = 4
	maxCanned      = 20
	maxMacros      = 10
	maxFields      = 12
	maxForms       = 4
	maxCollections = 6
	maxArticles    = 6
	maxRules       = 10
	maxViews       = 8
	maxGuardrails  = 10
	maxProcedures  = 6
	maxAppointment = 8
)

var (
	colorRE    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	slugRE     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	shortcutRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	fieldKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	clockRE    = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	intentRE   = regexp.MustCompile(`^[a-z][a-z0-9_]{1,99}$`)
)

// ErrInvalidPlan: the plan cannot be applied (empty or broken).
var ErrInvalidPlan = errors.New("setup plan is invalid")

// Normalize trims, deduplicates, caps and repairs a plan in place so every
// item that survives can be written without a database CHECK failing.
// Broken items are dropped rather than failing the whole plan: an AI
// answer with one bad macro still yields a usable workspace.
func (p *SetupPlan) Normalize() error {
	p.Version = PlanVersion
	s := &p.Summary
	s.CompanyName = clipTrim(s.CompanyName, 120)
	s.Industry = clipTrim(s.Industry, 80)
	s.Description = clipTrim(s.Description, 600)
	s.Website = clipTrim(s.Website, 300)
	s.Locale = oneOf(s.Locale, planLocales, "tr")
	s.Tone = oneOf(s.Tone, planTones, "friendly")
	langs := []string{}
	for _, l := range s.Languages {
		if l = strings.ToLower(strings.TrimSpace(l)); len(l) == 2 && !slices.Contains(langs, l) {
			langs = append(langs, l)
		}
	}
	s.Languages = capSlice(langs, 6)
	p.Branding.PrimaryColor = color(p.Branding.PrimaryColor)

	p.Teams = dedupe(capSlice(p.Teams, maxTeams), func(t *PlanTeam) (string, bool) {
		t.Name = clipTrim(t.Name, 120)
		t.Description = clipTrim(t.Description, 500)
		t.Color = color(t.Color)
		t.Key = Slugify(firstNonEmpty(t.Key, t.Name), 40)
		t.Skills = cleanList(t.Skills, 10, 64)
		return t.Key, t.Name != "" && t.Key != ""
	})
	p.Tags = dedupe(capSlice(p.Tags, maxTags), func(t *PlanTag) (string, bool) {
		t.Name = clipTrim(t.Name, 64)
		t.Color = color(t.Color)
		return strings.ToLower(t.Name), t.Name != ""
	})
	p.Statuses = dedupe(capSlice(p.Statuses, maxStatuses), func(st *PlanStatus) (string, bool) {
		st.Name = clipTrim(st.Name, 60)
		st.Slug = Slugify(firstNonEmpty(st.Slug, st.Name), 40)
		st.Color = color(st.Color)
		return st.Slug, st.Name != "" && st.Slug != "" && slices.Contains(planStatusCategory, st.Category)
	})
	if bh := p.BusinessHours; bh != nil {
		bh.Name = firstNonEmpty(clipTrim(bh.Name, 120), "Mesai saatleri")
		if _, err := time.LoadLocation(bh.Timezone); bh.Timezone == "" || err != nil {
			bh.Timezone = "Europe/Istanbul"
		}
		bh.HolidayCountry = strings.ToUpper(strings.TrimSpace(bh.HolidayCountry))
		if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(bh.HolidayCountry) {
			bh.HolidayCountry = "TR"
		}
		bh.Days = normalizeDays(bh.Days)
	}
	p.SLAPolicies = dedupe(capSlice(p.SLAPolicies, maxPolicies), func(sp *PlanSLAPolicy) (string, bool) {
		sp.Name = clipTrim(sp.Name, 120)
		sp.Description = clipTrim(sp.Description, 500)
		sp.Priorities = filterIn(sp.Priorities, planPriorities)
		sp.Targets = dedupe(sp.Targets, func(t *PlanSLATarget) (string, bool) {
			t.FirstResponse, t.NextResponse, t.Resolution = minutes(t.FirstResponse), minutes(t.NextResponse), minutes(t.Resolution)
			return t.Priority, slices.Contains(planPriorities, t.Priority) && t.FirstResponse+t.NextResponse+t.Resolution > 0
		})
		return strings.ToLower(sp.Name), sp.Name != "" && len(sp.Targets) > 0
	})
	p.CannedReplies = dedupe(capSlice(p.CannedReplies, maxCanned), func(c *PlanCannedReply) (string, bool) {
		c.Title = clipTrim(c.Title, 120)
		c.Category = clipTrim(c.Category, 60)
		c.Content = clipTrim(c.Content, 4000)
		c.Shortcut = strings.ReplaceAll(Slugify(firstNonEmpty(c.Shortcut, c.Title), 40), "-", "_")
		return c.Shortcut, c.Title != "" && c.Content != "" && shortcutRE.MatchString(c.Shortcut)
	})
	tagNames := map[string]bool{}
	for _, t := range p.Tags {
		tagNames[strings.ToLower(t.Name)] = true
	}
	p.Macros = dedupe(capSlice(p.Macros, maxMacros), func(m *PlanMacro) (string, bool) {
		m.Name = clipTrim(m.Name, 120)
		m.Description = clipTrim(m.Description, 500)
		m.Actions = dedupe(capSlice(m.Actions, 10), func(a *PlanMacroAction) (string, bool) {
			a.Value = clipTrim(a.Value, 4000)
			ok := slices.Contains(planMacroTypes, a.Type) && a.Value != ""
			switch a.Type {
			case "status":
				ok = ok && slices.Contains(planMacroStatuses, a.Value)
			case "priority":
				ok = ok && slices.Contains(planPriorities, a.Value)
			case "tag":
				a.Value = clipTrim(a.Value, 64)
			}
			return a.Type + ":" + a.Value, ok
		})
		return strings.ToLower(m.Name), m.Name != "" && len(m.Actions) > 0
	})
	p.CustomFields = dedupe(capSlice(p.CustomFields, maxFields), func(f *PlanCustomField) (string, bool) {
		f.Entity = oneOf(f.Entity, planFieldEntities, "conversation")
		f.Key = strings.ReplaceAll(Slugify(firstNonEmpty(f.Key, f.Label), 40), "-", "_")
		f.Label = clipTrim(f.Label, 100)
		f.Options = cleanList(f.Options, 30, 100)
		choice := f.Type == "select" || f.Type == "multiselect"
		if !choice {
			f.Options = []string{}
		}
		ok := f.Label != "" && fieldKeyRE.MatchString(f.Key) && slices.Contains(planFieldTypes, f.Type) && (!choice || len(f.Options) > 0)
		return f.Entity + ":" + f.Key, ok
	})
	fieldKeys := map[string]string{}
	for _, f := range p.CustomFields {
		fieldKeys[f.Key] = f.Entity
	}
	p.Forms = dedupe(capSlice(p.Forms, maxForms), func(f *PlanForm) (string, bool) {
		f.Name = clipTrim(f.Name, 120)
		f.Description = clipTrim(f.Description, 1000)
		f.Fields = dedupe(capSlice(f.Fields, 20), func(ff *PlanFormField) (string, bool) {
			ff.Label = clipTrim(ff.Label, 120)
			if key, ok := strings.CutPrefix(ff.Field, "custom:"); ok {
				_, known := fieldKeys[key]
				return ff.Field, known
			}
			return ff.Field, slices.Contains(planSystemFields, ff.Field)
		})
		return strings.ToLower(f.Name), f.Name != "" && len(f.Fields) > 0
	})
	p.Knowledge = dedupe(capSlice(p.Knowledge, maxCollections), func(c *PlanCollection) (string, bool) {
		c.Name = clipTrim(c.Name, 200)
		c.Slug = Slugify(firstNonEmpty(c.Slug, c.Name), 60)
		c.Articles = dedupe(capSlice(c.Articles, maxArticles), func(a *PlanArticle) (string, bool) {
			a.Title = clipTrim(a.Title, 300)
			a.Body = clipTrim(a.Body, 20000)
			a.Slug = Slugify(firstNonEmpty(a.Slug, a.Title), 80)
			return a.Slug, a.Title != "" && a.Body != "" && slugRE.MatchString(a.Slug)
		})
		return c.Slug, c.Name != "" && slugRE.MatchString(c.Slug)
	})
	// Article slugs are unique per locale across collections.
	seenArticle := map[string]bool{}
	for i := range p.Knowledge {
		p.Knowledge[i].Articles = slices.DeleteFunc(p.Knowledge[i].Articles, func(a PlanArticle) bool {
			dup := seenArticle[a.Slug]
			seenArticle[a.Slug] = true
			return dup
		})
	}
	if w := p.Widget; w != nil {
		w.GreetingTitle = clipTrim(w.GreetingTitle, 80)
		w.GreetingText = clipTrim(w.GreetingText, 300)
		w.Color = color(firstNonEmpty(w.Color, p.Branding.PrimaryColor))
		if w.GreetingTitle == "" && w.GreetingText == "" && w.Color == "" {
			p.Widget = nil
		}
	}
	if a := p.AIAgent; a != nil {
		// The agent module counts bytes; Turkish letters take two.
		a.Persona = clipTrim(a.Persona, 1400)
		a.Tone = clipTrim(a.Tone, 2000)
		a.Guardrails = cleanList(a.Guardrails, maxGuardrails, 400)
		a.HandoffKeywords = cleanList(a.HandoffKeywords, 30, 45)
		a.Procedures = dedupe(capSlice(a.Procedures, maxProcedures), func(pr *PlanProcedure) (string, bool) {
			pr.Intent = strings.ReplaceAll(Slugify(firstNonEmpty(pr.Intent, pr.Title), 60), "-", "_")
			pr.Title = clipTrim(pr.Title, 90)
			pr.Steps = cleanList(pr.Steps, 20, 450)
			total := 0
			for i, st := range pr.Steps {
				if total += len(st); total > 3000 {
					pr.Steps = pr.Steps[:i]
					break
				}
			}
			return pr.Intent, intentRE.MatchString(pr.Intent) && pr.Title != "" && len(pr.Steps) > 0
		})
		if a.Persona == "" && a.Tone == "" && len(a.Guardrails) == 0 && len(a.Procedures) == 0 {
			p.AIAgent = nil
		}
	}
	teamKeys := map[string]bool{}
	for _, t := range p.Teams {
		teamKeys[t.Key] = true
	}
	p.RoutingRules = dedupe(capSlice(p.RoutingRules, maxRules), func(r *PlanRoutingRule) (string, bool) {
		r.Name = clipTrim(r.Name, 120)
		r.Team = Slugify(r.Team, 40)
		r.Priorities = filterIn(r.Priorities, planPriorities)
		r.ChannelTypes = filterIn(r.ChannelTypes, planChannelTypes)
		return strings.ToLower(r.Name), r.Name != "" && teamKeys[r.Team]
	})
	p.Views = dedupe(capSlice(p.Views, maxViews), func(v *PlanView) (string, bool) {
		v.Name = clipTrim(v.Name, 120)
		v.Icon = clipTrim(v.Icon, 40)
		v.Statuses = filterIn(v.Statuses, []string{"open", "pending", "snoozed", "resolved", "closed"})
		v.Priorities = filterIn(v.Priorities, planPriorities)
		return strings.ToLower(v.Name), v.Name != ""
	})
	if sv := p.Survey; sv != nil {
		sv.Name = clipTrim(sv.Name, 120)
		sv.Question = clipTrim(sv.Question, 1000)
		if sv.Name == "" || sv.Question == "" {
			p.Survey = nil
		}
	}
	p.Appointments = dedupe(capSlice(p.Appointments, maxAppointment), func(a *PlanAppointment) (string, bool) {
		a.Name = clipTrim(a.Name, 120)
		a.Description = clipTrim(a.Description, 500)
		if a.DurationMinutes < 5 || a.DurationMinutes > 480 {
			a.DurationMinutes = 30
		}
		return strings.ToLower(a.Name), a.Name != ""
	})
	if n := p.Notifications; n != nil {
		n.EmailDigest = oneOf(n.EmailDigest, planDigests, "daily")
		if n.DigestHour < 0 || n.DigestHour > 23 {
			n.DigestHour = 9
		}
	}
	if len(p.Items()) == 0 {
		return ErrInvalidPlan
	}
	return nil
}

func normalizeDays(in []PlanDay) []PlanDay {
	byDay := map[string]PlanDay{}
	for _, d := range in {
		d.Day = strings.ToLower(strings.TrimSpace(d.Day))
		if !slices.Contains(planDays, d.Day) {
			continue
		}
		if !d.Closed && (!clockRE.MatchString(d.Open) || !clockRE.MatchString(d.Close) || d.Open >= d.Close) {
			d.Closed = true
		}
		if d.Closed {
			d.Open, d.Close = "", ""
		}
		byDay[d.Day] = d
	}
	out := make([]PlanDay, 0, len(planDays))
	for _, day := range planDays {
		d, ok := byDay[day]
		if !ok {
			d = PlanDay{Day: day, Closed: true}
		}
		out = append(out, d)
	}
	return out
}

// Item kinds (preview rows, ledger rows and progress events).
const (
	KindBranding      = "branding"
	KindTeam          = "team"
	KindTag           = "tag"
	KindStatus        = "status"
	KindBusinessHours = "business_hours"
	KindSLAPolicy     = "sla_policy"
	KindCannedReply   = "canned_reply"
	KindMacro         = "macro"
	KindCustomField   = "custom_field"
	KindForm          = "form"
	KindCollection    = "kb_collection"
	KindArticle       = "kb_article"
	KindWidget        = "widget"
	KindAIPersona     = "ai_persona"
	KindAIGuardrail   = "ai_guardrails"
	KindAIProcedure   = "ai_procedure"
	KindRoutingRule   = "routing_rule"
	KindView          = "view"
	KindSurvey        = "survey"
	KindAppointment   = "appointment_type"
	KindNotifications = "notifications"
)

// Item is one previewable, applicable unit of a plan. Key is stable for
// the same plan content, so selection and the ledger refer to it.
type Item struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	// Parent is the key this item needs (an article needs its collection,
	// a routing rule its team); unticking the parent skips the child.
	Parent string `json:"parent,omitempty"`
}

// Items lists the plan in apply order: settings and building blocks
// first, then what references them.
func (p *SetupPlan) Items() []Item {
	var out []Item
	add := func(kind, id, label, detail, parent string) {
		out = append(out, Item{Key: kind + ":" + id, Kind: kind, Label: label, Detail: detail, Parent: parent})
	}
	if p.Branding.PrimaryColor != "" {
		add(KindBranding, "primary", p.Branding.PrimaryColor, "", "")
	}
	if bh := p.BusinessHours; bh != nil {
		add(KindBusinessHours, "default", bh.Name, daysSummary(bh.Days), "")
	}
	for _, t := range p.Teams {
		add(KindTeam, t.Key, t.Name, t.Description, "")
	}
	for _, t := range p.Tags {
		add(KindTag, Slugify(t.Name, 64), t.Name, "", "")
	}
	for _, s := range p.Statuses {
		add(KindStatus, s.Slug, s.Name, s.Category, "")
	}
	for _, s := range p.SLAPolicies {
		add(KindSLAPolicy, Slugify(s.Name, 60), s.Name, slaSummary(s), "")
	}
	for _, f := range p.CustomFields {
		add(KindCustomField, f.Entity+"-"+f.Key, f.Label, f.Entity+" · "+f.Type, "")
	}
	for _, c := range p.CannedReplies {
		add(KindCannedReply, c.Shortcut, c.Title, "/"+c.Shortcut, "")
	}
	for _, m := range p.Macros {
		add(KindMacro, Slugify(m.Name, 60), m.Name, m.Description, "")
	}
	for _, f := range p.Forms {
		add(KindForm, Slugify(f.Name, 60), f.Name, fmt.Sprintf("%d", len(f.Fields)), "")
	}
	for _, c := range p.Knowledge {
		add(KindCollection, c.Slug, c.Name, fmt.Sprintf("%d", len(c.Articles)), "")
		for _, a := range c.Articles {
			add(KindArticle, a.Slug, a.Title, c.Name, KindCollection+":"+c.Slug)
		}
	}
	if w := p.Widget; w != nil {
		add(KindWidget, "greeting", firstNonEmpty(w.GreetingTitle, w.Color), w.GreetingText, "")
	}
	if a := p.AIAgent; a != nil {
		if a.Persona != "" {
			add(KindAIPersona, "persona", clip(a.Persona, 80), "", "")
		}
		if a.Tone != "" || len(a.Guardrails) > 0 {
			add(KindAIGuardrail, "guardrails", fmt.Sprintf("%d", len(a.Guardrails)), clip(a.Tone, 120), "")
		}
		for _, pr := range a.Procedures {
			add(KindAIProcedure, pr.Intent, pr.Title, fmt.Sprintf("%d", len(pr.Steps)), "")
		}
	}
	for _, r := range p.RoutingRules {
		add(KindRoutingRule, Slugify(r.Name, 60), r.Name, r.Team, KindTeam+":"+r.Team)
	}
	for _, v := range p.Views {
		add(KindView, Slugify(v.Name, 60), v.Name, "", "")
	}
	if sv := p.Survey; sv != nil {
		add(KindSurvey, "csat", sv.Name, sv.Question, "")
	}
	for _, a := range p.Appointments {
		add(KindAppointment, Slugify(a.Name, 60), a.Name, fmt.Sprintf("%d", a.DurationMinutes), "")
	}
	if n := p.Notifications; n != nil {
		add(KindNotifications, "owner", n.EmailDigest, fmt.Sprintf("%02d:00", n.DigestHour), "")
	}
	return out
}

func daysSummary(days []PlanDay) string {
	parts := make([]string, 0, len(days))
	for _, d := range days {
		if !d.Closed {
			parts = append(parts, d.Day+" "+d.Open+"-"+d.Close)
		}
	}
	return strings.Join(parts, ", ")
}

func slaSummary(s PlanSLAPolicy) string {
	for _, t := range s.Targets {
		if t.FirstResponse > 0 {
			return fmt.Sprintf("%s: %d", t.Priority, t.FirstResponse)
		}
	}
	return ""
}

// Slugify makes an ASCII slug (Turkish letters folded: ç→c, ğ→g, ı→i …).
func Slugify(s string, maxLen int) string {
	s = strings.NewReplacer("ı", "i", "İ", "i", "ş", "s", "Ş", "s", "ğ", "g", "Ğ", "g").Replace(s)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(t, s)
	if err == nil {
		s = folded
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= maxLen {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func clipTrim(s string, n int) string { return strings.TrimSpace(clip(strings.TrimSpace(s), n)) }

func color(c string) string {
	c = strings.TrimSpace(c)
	if colorRE.MatchString(c) {
		return strings.ToLower(c)
	}
	return ""
}

func minutes(v int) int {
	if v < 0 {
		return 0
	}
	return min(v, 525600)
}

func oneOf(v string, allowed []string, fallback string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if slices.Contains(allowed, v) {
		return v
	}
	return fallback
}

func filterIn(in, allowed []string) []string {
	out := []string{}
	for _, v := range in {
		if v = strings.ToLower(strings.TrimSpace(v)); slices.Contains(allowed, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func cleanList(in []string, maxItems, maxLen int) []string {
	out := []string{}
	for _, v := range in {
		if v = clipTrim(v, maxLen); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return capSlice(out, maxItems)
}

func capSlice[T any](in []T, n int) []T {
	if len(in) > n {
		return in[:n]
	}
	return in
}

// dedupe normalizes every element with fix (which returns its identity and
// whether it is valid) and keeps the first valid element per identity.
func dedupe[T any](in []T, fix func(*T) (string, bool)) []T {
	out := make([]T, 0, len(in))
	seen := map[string]bool{}
	for i := range in {
		v := in[i]
		id, ok := fix(&v)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, v)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
