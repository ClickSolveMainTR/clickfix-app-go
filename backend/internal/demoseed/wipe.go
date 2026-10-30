package demoseed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// keptTables survive a refresh: members, roles, the subscription and billing
// history, SSO / API credentials and the event log. Knowledge articles keep
// their immutable version history too (knowledge_* except suggestions); the
// seeder updates them in place by slug.
var keptTables = map[string]bool{
	"organization_members": true, "organization_member_roles": true, "organization_onboarding": true,
	"organization_sso": true, "roles": true, "sso_identities": true, "sso_login_states": true, "scim_groups": true,
	"mcp_oauth_grants": true, "public_api_keys": true, "support_access_grants": true,
	"outbox_events": true, "domain_event_log": true, "event_dead_letters": true,
	"ai_credit_reservation_lots": true,
}

// wipe deletes the demo organization's content from every organization
// scoped table except keptTables. Tables are discovered from the schema so
// rows written by later features (or by E2E runs) do not block a refresh;
// foreign keys between them are resolved by retrying in rounds.
func wipe(ctx context.Context, tx pgx.Tx, orgID int64) error {
	rows, err := tx.Query(ctx, `
		SELECT c.table_name FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = 'public' AND c.column_name = 'organization_id' AND t.table_type = 'BASE TABLE'
		  AND c.table_name <> 'organizations'
		ORDER BY c.table_name`)
	if err != nil {
		return err
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	pending := make([]string, 0, len(tables))
	for _, t := range tables {
		knowledge := strings.HasPrefix(t, "knowledge_") && t != "knowledge_suggestions"
		if !keptTables[t] && !knowledge && !strings.HasPrefix(t, "billing_") {
			pending = append(pending, t)
		}
	}
	for round := 0; len(pending) > 0; round++ {
		var blocked []string
		var lastErr error
		for _, t := range pending {
			sp, err := tx.Begin(ctx)
			if err != nil {
				return err
			}
			_, err = sp.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE organization_id = $1`, pgx.Identifier{t}.Sanitize()), orgID)
			if err == nil {
				if err := sp.Commit(ctx); err != nil {
					return err
				}
				continue
			}
			_ = sp.Rollback(ctx)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
				return fmt.Errorf("%s: %w", t, err)
			}
			blocked, lastErr = append(blocked, t), fmt.Errorf("%s: %w", t, err)
		}
		if len(blocked) == len(pending) {
			return fmt.Errorf("cannot clear demo data: %w", lastErr)
		}
		pending = blocked
	}
	return nil
}
