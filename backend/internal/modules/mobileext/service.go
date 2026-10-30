package mobileext

import (
	"log/slog"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apns"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewService builds the mobile surface service for the API and the worker.
// An unusable APNs key is logged and leaves Live Activity push off; nothing
// else depends on it.
func NewService(cfg config.APNsConfig, pool *pgxpool.Pool, q *db.Queries, log *slog.Logger) *usecase.Service {
	if log == nil {
		log = slog.Default()
	}
	client, err := apns.New(apns.Config{KeyID: cfg.KeyID, TeamID: cfg.TeamID, PrivateKey: cfg.PrivateKey, Topic: cfg.Topic}, nil)
	if err != nil {
		log.Error("apns_config_invalid", "error", err)
		client = nil
	}
	var sender usecase.Sender
	if client != nil {
		sender = client
	}
	return usecase.New(repository.New(pool, q), sender, log)
}
