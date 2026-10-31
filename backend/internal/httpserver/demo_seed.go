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

func (d demoSeeder) Start(org uuid.UUID) (string, []orghandler.DemoAccount, error) {
	res, err := d.p.Start(org)
	if err != nil {
		return "", nil, err
	}
	accounts := make([]orghandler.DemoAccount, 0, len(res.Accounts))
	for _, a := range res.Accounts {
		accounts = append(accounts, orghandler.DemoAccount(a))
	}
	return res.Password, accounts, nil
}
