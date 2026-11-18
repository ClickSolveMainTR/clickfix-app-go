package closingtexts

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInputValidation(t *testing.T) {
	for _, in := range []Input{{Name: "", BodyTr: "x"}, {Name: "x", BodyTr: " "}, {Name: "x", BodyTr: "x", Position: -1}} {
		if err := in.normalize(); !errors.Is(err, supportops.ErrInvalid) {
			t.Fatalf("%+v accepted", in)
		}
	}
	nilBrand := uuid.Nil
	in := Input{Name: " Genel ", BodyTr: " Saygılar ", BrandUUID: &nilBrand}
	if err := in.normalize(); err != nil || in.Name != "Genel" || in.BodyTr != "Saygılar" || in.BrandUUID != nil {
		t.Fatalf("normalize=%+v %v", in, err)
	}
}

func TestSaveKeepsOneDefaultPerBrand(t *testing.T) {
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
	var org int64
	if err := pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Closing') RETURNING id", "closing-svc-"+uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org) }()
	var brand uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO brands(organization_id,name) VALUES($1,'Acme') RETURNING uuid", org).Scan(&brand); err != nil {
		t.Fatal(err)
	}
	s := &Service{Pool: pool, Q: db.New(pool)}
	a, err := s.Save(ctx, org, 0, uuid.Nil, Input{Name: "A", BodyTr: "a", IsDefault: true})
	if err != nil || !a.IsDefault {
		t.Fatalf("a=%+v %v", a, err)
	}
	b, err := s.Save(ctx, org, 0, uuid.Nil, Input{Name: "B", BodyTr: "b", IsDefault: true})
	if err != nil || !b.IsDefault {
		t.Fatalf("b=%+v %v", b, err)
	}
	c, err := s.Save(ctx, org, 0, uuid.Nil, Input{Name: "C", BodyTr: "c", IsDefault: true, BrandUUID: &brand})
	if err != nil || !c.IsDefault || c.BrandUUID == nil {
		t.Fatalf("c=%+v %v", c, err)
	}
	list, err := s.List(ctx, org)
	if err != nil || len(list.Items) != 3 || list.Settings.ApplyToAI || len(list.Variables) == 0 {
		t.Fatalf("list=%+v %v", list, err)
	}
	defaults := map[string]bool{}
	for _, it := range list.Items {
		defaults[it.Name] = it.IsDefault
	}
	if defaults["A"] || !defaults["B"] || !defaults["C"] {
		t.Fatalf("defaults=%v", defaults)
	}
	other := uuid.New()
	if _, err := s.Save(ctx, org, 0, uuid.Nil, Input{Name: "X", BodyTr: "x", BrandUUID: &other}); !errors.Is(err, supportops.ErrInvalid) {
		t.Fatalf("unknown brand=%v", err)
	}
	if _, err := s.Save(ctx, org, 0, a.UUID, Input{Name: "A2", BodyTr: "a2"}); err != nil {
		t.Fatal(err)
	}
	if set, err := s.SaveSettings(ctx, org, Settings{ApplyToAI: true}); err != nil || !set.ApplyToAI {
		t.Fatalf("settings=%+v %v", set, err)
	}
	if err := s.Delete(ctx, org, b.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, org, b.UUID); err == nil {
		t.Fatal("deleted text still readable")
	}
}
