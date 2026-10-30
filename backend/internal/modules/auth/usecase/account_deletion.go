package usecase

import (
	"context"
	"github.com/hibiken/asynq"
)

const TaskAccountPurge = "app:auth:account-purge"

type AccountPurger interface{ PurgeDueAccounts(context.Context) error }

func AccountPurgeHandler(p AccountPurger) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error { return p.PurgeDueAccounts(ctx) }
}
