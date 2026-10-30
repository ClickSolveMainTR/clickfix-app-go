// Package guides serves the module guides of CF-141: coach-mark tours and
// "what is this page for?" cards for every web route and the main mobile
// screens, the ordered set-up tour, per-user progress and the platform
// guide editor (with AI drafting).
//
// Guides are platform content (not organization data): one row per
// (key, locale) with the steps as JSON. The rows shipped with the code
// (seed/guides.json) are written on boot and refreshed while no platform
// admin has edited them. Progress is per user and organization.
package guides

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// Vocabularies (mirrored by the CHECK constraints of migration 000203).
var (
	Surfaces   = []string{"web", "mobile"}
	Locales    = []string{"tr", "en"}
	Kinds      = []string{"tour", "page"}
	Statuses   = []string{"new", "in_progress", "completed", "skipped"}
	Placements = []string{"auto", "top", "bottom", "left", "right"}
	// ActionKinds are what a step's button may do on the client.
	ActionKinds = []string{"navigate", "open_assistant", "none"}
)

// Limits.
const (
	MaxSteps      = 20
	maxTitle      = 200
	maxSummary    = 2000
	maxStepBody   = 4000
	maxStepTitle  = 160
	maxTarget     = 100
	maxRoute      = 200
	maxSearchRows = 6
)

// Errors.
var (
	ErrInvalid  = errors.New("guides: invalid input")
	ErrNotFound = errors.New("guides: not found")
	ErrConflict = errors.New("guides: key already exists for this locale")
)

// StepAction is what a step's primary button does besides "Next".
type StepAction struct {
	Kind  string `json:"kind"`
	Route string `json:"route,omitempty"`
}

// Step is one coach mark. Target is a data-guide id on web, a testID /
// guide id on mobile; "" shows a centered modal.
type Step struct {
	ID        string      `json:"id"`
	Target    string      `json:"target"`
	Title     string      `json:"title"`
	Body      string      `json:"body"`
	Placement string      `json:"placement"`
	Action    *StepAction `json:"action,omitempty"`
}

// Progress is the caller's state of one guide.
type Progress struct {
	Status       string `json:"status"`
	StepIndex    int    `json:"step_index"`
	DismissSetup bool   `json:"dismiss_setup,omitempty"`
	Restart      bool   `json:"restart,omitempty"`
}

// Guide is the tenant view of one guide.
type Guide struct {
	Key      string   `json:"key"`
	Surface  string   `json:"surface"`
	Route    string   `json:"route"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Kind     string   `json:"kind"`
	Order    int      `json:"order"`
	Steps    []Step   `json:"steps"`
	Progress Progress `json:"progress"`
}

// AdminGuide is the platform editor view.
type AdminGuide struct {
	UUID        uuid.UUID `json:"uuid"`
	Key         string    `json:"key"`
	Locale      string    `json:"locale"`
	Surface     string    `json:"surface"`
	Route       string    `json:"route"`
	Kind        string    `json:"kind"`
	Order       int       `json:"order"`
	SetupOrder  int       `json:"setup_order"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Steps       []Step    `json:"steps"`
	Version     int       `json:"version"`
	Source      string    `json:"source"`
	IsPublished bool      `json:"is_published"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Input creates or replaces a guide (platform editor).
type Input struct {
	Key         string `json:"key"`
	Locale      string `json:"locale"`
	Surface     string `json:"surface"`
	Route       string `json:"route"`
	Kind        string `json:"kind"`
	Order       int    `json:"order"`
	SetupOrder  int    `json:"setup_order"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Steps       []Step `json:"steps"`
	IsPublished *bool  `json:"is_published"`
}

// Queries is the sqlc subset the service uses.
type Queries interface {
	GuideAutoStartClaim(context.Context, db.GuideAutoStartClaimParams) (int64, error)
	GuidePreferencesGet(context.Context, int64) (bool, error)
	GuidePreferencesSet(context.Context, db.GuidePreferencesSetParams) error
	GuideSetupSkip(context.Context, db.GuideSetupSkipParams) error
	GuidesForRoute(ctx context.Context, arg db.GuidesForRouteParams) ([]db.Guide, error)
	GuidesSetup(ctx context.Context, arg db.GuidesSetupParams) ([]db.Guide, error)
	GuideByKey(ctx context.Context, arg db.GuideByKeyParams) (db.Guide, error)
	GuideKeyExists(ctx context.Context, key string) (bool, error)
	GuideByUUID(ctx context.Context, id uuid.UUID) (db.Guide, error)
	GuidesSearch(ctx context.Context, arg db.GuidesSearchParams) ([]db.Guide, error)
	GuidesPlatformList(ctx context.Context, arg db.GuidesPlatformListParams) ([]db.Guide, error)
	GuidesPlatformCount(ctx context.Context, arg db.GuidesPlatformCountParams) (int64, error)
	GuideCreate(ctx context.Context, arg db.GuideCreateParams) (db.Guide, error)
	GuideUpdate(ctx context.Context, arg db.GuideUpdateParams) (db.Guide, error)
	GuideDelete(ctx context.Context, id uuid.UUID) (int64, error)
	GuideSeedUpsert(ctx context.Context, arg db.GuideSeedUpsertParams) error
	GuideProgressList(ctx context.Context, arg db.GuideProgressListParams) ([]db.GuideProgressListRow, error)
	GuideProgressUpsert(ctx context.Context, arg db.GuideProgressUpsertParams) error
	GuideProgressReset(ctx context.Context, arg db.GuideProgressResetParams) error
	ProductFeedbackCreate(ctx context.Context, arg db.ProductFeedbackCreateParams) (uuid.UUID, error)
	ProductFeedbackAttachmentsOwned(ctx context.Context, arg db.ProductFeedbackAttachmentsOwnedParams) (int64, error)
	ProductFeedbackList(ctx context.Context, arg db.ProductFeedbackListParams) ([]db.ProductFeedbackListRow, error)
	ProductFeedbackCount(ctx context.Context, arg db.ProductFeedbackCountParams) (int64, error)
	ProductFeedbackSetStatus(ctx context.Context, arg db.ProductFeedbackSetStatusParams) (int64, error)
}

// Drafter writes guide drafts with the platform AI (the assistant module
// implements it; nil disables POST /v1/platform/guides/generate).
type Drafter interface {
	Draft(ctx context.Context, actor int64, instructions, input string, schema map[string]any) (string, error)
}

// Service is the guides use case.
type Service struct {
	Q     Queries
	Draft Drafter
}

// New builds the service.
func New(q Queries) *Service { return &Service{Q: q} }

func oneOf(v string, list []string) bool {
	for _, x := range list {
		if v == x {
			return true
		}
	}
	return false
}

// NormalizeLocale maps anything but "en" to "tr" (the product default).
func NormalizeLocale(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if strings.HasPrefix(l, "en") {
		return "en"
	}
	return "tr"
}

// NormalizeSurface defaults to web.
func NormalizeSurface(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "web", nil
	}
	if !oneOf(s, Surfaces) {
		return "", ErrInvalid
	}
	return s, nil
}

// NormalizeRoute trims the query string, hash and trailing slash.
func NormalizeRoute(r string) string {
	r = strings.TrimSpace(r)
	if i := strings.IndexAny(r, "?#"); i >= 0 {
		r = r[:i]
	}
	if len(r) > 1 {
		r = strings.TrimRight(r, "/")
	}
	if r == "" {
		return ""
	}
	if !strings.HasPrefix(r, "/") {
		r = "/" + r
	}
	return r
}

// RouteMatches reports whether a concrete path (or the pattern itself)
// matches a guide route pattern; "[param]" segments match any segment.
func RouteMatches(pattern, path string) bool {
	if pattern == path {
		return true
	}
	ps := strings.Split(strings.Trim(pattern, "/"), "/")
	xs := strings.Split(strings.Trim(path, "/"), "/")
	if len(ps) != len(xs) {
		return false
	}
	for i := range ps {
		if strings.HasPrefix(ps[i], "[") && strings.HasSuffix(ps[i], "]") {
			continue
		}
		if ps[i] != xs[i] {
			return false
		}
	}
	return true
}

func decodeSteps(raw []byte) []Step {
	steps := []Step{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &steps)
	}
	return steps
}

func toGuide(g db.Guide, p map[string]Progress) Guide {
	out := Guide{
		Key: g.Key, Surface: g.Surface, Route: g.Route, Title: g.Title, Summary: g.Summary, Kind: g.Kind,
		Order: int(g.SortOrder), Steps: decodeSteps(g.Steps), Progress: Progress{Status: "new"},
	}
	if pr, ok := p[progressKey(g.Key)]; ok {
		out.Progress = pr
	}
	return out
}

func toAdmin(g db.Guide) AdminGuide {
	return AdminGuide{
		UUID: g.Uuid, Key: g.Key, Locale: g.Locale, Surface: g.Surface, Route: g.Route, Kind: g.Kind,
		Order: int(g.SortOrder), SetupOrder: int(g.SetupOrder), Title: g.Title, Summary: g.Summary,
		Steps: decodeSteps(g.Steps), Version: int(g.Version), Source: g.Source, IsPublished: g.IsPublished,
		UpdatedAt: g.UpdatedAt.Time,
	}
}

func (s *Service) progress(ctx context.Context, org, user int64) (map[string]Progress, error) {
	rows, err := s.Q.GuideProgressList(ctx, db.GuideProgressListParams{OrganizationID: org, UserID: user})
	if err != nil {
		return nil, fmt.Errorf("guides: progress: %w", err)
	}
	out := make(map[string]Progress, len(rows))
	canonical := make(map[string]bool, len(rows))
	for _, r := range rows {
		key := progressKey(r.GuideKey)
		// An explicit manual replay updates the canonical record; legacy aliases cannot override it.
		if r.GuideKey == key {
			out[key] = Progress{Status: r.Status, StepIndex: int(r.StepIndex)}
			canonical[key] = true
			continue
		}
		if canonical[key] {
			continue
		}
		// Until migrated, a legacy dismissal wins over stale progress on the other surface.
		old := out[key]
		if old.Status != "skipped" && old.Status != "completed" {
			out[key] = Progress{Status: r.Status, StepIndex: int(r.StepIndex)}
		}
	}
	return out, nil
}

// routeGuides returns the published guides of a surface/locale whose
// route matches; English falls back to Turkish when nothing is written.
func (s *Service) routeGuides(ctx context.Context, surface, locale, route string) ([]db.Guide, error) {
	load := func(loc string) ([]db.Guide, error) {
		rows, err := s.Q.GuidesForRoute(ctx, db.GuidesForRouteParams{Surface: surface, Locale: loc, Route: ""})
		if err != nil {
			return nil, fmt.Errorf("guides: list: %w", err)
		}
		if route == "" {
			return rows, nil
		}
		out := rows[:0]
		for _, g := range rows {
			if RouteMatches(g.Route, route) {
				out = append(out, g)
			}
		}
		return out, nil
	}
	rows, err := load(locale)
	if err != nil || len(rows) > 0 || locale == "tr" {
		return rows, err
	}
	return load("tr")
}

// ForRoute lists the guides of a route with the caller's progress.
func (s *Service) ForRoute(ctx context.Context, org, user int64, surface, route, locale string) ([]Guide, error) {
	surface, err := NormalizeSurface(surface)
	if err != nil {
		return nil, err
	}
	route = NormalizeRoute(route)
	if len(route) > maxRoute {
		return nil, ErrInvalid
	}
	rows, err := s.routeGuides(ctx, surface, NormalizeLocale(locale), route)
	if err != nil {
		return nil, err
	}
	prog, err := s.progress(ctx, org, user)
	if err != nil {
		return nil, err
	}
	out := make([]Guide, 0, len(rows))
	for _, g := range rows {
		out = append(out, toGuide(g, prog))
	}
	return out, nil
}

// SetupTour is the ordered onboarding checklist.
type SetupTour struct {
	Items     []Guide `json:"items"`
	Completed int     `json:"completed"`
	Total     int     `json:"total"`
	AutoStart bool    `json:"auto_start"`
}

// Setup returns the set-up tour; completed counts guides the caller
// finished or skipped.
func (s *Service) Setup(ctx context.Context, org, user int64, surface, locale string) (SetupTour, error) {
	surface, err := NormalizeSurface(surface)
	if err != nil {
		return SetupTour{}, err
	}
	locale = NormalizeLocale(locale)
	rows, err := s.Q.GuidesSetup(ctx, db.GuidesSetupParams{Surface: surface, Locale: locale})
	if err == nil && len(rows) == 0 && locale != "tr" {
		rows, err = s.Q.GuidesSetup(ctx, db.GuidesSetupParams{Surface: surface, Locale: "tr"})
	}
	if err != nil {
		return SetupTour{}, fmt.Errorf("guides: setup: %w", err)
	}
	prog, err := s.progress(ctx, org, user)
	if err != nil {
		return SetupTour{}, err
	}
	out := SetupTour{Items: make([]Guide, 0, len(rows)), Total: len(rows), AutoStart: prog["__setup__"].Status == ""}
	for _, g := range rows {
		item := toGuide(g, prog)
		if item.Progress.Status != "new" {
			out.AutoStart = false
		}
		if item.Progress.Status == "completed" || item.Progress.Status == "skipped" {
			out.Completed++
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// SaveProgress records the caller's position in one guide.
func (s *Service) SaveProgress(ctx context.Context, org, user int64, key string, p Progress) error {
	if !oneOf(p.Status, Statuses) || p.StepIndex < 0 || p.StepIndex > MaxSteps {
		return ErrInvalid
	}
	exists, err := s.Q.GuideKeyExists(ctx, key)
	if err != nil {
		return fmt.Errorf("guides: key: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	if p.DismissSetup && p.Status == "skipped" {
		if err := s.Q.GuideSetupSkip(ctx, db.GuideSetupSkipParams{OrganizationID: org, UserID: user}); err != nil {
			return err
		}
	}
	// #nosec G115 -- StepIndex is bounded by MaxSteps above.
	return s.Q.GuideProgressUpsert(ctx, db.GuideProgressUpsertParams{
		OrganizationID: org, UserID: user, GuideKey: progressKey(key), Status: p.Status, StepIndex: int32(p.StepIndex), Restart: p.Restart,
	})
}

// Reset forgets the caller's progress so every tour shows again.
func (s *Service) Reset(ctx context.Context, org, user int64) error {
	return s.Q.GuideProgressReset(ctx, db.GuideProgressResetParams{OrganizationID: org, UserID: user})
}

// PageContext is what the assistant knows about the page the user is on.
type PageContext struct {
	Key     string `json:"key"`
	Route   string `json:"route"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Steps   []Step `json:"steps"`
}

// Page returns the page guide of a route (or a guide by key), if any.
func (s *Service) Page(ctx context.Context, surface, route, key, locale string) (*PageContext, error) {
	surface, err := NormalizeSurface(surface)
	if err != nil {
		surface = "web"
	}
	locale = NormalizeLocale(locale)
	if key != "" {
		g, err := s.Q.GuideByKey(ctx, db.GuideByKeyParams{Key: key, Locale: locale})
		if err == nil {
			return &PageContext{Key: g.Key, Route: g.Route, Title: g.Title, Summary: g.Summary, Steps: decodeSteps(g.Steps)}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	rows, err := s.routeGuides(ctx, surface, locale, NormalizeRoute(route))
	if err != nil || route == "" {
		return nil, err
	}
	for _, g := range rows {
		if g.Kind == "page" {
			return &PageContext{Key: g.Key, Route: g.Route, Title: g.Title, Summary: g.Summary, Steps: decodeSteps(g.Steps)}, nil
		}
	}
	return nil, nil
}

// SearchHit is one guide match of the assistant's search_docs tool.
type SearchHit struct {
	Key     string `json:"key"`
	Route   string `json:"route"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Excerpt string `json:"excerpt"`
}

// Search finds guides by text (assistant search_docs).
func (s *Service) Search(ctx context.Context, query, locale string) ([]SearchHit, error) {
	query = strings.Join(strings.Fields(query), " ")
	if query == "" || utf8.RuneCountInString(query) > 200 {
		return nil, ErrInvalid
	}
	query = strings.NewReplacer("%", "", "_", " ").Replace(query)
	rows, err := s.Q.GuidesSearch(ctx, db.GuidesSearchParams{Locale: NormalizeLocale(locale), Query: query, MaxRows: maxSearchRows})
	if err != nil {
		return nil, err
	}
	out := make([]SearchHit, 0, len(rows))
	for _, g := range rows {
		hit := SearchHit{Key: g.Key, Route: g.Route, Title: g.Title, Summary: g.Summary}
		for _, st := range decodeSteps(g.Steps) {
			if strings.Contains(strings.ToLower(st.Title+" "+st.Body), strings.ToLower(query)) {
				hit.Excerpt = clip(st.Title+": "+st.Body, 400)
				break
			}
		}
		out = append(out, hit)
	}
	return out, nil
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

var (
	keyPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,99}$`)
	idPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	targetChars = regexp.MustCompile(`^[a-zA-Z0-9_.:-]*$`)
)

// ValidateSteps checks and normalizes steps in place.
func ValidateSteps(steps []Step) error {
	if len(steps) > MaxSteps {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for i := range steps {
		st := &steps[i]
		st.ID = strings.TrimSpace(st.ID)
		if st.ID == "" {
			st.ID = fmt.Sprintf("s%d", i+1)
		}
		st.Title = strings.TrimSpace(st.Title)
		st.Target = strings.TrimSpace(st.Target)
		if st.Placement == "" {
			st.Placement = "auto"
		}
		if !idPattern.MatchString(st.ID) || seen[st.ID] || st.Title == "" ||
			utf8.RuneCountInString(st.Title) > maxStepTitle || utf8.RuneCountInString(st.Body) > maxStepBody ||
			len(st.Target) > maxTarget || !targetChars.MatchString(st.Target) || !oneOf(st.Placement, Placements) {
			return ErrInvalid
		}
		seen[st.ID] = true
		if st.Action != nil {
			if !oneOf(st.Action.Kind, ActionKinds) {
				return ErrInvalid
			}
			st.Action.Route = NormalizeRoute(st.Action.Route)
			if st.Action.Kind == "navigate" && st.Action.Route == "" {
				return ErrInvalid
			}
			if st.Action.Kind == "none" {
				st.Action = nil
			}
		}
	}
	return nil
}

func (in *Input) validate() error {
	in.Key = strings.TrimSpace(in.Key)
	in.Title = strings.TrimSpace(in.Title)
	in.Summary = strings.TrimSpace(in.Summary)
	in.Route = NormalizeRoute(in.Route)
	if in.Kind == "" {
		in.Kind = "page"
	}
	if in.Steps == nil {
		in.Steps = []Step{}
	}
	if !keyPattern.MatchString(in.Key) || !oneOf(in.Locale, Locales) || !oneOf(in.Surface, Surfaces) ||
		!oneOf(in.Kind, Kinds) || in.Route == "" || len(in.Route) > maxRoute || in.Title == "" ||
		utf8.RuneCountInString(in.Title) > maxTitle || utf8.RuneCountInString(in.Summary) > maxSummary ||
		in.Order < 0 || in.Order > 100000 || in.SetupOrder < 0 || in.SetupOrder > 100 {
		return ErrInvalid
	}
	return ValidateSteps(in.Steps)
}

func actorParam(actor int64) pgtype.Int8 { return pgtype.Int8{Int64: actor, Valid: actor > 0} }

// AdminPage is a page of the platform guide list.
type AdminPage struct {
	Items  []AdminGuide `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// AdminList lists guides for the platform editor.
func (s *Service) AdminList(ctx context.Context, surface, locale, query string, limit, offset int) (AdminPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 || offset > 100000 {
		offset = 0
	}
	if (surface != "" && !oneOf(surface, Surfaces)) || (locale != "" && !oneOf(locale, Locales)) || len(query) > 100 {
		return AdminPage{}, ErrInvalid
	}
	query = strings.NewReplacer("%", "", "_", " ").Replace(strings.TrimSpace(query))
	// #nosec G115 -- limit/offset are bounded above.
	rows, err := s.Q.GuidesPlatformList(ctx, db.GuidesPlatformListParams{Surface: surface, Locale: locale, Query: query, MaxRows: int32(limit), SkipRows: int32(offset)})
	if err != nil {
		return AdminPage{}, err
	}
	total, err := s.Q.GuidesPlatformCount(ctx, db.GuidesPlatformCountParams{Surface: surface, Locale: locale, Query: query})
	if err != nil {
		return AdminPage{}, err
	}
	out := AdminPage{Items: make([]AdminGuide, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, g := range rows {
		out.Items = append(out.Items, toAdmin(g))
	}
	return out, nil
}

// AdminGet returns one guide.
func (s *Service) AdminGet(ctx context.Context, id uuid.UUID) (AdminGuide, error) {
	g, err := s.Q.GuideByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminGuide{}, ErrNotFound
	}
	if err != nil {
		return AdminGuide{}, err
	}
	return toAdmin(g), nil
}

// Create adds a guide (source custom, or ai when drafted).
func (s *Service) Create(ctx context.Context, actor int64, in Input, source string) (AdminGuide, error) {
	if err := in.validate(); err != nil {
		return AdminGuide{}, err
	}
	if source != "ai" {
		source = "custom"
	}
	steps, _ := json.Marshal(in.Steps)
	published := in.IsPublished == nil || *in.IsPublished
	if _, err := s.Q.GuideByKey(ctx, db.GuideByKeyParams{Key: in.Key, Locale: in.Locale}); err == nil {
		return AdminGuide{}, ErrConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return AdminGuide{}, err
	}
	// #nosec G115 -- Order/SetupOrder are bounded by validate.
	g, err := s.Q.GuideCreate(ctx, db.GuideCreateParams{
		Key: in.Key, Locale: in.Locale, Surface: in.Surface, Route: in.Route, Kind: in.Kind, SortOrder: int32(in.Order),
		SetupOrder: int32(in.SetupOrder), Title: in.Title, Summary: in.Summary, Steps: steps, Source: source,
		IsPublished: published, UpdatedBy: actorParam(actor),
	})
	if err != nil {
		return AdminGuide{}, err
	}
	return toAdmin(g), nil
}

// Update replaces a guide; the key and locale do not change.
func (s *Service) Update(ctx context.Context, actor int64, id uuid.UUID, in Input) (AdminGuide, error) {
	cur, err := s.Q.GuideByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminGuide{}, ErrNotFound
	}
	if err != nil {
		return AdminGuide{}, err
	}
	in.Key, in.Locale = cur.Key, cur.Locale
	if err := in.validate(); err != nil {
		return AdminGuide{}, err
	}
	steps, _ := json.Marshal(in.Steps)
	published := cur.IsPublished
	if in.IsPublished != nil {
		published = *in.IsPublished
	}
	// #nosec G115 -- Order/SetupOrder are bounded by validate.
	g, err := s.Q.GuideUpdate(ctx, db.GuideUpdateParams{
		Uuid: id, Surface: in.Surface, Route: in.Route, Kind: in.Kind, SortOrder: int32(in.Order),
		SetupOrder: int32(in.SetupOrder), Title: in.Title, Summary: in.Summary, Steps: steps,
		IsPublished: published, UpdatedBy: actorParam(actor),
	})
	if err != nil {
		return AdminGuide{}, err
	}
	return toAdmin(g), nil
}

// Delete removes a guide.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.Q.GuideDelete(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Surface-specific guide content shares one progress record per topic.
func progressKey(key string) string {
	aliases := map[string]string{"mobile.more_ai": "settings_ai", "mobile.more_channels": "channels", "mobile.more_team": "teams", "mobile.more_members": "settings_members", "mobile.more_reports": "reports", "mobile.agenda": "calendar"}
	if alias, ok := aliases[key]; ok {
		return alias
	}
	return strings.TrimPrefix(strings.TrimPrefix(key, "web."), "mobile.")
}

// ClaimAutoStart atomically marks a fresh guide as seen before any UI is shown.
func (s *Service) ClaimAutoStart(ctx context.Context, org, user int64, key string) (bool, error) {
	exists, err := s.Q.GuideKeyExists(ctx, key)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, ErrNotFound
	}
	n, err := s.Q.GuideAutoStartClaim(ctx, db.GuideAutoStartClaimParams{OrganizationID: org, UserID: user, GuideKey: progressKey(key), SourceKey: key})
	return n > 0, err
}

// Preferences follows the user across organizations and surfaces.
type Preferences struct {
	AutoStart bool `json:"auto_start"`
}

func (s *Service) Preferences(ctx context.Context, user int64) (Preferences, error) {
	on, err := s.Q.GuidePreferencesGet(ctx, user)
	return Preferences{AutoStart: on}, err
}
func (s *Service) SavePreferences(ctx context.Context, user int64, on bool) error {
	return s.Q.GuidePreferencesSet(ctx, db.GuidePreferencesSetParams{UserID: user, AutoStart: on})
}
