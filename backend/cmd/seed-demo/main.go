// Command seed-demo creates or refreshes the demo organization
// "Demo · Anadolu Yazılım" with fictional sample data (CF-134). A refresh
// replaces the demo content and rotates the demo password; the generated
// password is printed once and never stored.
//
// Usage (repo root):
//
//	make seed-demo
//
// In the production backend container (see docs/demo-org.md):
//
//	/app/seed-demo
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/demoseed"
)

func main() {
	conversations := flag.Int("conversations", demoseed.DefaultConversations, "number of sample conversations")
	asJSON := flag.Bool("json", false, "print the result as JSON (for scripts)")
	devCatalog := flag.Bool("dev-catalog", false, "publish a demo trial plan when the plan catalog is empty (local / CI only)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	if *devCatalog && cfg.App.Env == "production" {
		fatal("-dev-catalog is for local and CI databases; create real plans from platform billing in production")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	res, err := demoseed.Run(ctx, pool, demoseed.Options{
		// DEMO_PASSWORD pins the password (E2E runs); otherwise a random one is generated.
		Password:      os.Getenv("DEMO_PASSWORD"),
		Conversations: *conversations,
		DevCatalog:    *devCatalog,
		Log:           log,
	})
	if err != nil {
		fatal("%v", err)
	}

	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(res)
		return
	}
	action := "refreshed"
	if res.Created {
		action = "created"
	}
	fmt.Printf("demo organization %s: %s (%s)\n", action, demoseed.OrgName, res.OrganizationSlug)
	fmt.Printf("  conversations: %d, help center articles: %d\n", res.Conversations, res.Articles)
	fmt.Printf("  widget public id: %s\n", res.WidgetPublicID)
	fmt.Printf("  help center: /help/%s\n", res.OrganizationSlug)
	fmt.Println("  logins (shown once, not stored; run again to rotate):")
	for _, a := range res.Accounts {
		fmt.Printf("    %-11s %-45s %s\n", a.Role, a.Email, a.Name)
	}
	fmt.Printf("  password: %s\n", res.Password)
	fmt.Println("  search index: run search-reindex to make the demo data searchable")
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "seed-demo: "+format+"\n", args...)
	os.Exit(1)
}
