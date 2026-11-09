// Package billingtest publishes plan versions for integration tests outside
// the billing package.
//
// CI runs every package's tests at once against one shared database. The
// billing scenario tests add and remove catalog features while they run, and
// both SaveVersion and Publish validate a draft against the catalog as it is
// at that moment. A test that reads Features, saves and publishes in three
// calls therefore fails with "missing feature" or "unknown/duplicate feature"
// whenever the catalog changes in between. PublishVersion treats those two
// errors as the conflict they are: it rebuilds the grants from a fresh catalog
// and tries again.
package billingtest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/google/uuid"
)

// GrantFunc decides the grant for one catalog feature.
type GrantFunc func(model.Feature) model.Grant

// Attempts bounds the retries of PublishVersion. Each attempt re-reads the
// catalog, so a conflict needs another change in the same instant to repeat.
const Attempts = 10

// PublishVersion saves a version of plan that covers the whole feature
// catalog, publishes it and returns the published version. in supplies the
// prices and trial days; grant supplies one grant per feature.
func PublishVersion(ctx context.Context, svc *usecase.Service, plan uuid.UUID, actor int64, in model.VersionInput, grant GrantFunc) (model.Version, error) {
	draft := uuid.Nil
	var last error
	for attempt := range Attempts {
		features, err := svc.Features(ctx)
		if err != nil {
			return model.Version{}, err
		}
		in.Entitlements = in.Entitlements[:0]
		for _, feature := range features {
			in.Entitlements = append(in.Entitlements, grant(feature))
		}
		version, err := svc.SaveVersion(ctx, plan, draft, in)
		if err == nil {
			draft = version.UUID
			version, err = svc.Publish(ctx, plan, version.UUID, actor)
		}
		if err == nil {
			return version, nil
		}
		if !IsCatalogConflict(err) {
			return model.Version{}, err
		}
		last = err
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return model.Version{}, fmt.Errorf("billingtest: catalog kept changing for %d attempts: %w", Attempts, last)
}

// IsCatalogConflict reports whether err says the draft and the catalog
// disagree: a feature appeared (missing) or disappeared (unknown) between the
// catalog read and the validation, or disappeared between the validation and
// the grant insert, which the repository reports as a foreign-key conflict.
func IsCatalogConflict(err error) bool {
	if errors.Is(err, model.ErrConflict) {
		return true
	}
	return errors.Is(err, model.ErrMissingFeature) || errors.Is(err, model.ErrUnknownFeature)
}

// Grant returns a GrantFunc that enables every boolean feature and gives every
// limit feature limit, except the keys in overrides, whose limit is taken from
// the map.
func Grant(limit int64, overrides map[string]int64) GrantFunc {
	return func(feature model.Feature) model.Grant {
		g := model.Grant{Feature: feature.Key, Enforcement: "hard", WarnPercent: 80}
		if feature.Kind == "boolean" {
			enabled := true
			g.Enabled = &enabled
			return g
		}
		v := limit
		if o, ok := overrides[feature.Key]; ok {
			v = o
		}
		g.Limit = &v
		return g
	}
}
