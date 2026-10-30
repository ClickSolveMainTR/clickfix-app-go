package brands

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type limitEngine struct{ limit int64 }

func (e limitEngine) Require(context.Context, int64, string) error { return nil }
func (e limitEngine) RequireWritable(context.Context, int64) error { return nil }
func (e limitEngine) CheckLimit(_ context.Context, _ int64, key string, used, delta int64) (entitlement.Decision, error) {
	return entitlement.Check(entitlement.Value{Feature: key, Kind: "limit", Limit: e.limit, Enforcement: "hard"}, used, delta)
}
func TestBrandIsolationAndConcurrentLimit(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	var org, other int64
	for _, target := range []*int64{&org, &other} {
		if err := pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Brand test') RETURNING id", "brand-"+uuid.NewString()).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { _, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=ANY($1)", []int64{org, other}) }()
	svc := &Service{Repo: repository.New(pool, q), Entitlements: limitEngine{limit: 1}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"A", "B"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, err := svc.Save(ctx, org, uuid.Nil, Input{Name: name})
			results <- err
		}(name)
	}
	wg.Wait()
	close(results)
	successes, denied := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, entitlement.ErrDenied) {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || denied != 1 {
		t.Fatalf("race allowed %d, denied %d", successes, denied)
	}
	rows, err := svc.List(ctx, org)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if _, err := svc.Get(ctx, other, rows[0].UUID); err == nil {
		t.Fatal("foreign org read")
	}
	foreign, err := svc.Save(ctx, other, uuid.Nil, Input{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO channel_instances(organization_id,channel_type,name,brand_id) VALUES($1,'widget','Foreign',$2)", org, foreign.UUID); err == nil {
		t.Fatal("cross-org brand FK accepted")
	}
	var channel, contact int64
	if err := pool.QueryRow(ctx, "INSERT INTO channel_instances(organization_id,channel_type,name,brand_id) VALUES($1,'widget','Branded',$2) RETURNING id", org, rows[0].UUID).Scan(&channel); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Visitor') RETURNING id", org).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	var brand uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,channel_instance_id,contact_id) VALUES($1,1,'widget',$2,$3) RETURNING brand_id", org, channel, contact).Scan(&brand); err != nil || brand != rows[0].UUID {
		t.Fatalf("inherit brand=%v err=%v", brand, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE channel_instances SET brand_id=NULL WHERE id=$1", channel); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT brand_id FROM conversations WHERE organization_id=$1", org).Scan(&brand); err != nil || brand != rows[0].UUID {
		t.Fatalf("brand changed with channel: %v %v", brand, err)
	}
}
func TestBrandInputValidation(t *testing.T) {
	for _, in := range []Input{{Name: "A", SupportEmail: "x@example.com\r\nBcc: attacker@example.com"}, {Name: "A", PrimaryColor: "red"}, {Name: "A", HelpSubdomain: "https://other.example.com"}, {Name: "A", HelpPath: "../secret"}} {
		if in.validate() == nil {
			t.Fatalf("accepted %#v", in)
		}
	}
}

func TestHelpHostNormalization(t *testing.T) {
	if normalizeHelpHost(" Help.Brand.test:443, proxy.internal ") != "help.brand.test" {
		t.Fatal("host normalization")
	}
	for _, host := range []string{"brand", "help.brand.test"} {
		if !validHelpHost(host) {
			t.Fatal(host)
		}
	}
	for _, host := range []string{"https://brand.test", "bad..test", "-bad.test", "brand.test/path"} {
		if validHelpHost(host) {
			t.Fatal(host)
		}
	}
}
