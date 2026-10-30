package guides

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

//go:embed seed/*.json
var seedFS embed.FS

// SeedText is the content of one locale of a seeded guide.
type SeedText struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Steps   []Step `json:"steps"`
}

// SeedGuide is one guide shipped with the code, in both locales.
type SeedGuide struct {
	Key        string   `json:"key"`
	Surface    string   `json:"surface"`
	Route      string   `json:"route"`
	Kind       string   `json:"kind"`
	Order      int      `json:"order"`
	SetupOrder int      `json:"setup_order"`
	TR         SeedText `json:"tr"`
	EN         SeedText `json:"en"`
}

// SeedFile is a seed document. Bump Version whenever the content
// changes so existing installations refresh unedited rows.
type SeedFile struct {
	Version int         `json:"version"`
	Guides  []SeedGuide `json:"guides"`
}

// LoadSeed reads and validates every embedded seed file.
func LoadSeed() ([]SeedFile, error) {
	entries, err := seedFS.ReadDir("seed")
	if err != nil {
		return nil, err
	}
	files := []SeedFile{}
	keys := map[string]bool{}
	for _, e := range entries {
		raw, err := seedFS.ReadFile("seed/" + e.Name())
		if err != nil {
			return nil, err
		}
		var f SeedFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("guides: seed %s: %w", e.Name(), err)
		}
		if f.Version < 1 {
			return nil, fmt.Errorf("guides: seed %s: version must be >= 1", e.Name())
		}
		for _, g := range f.Guides {
			if keys[g.Key] {
				return nil, fmt.Errorf("guides: seed %s: duplicate key %s", e.Name(), g.Key)
			}
			keys[g.Key] = true
			for loc, text := range map[string]SeedText{"tr": g.TR, "en": g.EN} {
				in := Input{Key: g.Key, Locale: loc, Surface: g.Surface, Route: g.Route, Kind: g.Kind, Order: g.Order,
					SetupOrder: g.SetupOrder, Title: text.Title, Summary: text.Summary, Steps: text.Steps}
				if err := in.validate(); err != nil {
					return nil, fmt.Errorf("guides: seed %s: %s/%s: %w", e.Name(), g.Key, loc, err)
				}
			}
		}
		files = append(files, f)
	}
	return files, nil
}

// Seed writes the shipped guides; rows a platform admin edited are kept.
func (s *Service) Seed(ctx context.Context) error {
	files, err := LoadSeed()
	if err != nil {
		return err
	}
	for _, f := range files {
		for _, g := range f.Guides {
			for loc, text := range map[string]SeedText{"tr": g.TR, "en": g.EN} {
				in := Input{Key: g.Key, Locale: loc, Surface: g.Surface, Route: g.Route, Kind: g.Kind, Order: g.Order,
					SetupOrder: g.SetupOrder, Title: text.Title, Summary: text.Summary, Steps: text.Steps}
				if err := in.validate(); err != nil {
					return err
				}
				steps, _ := json.Marshal(in.Steps)
				// #nosec G115 -- bounded by validate.
				if err := s.Q.GuideSeedUpsert(ctx, db.GuideSeedUpsertParams{
					Key: in.Key, Locale: loc, Surface: in.Surface, Route: in.Route, Kind: in.Kind,
					SortOrder: int32(in.Order), SetupOrder: int32(in.SetupOrder), Title: in.Title,
					Summary: in.Summary, Steps: steps, SeedVersion: int32(f.Version),
				}); err != nil {
					return fmt.Errorf("guides: seed %s/%s: %w", g.Key, loc, err)
				}
			}
		}
	}
	return nil
}
