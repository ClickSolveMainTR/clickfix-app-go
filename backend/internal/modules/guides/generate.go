package guides

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// ErrAIUnavailable: no platform AI route is configured.
var ErrAIUnavailable = errors.New("guides: ai is not available")

// GenerateInput is the route metadata the AI drafts a guide from: the
// route, a free description and the page's visible strings (headings,
// button labels, card titles) and data-guide targets.
type GenerateInput struct {
	Surface     string   `json:"surface"`
	Route       string   `json:"route"`
	Locale      string   `json:"locale"`
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Strings     []string `json:"strings"`
	Targets     []string `json:"targets"`
	// Save stores the draft (source = ai) instead of only returning it.
	Save bool `json:"save"`
}

const generateInstructions = `You write in-product guides for ClickFix, a B2B customer-support platform (shared inbox,
channels, SLA, AI agents, help center, reports). From the route metadata produce ONE page guide: a short title,
a one-paragraph summary that answers "what is this page for?", and 3-6 coach-mark steps that explain the key cards
and buttons. Use only the given targets as step targets (or "" for a centered intro step). Bodies are short markdown
(1-3 sentences), practical, friendly and specific; never invent features that are not suggested by the strings.
Write in the language of "locale" (tr = Turkish, en = English). Step ids: lowercase a-z0-9_-.`

func generateSchema() map[string]any {
	str := map[string]any{"type": "string"}
	step := map[string]any{"type": "object", "additionalProperties": false,
		"required":   []string{"id", "target", "title", "body", "placement"},
		"properties": map[string]any{"id": str, "target": str, "title": str, "body": str, "placement": map[string]any{"type": "string", "enum": Placements}}}
	return map[string]any{"type": "object", "additionalProperties": false,
		"required":   []string{"title", "summary", "steps"},
		"properties": map[string]any{"title": str, "summary": str, "steps": map[string]any{"type": "array", "items": step}}}
}

// Generate drafts a guide for a route with the platform AI.
func (s *Service) Generate(ctx context.Context, actor int64, in GenerateInput) (Input, *AdminGuide, error) {
	if s.Draft == nil {
		return Input{}, nil, ErrAIUnavailable
	}
	surface, err := NormalizeSurface(in.Surface)
	if err != nil {
		return Input{}, nil, err
	}
	in.Route = NormalizeRoute(in.Route)
	in.Locale = NormalizeLocale(in.Locale)
	if in.Route == "" || len(in.Route) > maxRoute || utf8.RuneCountInString(in.Description) > 2000 || len(in.Strings) > 200 || len(in.Targets) > 50 {
		return Input{}, nil, ErrInvalid
	}
	if in.Key == "" {
		in.Key = surface + "." + strings.Trim(strings.NewReplacer("/", ".", "[", "", "]", "").Replace(in.Route), ".")
		if in.Route == "/" {
			in.Key = surface + ".dashboard"
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"surface": surface, "route": in.Route, "locale": in.Locale, "description": in.Description,
		"strings": in.Strings, "targets": in.Targets,
	})
	if len(payload) > 16000 {
		return Input{}, nil, ErrInvalid
	}
	text, err := s.Draft.Draft(ctx, actor, generateInstructions, string(payload), generateSchema())
	if err != nil {
		return Input{}, nil, err
	}
	var draft SeedText
	if err := json.Unmarshal([]byte(text), &draft); err != nil {
		return Input{}, nil, ErrAIUnavailable
	}
	allowed := map[string]bool{"": true}
	for _, t := range in.Targets {
		allowed[t] = true
	}
	for i := range draft.Steps {
		if !allowed[draft.Steps[i].Target] {
			draft.Steps[i].Target = ""
		}
	}
	if len(draft.Steps) > MaxSteps {
		draft.Steps = draft.Steps[:MaxSteps]
	}
	out := Input{Key: in.Key, Locale: in.Locale, Surface: surface, Route: in.Route, Kind: "page",
		Title: clip(draft.Title, maxTitle-1), Summary: clip(draft.Summary, maxSummary-1), Steps: draft.Steps}
	if err := out.validate(); err != nil {
		return Input{}, nil, err
	}
	if !in.Save {
		return out, nil, nil
	}
	saved, err := s.Create(ctx, actor, out, "ai")
	if err != nil {
		return out, nil, err
	}
	return out, &saved, nil
}
