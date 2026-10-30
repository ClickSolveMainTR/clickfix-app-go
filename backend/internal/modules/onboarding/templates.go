package onboarding

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Curated use-case templates (CF-140). Each file under templates/ is one
// versioned gallery entry carrying a full SetupPlan; bump "version" when
// its content changes so runs record which edition they applied.
//
//go:embed templates/*.json
var templateFS embed.FS

// Localized is a Turkish / English text pair.
type Localized struct {
	TR string `json:"tr"`
	EN string `json:"en"`
}

// Template is one gallery entry.
type Template struct {
	Key         string      `json:"key"`
	Version     int         `json:"version"`
	Icon        string      `json:"icon"`
	Accent      string      `json:"accent"`
	Order       int         `json:"order"`
	Name        Localized   `json:"name"`
	Description Localized   `json:"description"`
	Highlights  []Localized `json:"highlights"`
	// Industries are the questionnaire answers this template fits best;
	// discovery falls back to the closest one when AI is unavailable.
	Industries []string  `json:"industries"`
	Plan       SetupPlan `json:"plan"`
}

// TemplateSummary is the gallery card (the plan stays server side until
// the owner picks the template).
type TemplateSummary struct {
	Key         string         `json:"key"`
	Version     int            `json:"version"`
	Icon        string         `json:"icon"`
	Accent      string         `json:"accent"`
	Name        Localized      `json:"name"`
	Description Localized      `json:"description"`
	Highlights  []Localized    `json:"highlights"`
	Industries  []string       `json:"industries"`
	Counts      map[string]int `json:"counts"`
}

// ErrUnknownTemplate: no gallery template has this key.
var ErrUnknownTemplate = errors.New("unknown setup template")

var templateKeyRE = slugRE

// LoadTemplates parses and normalizes every embedded template.
func LoadTemplates() ([]Template, error) {
	return loadTemplates(templateFS)
}

func loadTemplates(fsys fs.FS) ([]Template, error) {
	files, err := fs.Glob(fsys, "templates/*.json")
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(files))
	for _, f := range files {
		raw, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var t Template
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("onboarding: template %s: %w", f, err)
		}
		if !templateKeyRE.MatchString(t.Key) || t.Version < 1 || t.Name.TR == "" || t.Name.EN == "" {
			return nil, fmt.Errorf("onboarding: template %s: key, version and name are required", f)
		}
		if err := t.Plan.Normalize(); err != nil {
			return nil, fmt.Errorf("onboarding: template %s: %w", f, err)
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b Template) int { return a.Order - b.Order })
	return out, nil
}

// Summary is the gallery card of a template.
func (t Template) Summary() TemplateSummary {
	counts := map[string]int{}
	for _, it := range t.Plan.Items() {
		counts[it.Kind]++
	}
	return TemplateSummary{
		Key: t.Key, Version: t.Version, Icon: t.Icon, Accent: t.Accent, Name: t.Name,
		Description: t.Description, Highlights: t.Highlights, Industries: t.Industries, Counts: counts,
	}
}

// closestTemplate picks the template whose industries match the answer
// (or the description) best; the first template otherwise.
func closestTemplate(templates []Template, industry, text string) Template {
	industry = strings.ToLower(industry)
	text = strings.ToLower(text)
	best, score := 0, -1
	for i, t := range templates {
		s := 0
		for _, ind := range t.Industries {
			ind = strings.ToLower(ind)
			if ind == industry {
				s += 10
			}
			if text != "" && strings.Contains(text, ind) {
				s++
			}
		}
		if s > score {
			best, score = i, s
		}
	}
	return templates[best]
}
