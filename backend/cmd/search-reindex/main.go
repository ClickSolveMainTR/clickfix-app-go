// Command search-reindex rebuilds the Meilisearch indexes from Postgres.
//
//	search-reindex                    # every spec (users, roles, conversations, messages, contacts, companies)
//	search-reindex -spec messages     # one spec
//	search-reindex -fresh             # empty each index first (drops documents of deleted records)
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/logging"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine/adapters"
)

func main() {
	spec := flag.String("spec", "", "reindex one spec only (default: all)")
	fresh := flag.Bool("fresh", false, "delete every document of the index before reindexing")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	if !cfg.Search.Enabled {
		log.Info("search_disabled")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	queries := database.NewQueries(pool)
	client := searchengine.NewClient(cfg.Search, log)
	if client == nil || !client.Enabled() {
		log.Error("search_client_unavailable")
		os.Exit(1)
	}
	reg := searchengine.NewRegistry(searchadapters.All(queries)...)
	indexer := searchengine.NewIndexer(client, reg, nil, log)
	counts, err := indexer.Rebuild(ctx, *spec, *fresh)
	if err != nil {
		log.Error("search_reindex_failed", "spec", *spec, "counts", counts, "error", err)
		os.Exit(1)
	}
	log.Info("search_reindex_completed", "counts", counts, "fresh", *fresh)
}
