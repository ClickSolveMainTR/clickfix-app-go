package httpserver

import (
	"context"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/demoseed"
	orghandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/handler"
)

// demoSeeder adapts demoseed.Platform to the organizations handler.
type demoSeeder struct{ p *demoseed.Platform }

func (d demoSeeder) TrialPrice(ctx context.Context, actor int64) (uuid.UUID, error) {
	return d.p.TrialPrice(ctx, actor)
}

func (d demoSeeder) NewPassword() (string, error) { return d.p.NewPassword() }

func (d demoSeeder) Accounts(slug string) []orghandler.DemoAccount {
	src := demoseed.Accounts(slug)
	out := make([]orghandler.DemoAccount, 0, len(src))
	for _, a := range src {
		out = append(out, orghandler.DemoAccount(a))
	}
	return out
}

func (d demoSeeder) Start(org uuid.UUID, slug, password string, actor int64) {
	d.p.Start(org, slug, password, actor)
}
