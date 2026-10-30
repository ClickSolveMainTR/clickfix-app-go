package billing_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestSchemaInvariants exercises the migrated PostgreSQL schema, not an in-memory model.
// Each subtest rolls back its writes so it can run against the worker's isolated DB.
func TestSchemaInvariants(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for billing schema integration tests")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	for _, tc := range []struct {
		name string
		sql  string
		code string
	}{
		{"draft price editable", "UPDATE billing_prices SET amount_minor = 12000 WHERE id = $1", ""},
		{"draft entitlement editable", "UPDATE billing_plan_entitlements SET value_limit = 10 WHERE plan_version_id = $2", ""},
		{"draft tier editable", "UPDATE billing_price_tiers SET unit_amount_minor = 20 WHERE price_id = $1", ""},
		{"published version immutable", "UPDATE billing_plan_versions SET trial_days = 9 WHERE id = $2", "23514"},
		{"published version cannot delete", "DELETE FROM billing_plan_versions WHERE id = $2", "23514"},
		{"published price immutable", "UPDATE billing_prices SET amount_minor = 12000 WHERE id = $1", "23514"},
		{"published price cannot delete", "DELETE FROM billing_prices WHERE id = $1", "23514"},
		{"published entitlement immutable", "UPDATE billing_plan_entitlements SET value_limit = 10 WHERE plan_version_id = $2", "23514"},
		{"published entitlement cannot delete", "DELETE FROM billing_plan_entitlements WHERE plan_version_id = $2", "23514"},
		{"published tier immutable", "UPDATE billing_price_tiers SET unit_amount_minor = 20 WHERE price_id = $1", "23514"},
		{"published tier cannot delete", "DELETE FROM billing_price_tiers WHERE price_id = $1", "23514"},
		{"published price cannot insert", "INSERT INTO billing_prices(plan_version_id,interval,currency,amount_minor,vat_basis_points) VALUES($2,'yearly','TRY',10000,2000)", "23514"},
		{"published tier cannot insert", "INSERT INTO billing_price_tiers(price_id,feature_id,lower_bound,unit_amount_minor) SELECT $1,id,100,1 FROM billing_features WHERE key='seats'", "23514"},
		{"published entitlement cannot insert", "INSERT INTO billing_plan_entitlements(plan_version_id,feature_id,kind,value_bool) SELECT $2,id,'boolean',true FROM billing_features WHERE key='ai.enabled'", "23514"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			plan := rowID(t, tx, "INSERT INTO billing_plans(code,name) VALUES($1,'Test') RETURNING id", "test-"+uuid.NewString())
			version := rowID(t, tx, "INSERT INTO billing_plan_versions(plan_id,version) VALUES($1,1) RETURNING id", plan)
			price := rowID(t, tx, "INSERT INTO billing_prices(plan_version_id,interval,currency,amount_minor,vat_basis_points) VALUES($1,'monthly','TRY',10000,2000) RETURNING id", version)
			mustExec(t, tx, "INSERT INTO billing_plan_entitlements(plan_version_id,feature_id,kind,value_limit) SELECT $1,id,'limit',5 FROM billing_features WHERE key='seats'", version)
			mustExec(t, tx, "INSERT INTO billing_price_tiers(price_id,feature_id,lower_bound,unit_amount_minor) SELECT $1,id,0,10 FROM billing_features WHERE key='seats'", price)
			if tc.code != "" {
				mustExec(t, tx, "UPDATE billing_plan_versions SET status='published',published_at=now() WHERE id=$1", version)
			}
			// Both placeholders are supplied via a CTE so SQL cases may use either.
			_, err = tx.Exec(ctx, "WITH params AS (SELECT $1::bigint AS price_id, $2::bigint AS version_id) "+tc.sql, price, version)
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Fatalf("expected SQLSTATE %s; got %v", tc.code, err)
			}
		})
	}
}

func TestGrandfatheringAndTenantBoundaries(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	org := rowID(t, tx, "INSERT INTO organizations(slug,name) VALUES($1,'Test') RETURNING id", uuid.NewString())
	otherOrg := rowID(t, tx, "INSERT INTO organizations(slug,name) VALUES($1,'Other') RETURNING id", uuid.NewString())
	plan := rowID(t, tx, "INSERT INTO billing_plans(code,name) VALUES($1,'Test') RETURNING id", "test-"+uuid.NewString())
	version := rowID(t, tx, "INSERT INTO billing_plan_versions(plan_id,version) VALUES($1,1) RETURNING id", plan)
	price := rowID(t, tx, "INSERT INTO billing_prices(plan_version_id,interval,currency,amount_minor,vat_basis_points) VALUES($1,'monthly','TRY',10000,2000) RETURNING id", version)
	mustExec(t, tx, "UPDATE billing_plan_versions SET status='published',published_at=now() WHERE id=$1", version)
	sub := rowID(t, tx, `INSERT INTO billing_subscriptions(organization_id,plan_version_id,price_id,status,period_start,period_end)
		VALUES($1,$2,$3,'active',now(),now()+interval '1 month') RETURNING id`, org, version, price)
	newVersion := rowID(t, tx, "INSERT INTO billing_plan_versions(plan_id,version) VALUES($1,2) RETURNING id", plan)
	rowID(t, tx, "INSERT INTO billing_prices(plan_version_id,interval,currency,amount_minor,vat_basis_points) VALUES($1,'monthly','TRY',20000,2000) RETURNING id", newVersion)
	mustExec(t, tx, "UPDATE billing_plan_versions SET status='published',published_at=now() WHERE id=$1", newVersion)
	var amount int64
	if err := tx.QueryRow(ctx, `SELECT p.amount_minor FROM billing_subscriptions s JOIN billing_prices p ON p.id=s.price_id
		WHERE s.organization_id=$1 AND s.id=$2`, org, sub).Scan(&amount); err != nil || amount != 10000 {
		t.Fatalf("existing subscription price changed: amount=%d error=%v", amount, err)
	}
	ledger := rowID(t, tx, `INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key)
        VALUES($1,$2,'ai_credits',100,'Test grant','grant-1') RETURNING id`, org, sub)
	usagePeriod := rowID(t, tx, `INSERT INTO billing_usage_periods(organization_id,starts_at,ends_at)
        VALUES($1,date_trunc('month',now()),date_trunc('month',now())+interval '1 month') RETURNING id`, org)
	usage := rowID(t, tx, `INSERT INTO billing_usage_records(organization_id,usage_period_id,feature_id,idempotency_key,quantity,occurred_at)
        SELECT $1,$2,id,'event-1',10,now() FROM billing_features WHERE key='ai.tokens' RETURNING id`, org, usagePeriod)
	for _, tc := range []struct {
		name string
		sql  string
		args []any
		code string
	}{
		{"ledger update forbidden", "UPDATE billing_credit_ledger SET delta=500 WHERE id=$1", []any{ledger}, "23514"},
		{"ledger delete forbidden", "DELETE FROM billing_credit_ledger WHERE id=$1", []any{ledger}, "23514"},
		{"usage update forbidden", "UPDATE billing_usage_records SET quantity=500 WHERE id=$1", []any{usage}, "23514"},
		{"usage delete forbidden", "DELETE FROM billing_usage_records WHERE id=$1", []any{usage}, "23514"},
		{"usage event idempotency", `INSERT INTO billing_usage_records(organization_id,usage_period_id,feature_id,idempotency_key,quantity,occurred_at)
            SELECT $1,$2,id,'event-1',10,now() FROM billing_features WHERE key='ai.tokens'`, []any{org, usagePeriod}, "23505"},
		{"usage foreign organization", `INSERT INTO billing_usage_records(organization_id,usage_period_id,feature_id,idempotency_key,quantity,occurred_at)
            SELECT $1,$2,id,'event-2',10,now() FROM billing_features WHERE key='ai.tokens'`, []any{otherOrg, usagePeriod}, "23503"},
		{"credit idempotency", `INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key)
            VALUES($1,$2,'ai_credits',100,'Duplicate grant','grant-1')`, []any{org, sub}, "23505"},
		{"second live subscription", `INSERT INTO billing_subscriptions(organization_id,plan_version_id,price_id,status,period_start,period_end)
			VALUES($1,$2,$3,'trialing',now(),now()+interval '1 month')`, []any{org, version, price}, "23505"},
		{"price version mismatch", "UPDATE billing_subscriptions SET plan_version_id=$1 WHERE id=$2", []any{newVersion, sub}, "23503"},
		{"cross organization item", `INSERT INTO billing_subscription_items(organization_id,subscription_id,addon_id,quantity,unit_amount_minor,currency)
			SELECT $1,$2,id,1,100,'TRY' FROM billing_addons WHERE code=$3`, []any{otherOrg, sub, "test-addon"}, "23503"},
		{"typed entitlement", `INSERT INTO billing_plan_entitlements(plan_version_id,feature_id,kind,value_bool)
			SELECT $1,id,'boolean',true FROM billing_features WHERE key='seats'`, []any{rowID(t, tx, "INSERT INTO billing_plan_versions(plan_id,version) VALUES($1,3) RETURNING id", plan)}, "23503"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustExec(t, tx, "SAVEPOINT invariant")
			if tc.name == "cross organization item" {
				mustExec(t, tx, "INSERT INTO billing_addons(code,name,feature_id,units,amount_minor,currency) SELECT 'test-addon','Test',id,1,100,'TRY' FROM billing_features WHERE key='seats'")
			}
			_, err := tx.Exec(ctx, tc.sql, tc.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Errorf("expected SQLSTATE %s; got %v", tc.code, err)
			}
			mustExec(t, tx, "ROLLBACK TO SAVEPOINT invariant")
		})
	}
}

func rowID(t *testing.T, tx pgx.Tx, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func mustExec(t *testing.T, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
