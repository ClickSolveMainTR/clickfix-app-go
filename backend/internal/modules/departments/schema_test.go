package departments_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CF-224: every department column of the schema is a composite foreign key
// (organization_id, <column>) -> departments(organization_id, id).
//
// knowledge_chunks.department_id is still UUID; CF-222 converts it and then
// the exception goes away.
var departmentColumnWithoutKey = map[string]bool{"knowledge_chunks.department_id": true}

func schemaConn(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	return ctx, conn
}

func TestDepartmentColumnsHaveForeignKey(t *testing.T) {
	ctx, conn := schemaConn(t)
	// Department columns that no composite key to departments covers.
	rows, err := conn.Query(ctx, `
SELECT c.table_name || '.' || c.column_name
FROM information_schema.columns c
WHERE c.table_schema = 'public' AND c.column_name ~ 'department_id$'
  AND NOT EXISTS (
    SELECT 1 FROM pg_constraint k
    WHERE k.contype = 'f' AND k.confrelid = 'departments'::regclass
      AND k.conrelid = format('%I.%I', c.table_schema, c.table_name)::regclass
      AND k.conkey = ARRAY[
        (SELECT attnum FROM pg_attribute WHERE attrelid = k.conrelid AND attname = 'organization_id'),
        (SELECT attnum FROM pg_attribute WHERE attrelid = k.conrelid AND attname = c.column_name)]::int2[]
      AND k.confkey = ARRAY[
        (SELECT attnum FROM pg_attribute WHERE attrelid = 'departments'::regclass AND attname = 'organization_id'),
        (SELECT attnum FROM pg_attribute WHERE attrelid = 'departments'::regclass AND attname = 'id')]::int2[])
ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for _, col := range missing {
		if !departmentColumnWithoutKey[col] {
			bad = append(bad, col)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("department columns without a key to departments(organization_id, id): %s", strings.Join(bad, ", "))
	}
}

// Rows of the knowledge layer accept their own organization's department,
// reject another organization's, and fall back to organization-wide (NULL)
// when the department is deleted.
func TestKnowledgeLayerDepartmentKeys(t *testing.T) {
	ctx, conn := schemaConn(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	org := func() int64 {
		return scalar("INSERT INTO organizations(slug,name) VALUES($1,'Departman FK') RETURNING id", "dfk-"+uuid.NewString()[:12])
	}
	dept := func(org int64, name string) int64 {
		return scalar("INSERT INTO departments(organization_id,name) VALUES($1,$2) RETURNING id", org, name)
	}
	orgA, orgB := org(), org()
	own, foreign := dept(orgA, "Destek"), dept(orgB, "Satış")

	inserts := map[string]string{
		"history_imports": "INSERT INTO history_imports(organization_id,department_id,source,consent_analysis) VALUES($1,$2,'upload',true) RETURNING id",
		"categories":      "INSERT INTO categories(organization_id,department_id,name,slug) VALUES($1,$2,'Fatura','fatura-'||substr(md5(random()::text),1,8)) RETURNING id",
		"knowledge_gap_questions": "INSERT INTO knowledge_gap_questions(organization_id,department_id,audience,locale,scope_key,question_hash,question) " +
			"VALUES($1,$2,'customer','tr','-:-',md5(random()::text)||md5(random()::text),'Fatura nerede?') RETURNING id",
	}
	ids := map[string]int64{}
	for table, sql := range inserts {
		ids[table] = scalar(sql, orgA, own)

		if _, err := tx.Exec(ctx, "SAVEPOINT cross_org"); err != nil {
			t.Fatal(err)
		}
		var ignored int64
		err := tx.QueryRow(ctx, sql, orgA, foreign).Scan(&ignored)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Fatalf("%s: another organization's department was accepted (err %v)", table, err)
		}
		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT cross_org"); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := tx.Exec(ctx, "DELETE FROM departments WHERE organization_id=$1 AND id=$2", orgA, own); err != nil {
		t.Fatalf("delete department: %v", err)
	}
	for table, id := range ids {
		var dep *int64
		if err := tx.QueryRow(ctx, "SELECT department_id FROM "+table+" WHERE id=$1", id).Scan(&dep); err != nil {
			t.Fatal(err)
		}
		if dep != nil {
			t.Fatalf("%s: department_id = %d after the department was deleted, want NULL", table, *dep)
		}
	}
}
