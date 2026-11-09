package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	knowledgemodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
)

// Item statuses in the preview (new / exists / unavailable) and in the
// ledger (created / updated / skipped / unavailable / failed).
const (
	PreviewNew         = "new"
	PreviewExists      = "exists"
	PreviewUnavailable = "unavailable"

	ItemCreated     = "created"
	ItemUpdated     = "updated"
	ItemSkipped     = "skipped"
	ItemUnavailable = "unavailable"
	ItemFailed      = "failed"
)

// Knowledge is the knowledge usecase subset the applier needs.
type Knowledge interface {
	SaveCollection(ctx context.Context, org int64, id uuid.UUID, in knowledgemodel.CollectionInput) (db.KnowledgeCollection, error)
	SaveArticle(ctx context.Context, org, actor int64, id uuid.UUID, in knowledgemodel.ArticleInput, create bool) (db.KnowledgeArticle, error)
	Publish(ctx context.Context, org int64, id uuid.UUID, version int32, publish bool) error
	DeleteArticle(ctx context.Context, org int64, id uuid.UUID) error
	DeleteCollection(ctx context.Context, org int64, id uuid.UUID) error
}

// Profiles reads and patches the organization profile (branding).
type Profiles interface {
	GetProfile(ctx context.Context, orgID int64) (orgmodel.OrganizationProfile, error)
	PatchProfile(ctx context.Context, orgID int64, in orgmodel.PatchProfileInput) (orgmodel.OrganizationProfile, []string, error)
}

// AppointmentTypes is the appointments module (CF-138, AppointmentsAdapter).
// While it is nil or Enabled answers false (not in the organization's plan
// or switched off), appointment items are "unavailable".
type AppointmentTypes interface {
	Enabled(ctx context.Context, org int64) bool
	Find(ctx context.Context, org int64, name string) (uuid.UUID, bool, error)
	Create(ctx context.Context, org, actor int64, in PlanAppointment) (uuid.UUID, error)
	Delete(ctx context.Context, org int64, id uuid.UUID) error
}

// Applier writes plan items through the owning modules' sqlc queries and
// usecases (the demo seed's approach), one item at a time.
type Applier struct {
	Pool         *pgxpool.Pool
	Q            *db.Queries
	KB           Knowledge
	Profiles     Profiles
	Appointments AppointmentTypes
}

// PreviewItem is one row of the preview diff.
type PreviewItem struct {
	Item
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// applyState carries what earlier items created, for references.
type applyState struct {
	org, actor int64
	plan       *SetupPlan
	teams      map[string]int64     // plan team key → id
	fields     map[string]uuid.UUID // custom field key → uuid
	fieldTypes map[string]string    // custom field key → entity
	cols       map[string]uuid.UUID // collection slug → uuid
	hoursID    int64
}

type result struct {
	status   string
	id       int64
	uuid     uuid.UUID
	previous []byte
	reason   string
}

func found(err error) (bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// exists answers the preview: does an entity with this item's natural key
// already exist (then apply skips it), or can it not be applied at all?
func (a *Applier) exists(ctx context.Context, org int64, p *SetupPlan, it Item) (string, string, error) {
	q := a.Q
	id := strings.TrimPrefix(it.Key, it.Kind+":")
	var err error
	switch it.Kind {
	case KindTeam:
		_, err = q.OnboardingFindTeam(ctx, db.OnboardingFindTeamParams{OrganizationID: org, Name: it.Label})
	case KindTag:
		_, err = q.OnboardingFindTag(ctx, db.OnboardingFindTagParams{OrganizationID: org, Name: it.Label})
	case KindStatus:
		_, err = q.OnboardingFindStatus(ctx, db.OnboardingFindStatusParams{OrganizationID: org, Slug: id})
	case KindBusinessHours:
		_, err = q.OnboardingFindBusinessHours(ctx, db.OnboardingFindBusinessHoursParams{OrganizationID: org, Name: it.Label})
	case KindSLAPolicy:
		_, err = q.OnboardingFindSLAPolicy(ctx, db.OnboardingFindSLAPolicyParams{OrganizationID: org, Name: it.Label})
	case KindCannedReply:
		_, err = q.OnboardingFindCannedResponse(ctx, db.OnboardingFindCannedResponseParams{OrganizationID: org, Shortcut: id})
	case KindMacro:
		_, err = q.OnboardingFindMacro(ctx, db.OnboardingFindMacroParams{OrganizationID: org, Name: it.Label})
	case KindCustomField:
		entity, key, _ := strings.Cut(id, "-")
		_, err = q.OnboardingFindCustomField(ctx, db.OnboardingFindCustomFieldParams{OrganizationID: org, EntityType: entity, Key: key})
	case KindForm:
		_, err = q.OnboardingFindRequestForm(ctx, db.OnboardingFindRequestFormParams{OrganizationID: org, Name: it.Label})
	case KindCollection:
		_, err = q.OnboardingFindCollection(ctx, db.OnboardingFindCollectionParams{OrganizationID: org, Slug: id})
	case KindArticle:
		_, err = q.OnboardingFindArticle(ctx, db.OnboardingFindArticleParams{OrganizationID: org, Locale: p.Summary.Locale, Slug: id})
	case KindRoutingRule:
		_, err = q.OnboardingFindRoutingRule(ctx, db.OnboardingFindRoutingRuleParams{OrganizationID: org, Name: it.Label})
	case KindView:
		_, err = q.OnboardingFindView(ctx, db.OnboardingFindViewParams{OrganizationID: org, Name: it.Label})
	case KindSurvey:
		_, err = q.OnboardingFindSurvey(ctx, db.OnboardingFindSurveyParams{OrganizationID: org, Kind: "csat"})
	case KindAIProcedure:
		_, err = q.OnboardingFindProcedure(ctx, db.OnboardingFindProcedureParams{OrganizationID: org, Intent: id, Locale: p.Summary.Locale})
	case KindWidget:
		_, err = q.OnboardingFirstWidget(ctx, org)
		if errors.Is(err, pgx.ErrNoRows) {
			// No widget yet: the final "connect your first channel" step
			// creates it with these settings.
			return PreviewUnavailable, "no_widget", nil
		}
		return PreviewNew, "", err
	case KindAppointment:
		if a.Appointments == nil || !a.Appointments.Enabled(ctx, org) {
			return PreviewUnavailable, "module_disabled", nil
		}
		_, ok, ferr := a.Appointments.Find(ctx, org, it.Label)
		if ok {
			return PreviewExists, "", ferr
		}
		return PreviewNew, "", ferr
	case KindNotifications:
		return PreviewNew, "", nil
	default:
		// Settings (branding, AI persona and guardrails) are overwritten and
		// restored on undo.
		return PreviewNew, "", nil
	}
	ok, err := found(err)
	if err != nil {
		return "", "", err
	}
	if ok {
		return PreviewExists, "", nil
	}
	return PreviewNew, "", nil
}

// Preview diffs the plan against the organization.
func (a *Applier) Preview(ctx context.Context, org int64, p *SetupPlan) ([]PreviewItem, error) {
	items := p.Items()
	out := make([]PreviewItem, 0, len(items))
	for _, it := range items {
		status, reason, err := a.exists(ctx, org, p, it)
		if err != nil {
			return nil, fmt.Errorf("onboarding: preview %s: %w", it.Key, err)
		}
		out = append(out, PreviewItem{Item: it, Status: status, Reason: reason})
	}
	return out, nil
}

// apply writes one item. An entity that already exists is skipped (and
// remembered for references) rather than changed.
func (a *Applier) apply(ctx context.Context, st *applyState, it Item) (result, error) {
	status, reason, err := a.exists(ctx, st.org, st.plan, it)
	if err != nil {
		return result{}, err
	}
	switch status {
	case PreviewUnavailable:
		return result{status: ItemUnavailable, reason: reason}, nil
	case PreviewExists:
		a.remember(ctx, st, it)
		return result{status: ItemSkipped, reason: "exists"}, nil
	}
	p := st.plan
	id := strings.TrimPrefix(it.Key, it.Kind+":")
	q, org := a.Q, st.org
	switch it.Kind {
	case KindBranding:
		return a.applyBranding(ctx, st)
	case KindBusinessHours:
		bh := p.BusinessHours
		_, derr := q.GetDefaultBusinessHours(ctx, org)
		hasDefault, derr := found(derr)
		if derr != nil {
			return result{}, derr
		}
		row, err := q.InsertBusinessHours(ctx, db.InsertBusinessHoursParams{
			OrganizationID: org, Name: bh.Name, Timezone: bh.Timezone, WorkingHours: mustJSON(workingHours(bh.Days)),
			UsePublicHolidays: true, HolidayCountry: bh.HolidayCountry, IsDefault: !hasDefault,
		})
		if err != nil {
			return result{}, err
		}
		st.hoursID = row.ID
		return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
	case KindTeam:
		t := findTeam(p, id)
		row, err := q.CreateTeam(ctx, db.CreateTeamParams{
			OrganizationID: org, Name: t.Name, Description: t.Description, Color: t.Color,
			Timezone: textOrNull(timezoneOf(p)), WorkingHours: workingHoursOrNull(p), Skills: nonNil(t.Skills),
		})
		if err != nil {
			return result{}, err
		}
		st.teams[t.Key] = row.ID
		return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
	case KindTag:
		for _, t := range p.Tags {
			if t.Name == it.Label {
				row, err := q.UpsertTag(ctx, db.UpsertTagParams{OrganizationID: org, Name: t.Name, Color: t.Color})
				if err != nil {
					return result{}, err
				}
				return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
			}
		}
	case KindStatus:
		for _, s := range p.Statuses {
			if s.Slug != id {
				continue
			}
			order, err := q.OnboardingNextStatusOrder(ctx, org)
			if err != nil {
				return result{}, err
			}
			row, err := q.InsertConversationStatus(ctx, db.InsertConversationStatusParams{
				OrganizationID: org, Name: s.Name, Slug: s.Slug, Category: s.Category, Color: s.Color, SortOrder: order,
			})
			if err != nil {
				return result{}, err
			}
			return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
		}
	case KindSLAPolicy:
		for _, sp := range p.SLAPolicies {
			if !strings.EqualFold(sp.Name, it.Label) {
				continue
			}
			return a.applySLA(ctx, st, sp)
		}
	case KindCustomField:
		for _, f := range p.CustomFields {
			if f.Entity+"-"+f.Key == id {
				return a.applyCustomField(ctx, st, f)
			}
		}
	case KindCannedReply:
		for _, c := range p.CannedReplies {
			if c.Shortcut != id {
				continue
			}
			row, err := q.InsertCannedResponse(ctx, db.InsertCannedResponseParams{
				OrganizationID: org, Scope: "organization", Title: c.Title, Shortcut: c.Shortcut,
				Category: c.Category, Content: c.Content, CreatedBy: i8(st.actor),
			})
			if err != nil {
				return result{}, err
			}
			return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
		}
	case KindMacro:
		for _, m := range p.Macros {
			if !strings.EqualFold(m.Name, it.Label) {
				continue
			}
			row, err := q.InsertMacro(ctx, db.InsertMacroParams{
				OrganizationID: org, Scope: "organization", Name: m.Name, Description: m.Description,
				Actions: mustJSON(m.Actions), CreatedBy: i8(st.actor),
			})
			if err != nil {
				return result{}, err
			}
			return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
		}
	case KindForm:
		for _, f := range p.Forms {
			if strings.EqualFold(f.Name, it.Label) {
				return a.applyForm(ctx, st, f)
			}
		}
	case KindCollection:
		for _, c := range p.Knowledge {
			if c.Slug != id {
				continue
			}
			row, err := a.KB.SaveCollection(ctx, org, uuid.New(), knowledgemodel.CollectionInput{Name: c.Name, Slug: c.Slug, Kind: "collection"})
			if err != nil {
				return result{}, err
			}
			st.cols[c.Slug] = row.Uuid
			return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
		}
	case KindArticle:
		return a.applyArticle(ctx, st, it)
	case KindWidget:
		return a.applyWidget(ctx, st)
	case KindAIPersona:
		return a.applyPersona(ctx, st)
	case KindAIGuardrail:
		return a.applyGuardrails(ctx, st)
	case KindAIProcedure:
		for _, pr := range p.AIAgent.Procedures {
			if pr.Intent != id {
				continue
			}
			draft := map[string]any{"intent": pr.Intent, "locale": p.Summary.Locale, "title": pr.Title, "steps": pr.Steps, "allowed_tools": []string{}, "revision": 1}
			row, err := q.AIProcedureCreate(ctx, db.AIProcedureCreateParams{
				OrganizationID: org, Intent: pr.Intent, Locale: p.Summary.Locale, Title: pr.Title, Draft: mustJSON(draft),
			})
			if err != nil {
				return result{}, err
			}
			return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
		}
	case KindRoutingRule:
		for _, r := range p.RoutingRules {
			if strings.EqualFold(r.Name, it.Label) {
				return a.applyRoutingRule(ctx, st, r)
			}
		}
	case KindView:
		for _, v := range p.Views {
			if !strings.EqualFold(v.Name, it.Label) {
				continue
			}
			filters := map[string]any{}
			if len(v.Statuses) > 0 {
				filters["statuses"] = v.Statuses
			}
			if len(v.Priorities) > 0 {
				filters["priorities"] = v.Priorities
			}
			if v.Unassigned {
				filters["assignee"] = "unassigned"
			}
			pos, err := q.OnboardingNextViewPosition(ctx, org)
			if err != nil {
				return result{}, err
			}
			row, err := q.InsertConversationView(ctx, db.InsertConversationViewParams{
				OrganizationID: org, Scope: "organization", Name: v.Name, Icon: firstNonEmpty(v.Icon, "inbox"),
				Filters: mustJSON(filters), Position: pos, CreatedBy: i8(st.actor),
			})
			if err != nil {
				return result{}, err
			}
			return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
		}
	case KindSurvey:
		row, err := q.SurveyCreateTemplate(ctx, db.SurveyCreateTemplateParams{
			OrganizationID: org, Name: p.Survey.Name, Kind: "csat", Question: p.Survey.Question, OnResolve: true, Enabled: true,
		})
		if err != nil {
			return result{}, err
		}
		return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
	case KindAppointment:
		for _, ap := range p.Appointments {
			if strings.EqualFold(ap.Name, it.Label) {
				u, err := a.Appointments.Create(ctx, org, st.actor, ap)
				if err != nil {
					return result{}, err
				}
				return result{status: ItemCreated, uuid: u}, nil
			}
		}
	case KindNotifications:
		return a.applyNotifications(ctx, st)
	}
	return result{}, fmt.Errorf("onboarding: item %s not in plan", it.Key)
}

// remember resolves an existing entity for later references (a routing
// rule pointing at a team that was already there).
func (a *Applier) remember(ctx context.Context, st *applyState, it Item) {
	id := strings.TrimPrefix(it.Key, it.Kind+":")
	switch it.Kind {
	case KindTeam:
		if row, err := a.Q.OnboardingFindTeam(ctx, db.OnboardingFindTeamParams{OrganizationID: st.org, Name: it.Label}); err == nil {
			st.teams[id] = row.ID
		}
	case KindBusinessHours:
		if row, err := a.Q.OnboardingFindBusinessHours(ctx, db.OnboardingFindBusinessHoursParams{OrganizationID: st.org, Name: it.Label}); err == nil {
			st.hoursID = row.ID
		}
	case KindCustomField:
		entity, key, _ := strings.Cut(id, "-")
		if row, err := a.Q.OnboardingFindCustomField(ctx, db.OnboardingFindCustomFieldParams{OrganizationID: st.org, EntityType: entity, Key: key}); err == nil && !row.ArchivedAt.Valid {
			st.fields[key], st.fieldTypes[key] = row.Uuid, entity
		}
	case KindCollection:
		if row, err := a.Q.OnboardingFindCollection(ctx, db.OnboardingFindCollectionParams{OrganizationID: st.org, Slug: id}); err == nil {
			st.cols[id] = row.Uuid
		}
	}
}

func (a *Applier) applyBranding(ctx context.Context, st *applyState) (result, error) {
	prof, err := a.Profiles.GetProfile(ctx, st.org)
	if err != nil {
		return result{}, err
	}
	prev := mustJSON(map[string]any{"branding": prof.Branding, "website": prof.Company.Website})
	in := orgmodel.PatchProfileInput{Branding: &orgmodel.Branding{PrimaryColor: st.plan.Branding.PrimaryColor}}
	if prof.Company.Website == "" && st.plan.Summary.Website != "" {
		c := prof.Company
		c.Website = st.plan.Summary.Website
		in.Company = &c
	}
	if _, _, err := a.Profiles.PatchProfile(ctx, st.org, in); err != nil {
		return result{}, err
	}
	return result{status: ItemUpdated, previous: prev}, nil
}

func (a *Applier) applySLA(ctx context.Context, st *applyState, sp PlanSLAPolicy) (result, error) {
	var out result
	err := pgx.BeginFunc(ctx, a.Pool, func(tx pgx.Tx) error {
		q := a.Q.WithTx(tx)
		pos, err := q.OnboardingNextSLAPosition(ctx, st.org)
		if err != nil {
			return err
		}
		hours := pgtype.Int8{}
		if st.hoursID != 0 {
			hours = i8(st.hoursID)
		}
		cond := map[string]any{}
		if len(sp.Priorities) > 0 {
			cond["priorities"] = sp.Priorities
		}
		row, err := q.InsertSLAPolicy(ctx, db.InsertSLAPolicyParams{
			OrganizationID: st.org, Name: sp.Name, Description: sp.Description, IsActive: true, Position: pos,
			BusinessHoursID: hours, Conditions: mustJSON(cond), WarningPercent: 80,
		})
		if err != nil {
			return err
		}
		for _, t := range sp.Targets {
			if err := q.InsertSLAPolicyTarget(ctx, db.InsertSLAPolicyTargetParams{
				OrganizationID: st.org, PolicyID: row.ID, Priority: t.Priority,
				FirstResponseMinutes: i4(t.FirstResponse), NextResponseMinutes: i4(t.NextResponse), ResolutionMinutes: i4(t.Resolution),
			}); err != nil {
				return err
			}
		}
		out = result{status: ItemCreated, id: row.ID, uuid: row.Uuid}
		return nil
	})
	return out, err
}

func (a *Applier) applyCustomField(ctx context.Context, st *applyState, f PlanCustomField) (result, error) {
	opts := []customfields.Option{}
	for _, o := range f.Options {
		v := strings.ReplaceAll(Slugify(o, 60), "-", "_")
		if v == "" {
			v = fmt.Sprintf("option_%d", len(opts)+1)
		}
		opts = append(opts, customfields.Option{Value: v, Label: o})
	}
	def := customfields.Definition{Key: f.Key, Label: f.Label, Type: customfields.FieldType(f.Type), Options: opts}
	if errs := customfields.ValidateDefinition(def, nil); len(errs) > 0 {
		return result{}, fmt.Errorf("custom field %s: %s", f.Key, errs[0].Message)
	}
	var optJSON []byte
	if len(opts) > 0 {
		optJSON = mustJSON(opts)
	} else {
		optJSON = []byte(`[]`)
	}
	row, err := a.Q.CreateCustomFieldDefinition(ctx, db.CreateCustomFieldDefinitionParams{
		OrganizationID: st.org, EntityType: f.Entity, Key: f.Key, Label: f.Label, FieldType: f.Type,
		Options: optJSON, Config: []byte(`{}`),
		// CF-196: live at once, every surface, identified visitors may send it.
		Visibility: mustJSON(customfields.DefaultVisibility), WidgetAccess: customfields.WidgetAccessVerified,
	})
	if err != nil {
		return result{}, err
	}
	st.fields[f.Key], st.fieldTypes[f.Key] = row.Uuid, f.Entity
	return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
}

// formField mirrors forms/engine.Field.
type formField struct {
	Key               string     `json:"key"`
	Source            string     `json:"source"`
	CustomFieldUUID   *uuid.UUID `json:"custom_field_uuid,omitempty"`
	Label             string     `json:"label"`
	Help              string     `json:"help"`
	Required          bool       `json:"required"`
	RequiredToResolve bool       `json:"required_to_resolve"`
}

func (a *Applier) applyForm(ctx context.Context, st *applyState, f PlanForm) (result, error) {
	fields := []formField{}
	for _, ff := range f.Fields {
		key, custom := strings.CutPrefix(ff.Field, "custom:")
		if !custom {
			fields = append(fields, formField{Key: ff.Field, Source: "system", Label: ff.Label, Required: ff.Required})
			continue
		}
		id, ok := st.fields[key]
		if !ok {
			continue // the custom field was unticked or failed
		}
		fid := id
		entity := st.fieldTypes[key]
		fields = append(fields, formField{Key: entity + "." + key, Source: entity, CustomFieldUUID: &fid, Label: ff.Label, Required: ff.Required})
	}
	if len(fields) == 0 {
		return result{status: ItemSkipped, reason: "no_fields"}, nil
	}
	pos, err := a.Q.OnboardingNextFormPosition(ctx, st.org)
	if err != nil {
		return result{}, err
	}
	row, err := a.Q.InsertRequestForm(ctx, db.InsertRequestFormParams{
		OrganizationID: st.org, Name: f.Name, Description: f.Description, IsActive: true,
		Channels: []string{"widget", "web"}, Fields: mustJSON(fields), Position: pos,
	})
	if err != nil {
		return result{}, err
	}
	return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
}

func (a *Applier) applyArticle(ctx context.Context, st *applyState, it Item) (result, error) {
	slug := strings.TrimPrefix(it.Key, KindArticle+":")
	colSlug := strings.TrimPrefix(it.Parent, KindCollection+":")
	for _, c := range st.plan.Knowledge {
		if c.Slug != colSlug {
			continue
		}
		for _, art := range c.Articles {
			if art.Slug != slug {
				continue
			}
			col, ok := st.cols[colSlug]
			if !ok {
				return result{status: ItemSkipped, reason: "no_collection"}, nil
			}
			visibility := "internal"
			if art.Public {
				visibility = "public"
			}
			id := uuid.New()
			row, err := a.KB.SaveArticle(ctx, st.org, st.actor, id, knowledgemodel.ArticleInput{
				Collection: &col, Slug: art.Slug, Locale: st.plan.Summary.Locale, Title: art.Title, Body: art.Body,
				Tags: []string{"onboarding"}, Visibility: visibility,
			}, true)
			if err != nil {
				return result{}, err
			}
			if err := a.KB.Publish(ctx, st.org, row.Uuid, row.Version, true); err != nil {
				return result{}, err
			}
			return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
		}
	}
	return result{}, fmt.Errorf("onboarding: article %s not in plan", slug)
}

func (a *Applier) applyWidget(ctx context.Context, st *applyState) (result, error) {
	w, err := a.Q.OnboardingFirstWidget(ctx, st.org)
	if err != nil {
		return result{}, err
	}
	settings := map[string]any{}
	if len(w.Settings) > 0 {
		_ = json.Unmarshal(w.Settings, &settings)
	}
	prev := mustJSON(map[string]any{"settings": json.RawMessage(nonEmptyJSON(w.Settings))})
	pw := st.plan.Widget
	if pw.GreetingTitle != "" {
		settings["greeting_title"] = pw.GreetingTitle
	}
	if pw.GreetingText != "" {
		settings["greeting_text"] = pw.GreetingText
	}
	if pw.Color != "" {
		settings["color"] = pw.Color
	}
	if _, err := a.Q.OnboardingSetWidgetSettings(ctx, db.OnboardingSetWidgetSettingsParams{OrganizationID: st.org, ID: w.ID, Settings: mustJSON(settings)}); err != nil {
		return result{}, err
	}
	return result{status: ItemUpdated, id: w.ID, uuid: w.Uuid, previous: prev}, nil
}

func (a *Applier) applyPersona(ctx context.Context, st *applyState) (result, error) {
	type agentConfig = map[string]any
	cfg := agentConfig{}
	var prev []byte
	row, err := a.Q.AIAgentConfig(ctx, st.org)
	switch {
	case err == nil:
		prev = mustJSON(map[string]any{"config": json.RawMessage(nonEmptyJSON(row.Config))})
		_ = json.Unmarshal(row.Config, &cfg)
	case errors.Is(err, pgx.ErrNoRows):
		prev = []byte(`{"config":null}`)
		// Same defaults as aiagents/model.DefaultConfig (mode stays off).
		cfg = agentConfig{"mode": "off", "locale": "tr", "timezone": "Europe/Istanbul", "confidence_threshold": 0.85,
			"channels": []string{}, "categories": []string{}, "weekdays": []int{0, 1, 2, 3, 4, 5, 6}, "end_minute": 1440,
			"handoff_keywords": []string{"insan", "temsilci", "human", "agent"}}
	default:
		return result{}, err
	}
	ag := st.plan.AIAgent
	cfg["persona"] = ag.Persona
	cfg["locale"] = st.plan.Summary.Locale
	if len(ag.HandoffKeywords) > 0 {
		cfg["handoff_keywords"] = ag.HandoffKeywords
	}
	saved, err := a.Q.AIAgentSaveConfig(ctx, db.AIAgentSaveConfigParams{OrganizationID: st.org, Config: mustJSON(cfg), UpdatedBy: i8(st.actor)})
	if err != nil {
		return result{}, err
	}
	return result{status: ItemUpdated, id: saved.ID, previous: prev}, nil
}

// applyGuardrails writes the tone and the "never do" rules as business
// context sections the agents read (one section each, created only).
func (a *Applier) applyGuardrails(ctx context.Context, st *applyState) (result, error) {
	ag := st.plan.AIAgent
	sections := []struct{ kind, title, body string }{}
	if ag.Tone != "" {
		sections = append(sections, struct{ kind, title, body string }{"tone", "Yanıt tonu (kurulum)", ag.Tone})
	}
	if len(ag.Guardrails) > 0 {
		sections = append(sections, struct{ kind, title, body string }{"forbidden", "Yapılmaması gerekenler (kurulum)", "- " + strings.Join(ag.Guardrails, "\n- ")})
	}
	ids := []int64{}
	err := pgx.BeginFunc(ctx, a.Pool, func(tx pgx.Tx) error {
		q := a.Q.WithTx(tx)
		count, err := q.OnboardingCountContextSections(ctx, st.org)
		if err != nil {
			return err
		}
		for i, s := range sections {
			if count+int64(i) >= 20 {
				break
			}
			if err := ai.Guard(s.body); err != nil {
				return err
			}
			if _, ferr := q.OnboardingFindContextSection(ctx, db.OnboardingFindContextSectionParams{OrganizationID: st.org, Kind: s.kind, Title: s.title}); ferr == nil {
				continue
			}
			row, err := q.BusinessContextCreate(ctx, db.BusinessContextCreateParams{
				OrganizationID: st.org, Kind: s.kind, Title: s.title, Body: s.body, Active: true,
				SortOrder: safeconv.ToInt32(int(count) + i), UpdatedBy: st.actor,
			})
			if err != nil {
				return err
			}
			if err := q.BusinessContextSnapshot(ctx, db.BusinessContextSnapshotParams{OrganizationID: st.org, ID: row.ID}); err != nil {
				return err
			}
			ids = append(ids, row.ID)
		}
		return nil
	})
	if err != nil {
		return result{}, err
	}
	if len(ids) == 0 {
		return result{status: ItemSkipped, reason: "exists"}, nil
	}
	return result{status: ItemCreated, previous: mustJSON(map[string]any{"section_ids": ids})}, nil
}

func (a *Applier) applyRoutingRule(ctx context.Context, st *applyState, r PlanRoutingRule) (result, error) {
	team, ok := st.teams[r.Team]
	if !ok {
		return result{status: ItemSkipped, reason: "no_team"}, nil
	}
	cond := map[string]any{"channel_types": nonNil(r.ChannelTypes), "priorities": nonNil(r.Priorities), "tag_uuids": []string{}}
	pos, err := a.Q.OnboardingNextRoutingPosition(ctx, st.org)
	if err != nil {
		return result{}, err
	}
	row, err := a.Q.InsertRoutingRule(ctx, db.InsertRoutingRuleParams{
		OrganizationID: st.org, Name: r.Name, Position: pos, IsActive: true, Conditions: mustJSON(cond),
		TeamID: i8(team), Skills: []string{},
	})
	if err != nil {
		return result{}, err
	}
	return result{status: ItemCreated, id: row.ID, uuid: row.Uuid}, nil
}

// applyNotifications sets the owner's digest only when they never chose
// their own notification settings.
func (a *Applier) applyNotifications(ctx context.Context, st *applyState) (result, error) {
	_, err := a.Q.GetNotificationPreferences(ctx, st.actor)
	if ok, ferr := found(err); ferr != nil {
		return result{}, ferr
	} else if ok {
		return result{status: ItemSkipped, reason: "exists"}, nil
	}
	n := st.plan.Notifications
	tz := timezoneOf(st.plan)
	if tz == "" {
		tz = "Europe/Istanbul"
	}
	if _, err := a.Q.UpsertNotificationPreferences(ctx, db.UpsertNotificationPreferencesParams{
		UserID: st.actor, EmailEnabled: true, InappEnabled: true, RealtimeEnabled: true, PushEnabled: true,
		EventChannels: []byte(`{}`), QuietHoursStart: "22:00", QuietHoursEnd: "08:00", Timezone: tz,
		EmailDigest: n.EmailDigest, DigestHour: safeconv.ToInt16(n.DigestHour),
	}); err != nil {
		return result{}, err
	}
	return result{status: ItemCreated, id: st.actor}, nil
}

// undo reverses one ledger row. Entities a person may have started using
// are removed only where the owning table allows it safely; the rest
// report why they stayed.
func (a *Applier) undo(ctx context.Context, org, actor int64, row db.OnboardingSetupItem) error {
	q := a.Q
	id := row.EntityID.Int64
	eu := uuid.UUID(row.EntityUuid.Bytes)
	gone := func(n int64, err error) error {
		if err != nil {
			return err
		}
		if n == 0 {
			return errInUse
		}
		return nil
	}
	switch row.Kind {
	case KindBranding:
		var prev struct {
			Branding orgmodel.Branding `json:"branding"`
			Website  string            `json:"website"`
		}
		_ = json.Unmarshal(row.Previous, &prev)
		in := orgmodel.PatchProfileInput{Branding: &prev.Branding}
		if prof, err := a.Profiles.GetProfile(ctx, org); err == nil && prev.Website == "" {
			c := prof.Company
			c.Website = ""
			in.Company = &c
		}
		_, _, err := a.Profiles.PatchProfile(ctx, org, in)
		return err
	case KindBusinessHours:
		return gone(q.DeleteBusinessHours(ctx, db.DeleteBusinessHoursParams{OrganizationID: org, ID: id}))
	case KindTeam:
		return gone(q.DeleteTeam(ctx, db.DeleteTeamParams{OrganizationID: org, Uuid: eu}))
	case KindTag:
		return gone(q.DeleteTag(ctx, db.DeleteTagParams{OrganizationID: org, ID: id}))
	case KindStatus:
		return gone(q.OnboardingDeleteStatus(ctx, db.OnboardingDeleteStatusParams{OrganizationID: org, ID: id}))
	case KindSLAPolicy:
		return gone(q.DeleteSLAPolicy(ctx, db.DeleteSLAPolicyParams{OrganizationID: org, ID: id}))
	case KindCustomField:
		return gone(q.OnboardingDeleteCustomField(ctx, db.OnboardingDeleteCustomFieldParams{OrganizationID: org, ID: id}))
	case KindCannedReply:
		return gone(q.DeleteCannedResponse(ctx, db.DeleteCannedResponseParams{OrganizationID: org, ID: id}))
	case KindMacro:
		return gone(q.DeleteMacro(ctx, db.DeleteMacroParams{OrganizationID: org, ID: id}))
	case KindForm:
		return gone(q.DeleteRequestForm(ctx, db.DeleteRequestFormParams{OrganizationID: org, Uuid: eu}))
	case KindArticle:
		return a.KB.DeleteArticle(ctx, org, eu)
	case KindCollection:
		live, err := q.OnboardingCountLiveArticles(ctx, db.OnboardingCountLiveArticlesParams{OrganizationID: org, CollectionID: i8(id)})
		if err != nil {
			return err
		}
		if live > 0 {
			return errInUse
		}
		if err := q.OnboardingDetachDeletedArticles(ctx, db.OnboardingDetachDeletedArticlesParams{OrganizationID: org, CollectionID: i8(id)}); err != nil {
			return err
		}
		return a.KB.DeleteCollection(ctx, org, eu)
	case KindWidget:
		var prev struct {
			Settings json.RawMessage `json:"settings"`
		}
		_ = json.Unmarshal(row.Previous, &prev)
		return gone(q.OnboardingSetWidgetSettings(ctx, db.OnboardingSetWidgetSettingsParams{OrganizationID: org, ID: id, Settings: nonEmptyJSON(prev.Settings)}))
	case KindAIPersona:
		var prev struct {
			Config json.RawMessage `json:"config"`
		}
		_ = json.Unmarshal(row.Previous, &prev)
		cfg := []byte(prev.Config)
		if len(cfg) == 0 || string(cfg) == "null" {
			// There was no configuration: keep the defaults, drop the persona.
			cur, err := q.AIAgentConfig(ctx, org)
			if err != nil {
				return err
			}
			m := map[string]any{}
			_ = json.Unmarshal(cur.Config, &m)
			m["persona"] = ""
			cfg = mustJSON(m)
		}
		_, err := q.AIAgentSaveConfig(ctx, db.AIAgentSaveConfigParams{OrganizationID: org, Config: cfg, UpdatedBy: pgtype.Int8{}})
		return err
	case KindAIGuardrail:
		var prev struct {
			SectionIDs []int64 `json:"section_ids"`
		}
		_ = json.Unmarshal(row.Previous, &prev)
		return pgx.BeginFunc(ctx, a.Pool, func(tx pgx.Tx) error {
			tq := q.WithTx(tx)
			for _, sid := range prev.SectionIDs {
				n, err := tq.OnboardingDeactivateContextSection(ctx, db.OnboardingDeactivateContextSectionParams{OrganizationID: org, ID: sid, UpdatedBy: actor})
				if err != nil {
					return err
				}
				if n == 0 {
					continue
				}
				if err := tq.BusinessContextSnapshot(ctx, db.BusinessContextSnapshotParams{OrganizationID: org, ID: sid}); err != nil {
					return err
				}
			}
			return nil
		})
	case KindAIProcedure:
		return gone(q.OnboardingDeleteProcedure(ctx, db.OnboardingDeleteProcedureParams{OrganizationID: org, ID: id}))
	case KindRoutingRule:
		return gone(q.DeleteRoutingRule(ctx, db.DeleteRoutingRuleParams{OrganizationID: org, Uuid: eu}))
	case KindView:
		return gone(q.DeleteConversationView(ctx, db.DeleteConversationViewParams{OrganizationID: org, ID: id}))
	case KindSurvey:
		return gone(q.OnboardingDeleteSurvey(ctx, db.OnboardingDeleteSurveyParams{OrganizationID: org, ID: id}))
	case KindAppointment:
		if a.Appointments == nil {
			return errInUse
		}
		return a.Appointments.Delete(ctx, org, eu)
	case KindNotifications:
		_, err := q.OnboardingDeleteNotificationPreferences(ctx, id)
		return err
	}
	return nil
}

// errInUse: the entity is referenced or was already removed; undo leaves it.
var errInUse = errors.New("in use or already removed")

func findTeam(p *SetupPlan, key string) PlanTeam {
	for _, t := range p.Teams {
		if t.Key == key {
			return t
		}
	}
	return PlanTeam{Key: key, Name: key}
}

func timezoneOf(p *SetupPlan) string {
	if p.BusinessHours != nil {
		return p.BusinessHours.Timezone
	}
	return ""
}

func workingHours(days []PlanDay) map[string]any {
	out := map[string]any{}
	for _, d := range days {
		out[d.Day] = map[string]any{"closed": d.Closed, "open": d.Open, "close": d.Close}
	}
	return out
}

func workingHoursOrNull(p *SetupPlan) []byte {
	if p.BusinessHours == nil {
		return nil
	}
	return mustJSON(workingHours(p.BusinessHours.Days))
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func nonEmptyJSON(b []byte) []byte {
	if len(b) == 0 {
		return []byte(`{}`)
	}
	return b
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func i8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func i4(v int) pgtype.Int4 { return pgtype.Int4{Int32: safeconv.ToInt32(v), Valid: v > 0} }

func textOrNull(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
