package billing_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/billingtest"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/google/uuid"
)

// billingtest.PublishVersion keeps up with a catalog that changes under it:
// while one goroutine adds and removes a transient feature the way the
// scenario test does (held for a while, then gone for a while), every
// PublishVersion call still ends with a published version that covers the
// catalog as it was at that moment. Without the retry this fails within a
// few iterations with "missing feature", "unknown/duplicate feature" or the
// grant insert's foreign-key conflict. It lives in this package because
// the other billing tests build versions from a fixture-time catalog and
// run sequentially with it; in its own package it would churn the catalog
// under them.
func TestPublishVersionSurvivesCatalogChanges(t *testing.T) {
	f := fixture(t)
	ctx, pool, svc, actor := f.ctx, f.pool, f.svc, f.actor

	key := "module.sim_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM billing_features WHERE key=$1", key) })
	// Saving and publishing a version takes tens of milliseconds; a catalog
	// that flips every quarter second overlaps many of the attempts without
	// starving all of them.
	const hold = 250 * time.Millisecond
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, _ = pool.Exec(ctx, "INSERT INTO billing_features(key,kind,label_tr,label_en,default_bool) VALUES($1,'boolean','Simülasyon','Simulation',true)", key)
			time.Sleep(hold)
			_, _ = pool.Exec(ctx, "DELETE FROM billing_features WHERE key=$1", key)
			time.Sleep(hold)
		}
	})
	t.Cleanup(func() { close(stop); wg.Wait() })

	in := model.VersionInput{Prices: []model.Price{{Interval: "monthly", Currency: "TRY", Amount: 10000, VAT: 2000}}}
	for i := range 12 {
		plan, err := svc.Create(ctx, model.PlanInput{Code: "sim-" + uuid.NewString(), Name: "Sim", Visibility: "hidden"})
		if err != nil {
			t.Fatal(err)
		}
		version, err := billingtest.PublishVersion(ctx, svc, plan.UUID, actor, in, billingtest.Grant(10, nil))
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if version.Status != "published" {
			t.Fatalf("iteration %d: status %s", i, version.Status)
		}
	}
}
