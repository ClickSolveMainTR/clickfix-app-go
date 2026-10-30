package usecase_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/hibiken/asynq"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests follow the repository's TEST_DATABASE_URL convention so CI
// always exercises them against migrated Postgres; no external service is called.
type store struct {
	storage.Driver
	Memory *storage.Memory
	fail   bool
}

func (s *store) ListVersions(context.Context, string) ([]storage.VersionInfo, error) { return nil, nil }
func (s *store) Delete(ctx context.Context, key string) error {
	if s.fail {
		return errors.New("storage unavailable")
	}
	return s.Memory.Delete(ctx, key)
}

type fixture struct {
	pool                           *pgxpool.Pool
	svc                            *usecase.Service
	store                          *store
	org, other, contact, conv, msg int64
	contactUUID, convUUID          uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	f := fixture{pool: pool, store: &store{Memory: storage.NewMemory()}}
	f.svc = &usecase.Service{Repo: repository.New(pool, db.New(pool)), Store: f.store}
	f.org = f.id(t, "INSERT INTO organizations(slug,name) VALUES($1,'Compliance') RETURNING id", "privacy-"+uuid.NewString()[:12])
	f.other = f.id(t, "INSERT INTO organizations(slug,name) VALUES($1,'Other') RETURNING id", "privacy-"+uuid.NewString()[:12])
	f.contact = f.id(t, "INSERT INTO contacts(organization_id,name,email,notes) VALUES($1,'Özel kişi','private@example.test','private note') RETURNING id", f.org)
	f.conv = f.id(t, "INSERT INTO conversations(organization_id,contact_id,number,channel_type,subject,status,closed_at) VALUES($1,$2,1,'test','Private subject','closed',now()-interval '60 days') RETURNING id", f.org, f.contact)
	f.msg = f.id(t, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','Private transcript') RETURNING id", f.org, f.conv)
	if e = pool.QueryRow(ctx, "SELECT uuid FROM contacts WHERE organization_id=$1 AND id=$2", f.org, f.contact).Scan(&f.contactUUID); e != nil {
		t.Fatal(e)
	}
	if e = pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id=$1 AND id=$2", f.org, f.conv).Scan(&f.convUUID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, query := range []string{"DELETE FROM data_subject_requests WHERE organization_id=$1", "DELETE FROM contact_consents WHERE organization_id=$1", "DELETE FROM ai_runs WHERE organization_id=$1", "DELETE FROM organizations WHERE id=$1"} {
			_, _ = pool.Exec(ctx, query, f.org)
		}
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", f.other)
	})
	return f
}
func (f fixture) id(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var id int64
	if e := f.pool.QueryRow(context.Background(), query, args...).Scan(&id); e != nil {
		t.Fatal(e)
	}
	return id
}
func (f fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, e := f.pool.Exec(context.Background(), query, args...); e != nil {
		t.Fatal(e)
	}
}
func (f fixture) request(t *testing.T, kind string) usecase.Request {
	t.Helper()
	v, e := f.svc.Create(context.Background(), f.org, 0, f.contactUUID, kind)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestSubjectExportAndErasureIsolation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	key := "orgs/test/private"
	if e := f.store.Upload(ctx, storage.File{Body: strings.NewReader("private bytes")}, key); e != nil {
		t.Fatal(e)
	}
	f.id(t, "INSERT INTO attachments(organization_id,owner_type,owner_id,filename,content_type,size_bytes,sha256,object_key) VALUES($1,'conversation',$2,'../../private.txt','text/plain',13,'hash',$3) RETURNING id", f.org, f.convUUID, key)
	// Cross-org UUIDs cannot be used to create or inspect a subject request.
	if _, e := f.svc.Create(ctx, f.other, 0, f.contactUUID, "export"); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("cross-org request: %v", e)
	}
	req := f.request(t, "export")
	if e := f.svc.Process(ctx, f.other, req.UUID); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("cross-org process: %v", e)
	}
	if e := f.svc.Process(ctx, f.org, req.UUID); e != nil {
		t.Fatal(e)
	}
	row, e := f.svc.Repo.Q.ComplianceGetRequest(ctx, db.ComplianceGetRequestParams{OrganizationID: f.org, Uuid: req.UUID})
	if e != nil || row.Status != "completed" || row.ObjectKey == "" {
		t.Fatalf("export: %+v %v", row, e)
	}
	body, _, e := f.store.Download(ctx, row.ObjectKey)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(body)
	_ = body.Close()
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, file := range z.File {
		if strings.Contains(file.Name, "..") || strings.HasPrefix(file.Name, "/") {
			t.Fatalf("unsafe archive name %s", file.Name)
		}
		if strings.HasPrefix(file.Name, "attachments/") {
			r, e := file.Open()
			if e != nil {
				t.Fatal(e)
			}
			b, _ := io.ReadAll(r)
			_ = r.Close()
			found = string(b) == "private bytes"
		}
	}
	if !found {
		t.Fatal("attachment bytes missing from ZIP")
	}
	erasure := f.request(t, "erase")
	f.store.fail = true
	if e = f.svc.Process(ctx, f.org, erasure.UUID); e == nil {
		t.Fatal("storage failure must fail erasure")
	}
	var name string
	_ = f.pool.QueryRow(ctx, "SELECT name FROM contacts WHERE organization_id=$1 AND id=$2", f.org, f.contact).Scan(&name)
	if name != "Özel kişi" {
		t.Fatal("failed erasure changed contact")
	}
	f.store.fail = false
	if e = f.svc.Process(ctx, f.org, erasure.UUID); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.Process(ctx, f.org, erasure.UUID); e != nil {
		t.Fatal("replay", e)
	}
	var content string
	var erased bool
	if e = f.pool.QueryRow(ctx, "SELECT c.erased_at IS NOT NULL,m.content FROM contacts c JOIN messages m ON m.organization_id=c.organization_id WHERE c.organization_id=$1 AND c.id=$2 AND m.id=$3", f.org, f.contact, f.msg).Scan(&erased, &content); e != nil {
		t.Fatal(e)
	}
	if !erased || content != "" {
		t.Fatal("subject data remains")
	}
	exists, _ := f.store.Exists(ctx, key)
	if exists {
		t.Fatal("attachment bytes remain")
	}
	f.exec(t, "UPDATE messages SET content='late provider update' WHERE organization_id=$1 AND id=$2", f.org, f.msg)
	_ = f.pool.QueryRow(ctx, "SELECT content FROM messages WHERE organization_id=$1 AND id=$2", f.org, f.msg).Scan(&content)
	if content != "" {
		t.Fatal("late write resurrected content")
	}
	f.exec(t, "UPDATE contacts SET name='resurrect',email='resurrect@example.test' WHERE organization_id=$1 AND id=$2", f.org, f.contact)
	var emailNull bool
	_ = f.pool.QueryRow(ctx, "SELECT email IS NULL FROM contacts WHERE organization_id=$1 AND id=$2", f.org, f.contact).Scan(&emailNull)
	if !emailNull {
		t.Fatal("stale contact update restored personal data")
	}
	if _, e = f.pool.Exec(ctx, "INSERT INTO conversations(organization_id,contact_id,number,channel_type) VALUES($1,$2,99,'test')", f.org, f.contact); e == nil {
		t.Fatal("new conversation accepted erased subject")
	}
	if _, e = f.pool.Exec(ctx, "INSERT INTO attachments(organization_id,owner_type,owner_id,filename,content_type,size_bytes,sha256,object_key) VALUES($1,'conversation',$2,'late.txt','text/plain',1,'hash','late-key')", f.org, f.convUUID); e == nil {
		t.Fatal("late attachment accepted disposed owner")
	}
	exists, _ = f.store.Exists(ctx, row.ObjectKey)
	if exists {
		t.Fatal("previous subject export survived erasure")
	}

}
func TestRetentionOnlyDisposesDueClosedConversations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	open := f.id(t, "INSERT INTO conversations(organization_id,contact_id,number,channel_type,status) VALUES($1,$2,2,'test','open') RETURNING id", f.org, f.contact)
	f.id(t, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','Keep open') RETURNING id", f.org, open)
	run := f.id(t, "INSERT INTO ai_runs(organization_id,conversation_id,task,provider,model,status,input,output) VALUES($1,$2,'reply','fake','fake','running','Private AI input','') RETURNING id", f.org, f.conv)
	if _, e := f.svc.SavePolicy(ctx, f.org, usecase.Policy{ConversationDays: 30, Action: "anonymize"}); e != nil {
		t.Fatal(e)
	}
	if e := f.svc.Sweep(ctx, nil); e != nil {
		t.Fatal(e)
	}
	var got string
	_ = f.pool.QueryRow(ctx, "SELECT content FROM messages WHERE organization_id=$1 AND conversation_id=$2", f.org, open).Scan(&got)
	if got != "Keep open" {
		t.Fatalf("open conversation disposed: %s", got)
	}
	f.exec(t, "UPDATE ai_runs SET output='late AI result',status='completed' WHERE organization_id=$1 AND id=$2", f.org, run)
	_ = f.pool.QueryRow(ctx, "SELECT input||output FROM ai_runs WHERE organization_id=$1 AND id=$2", f.org, run).Scan(&got)
	if got != "" {
		t.Fatal("late AI result resurrected data")
	}
	if e := f.svc.Sweep(ctx, nil); e != nil {
		t.Fatal("repeat sweep", e)
	}
}

func (s *store) Upload(ctx context.Context, f storage.File, key string) error {
	return s.Memory.Upload(ctx, f, key)
}
func (s *store) Download(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	return s.Memory.Download(ctx, key)
}
func (s *store) Exists(ctx context.Context, key string) (bool, error) {
	return s.Memory.Exists(ctx, key)
}

func TestMetaErasureUsesComplianceAndIsIdempotent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	subject := "159753"
	// Both an OAuth connecting owner and page/IG-scoped contact identities are supported.
	instance := f.id(t, "INSERT INTO channel_instances(organization_id,channel_type,name,external_id,credentials_enc,settings) VALUES($1,'messenger','Page',gen_random_uuid()::text,'sealed-token',jsonb_build_object('meta_user_id',$2::text)) RETURNING id", f.org, subject)
	f.id(t, "INSERT INTO contact_identities(organization_id,contact_id,channel_type,channel_instance_id,external_id) VALUES($1,$2,'messenger',$3,$4) RETURNING id", f.org, f.contact, instance, subject)
	// An Instagram contact with the same numeric id belongs to a different app and must stay intact.
	f.id(t, "INSERT INTO contact_identities(organization_id,contact_id,channel_type,external_id) VALUES($1,$2,'instagram',$3) RETURNING id", f.other, f.id(t, "INSERT INTO contacts(organization_id,name) VALUES($1,'Other app') RETURNING id", f.other), subject)
	// A numeric WhatsApp phone identity must not be confused with an app-scoped id.
	f.id(t, "INSERT INTO contact_identities(organization_id,contact_id,channel_type,external_id) SELECT organization_id,id,'whatsapp',$2 FROM contacts WHERE organization_id=$1 RETURNING id", f.other, subject)
	code, e := f.svc.MetaCallback(ctx, "facebook", subject, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(code) != 32 {
		t.Fatal("invalid code")
	}
	var status, token string
	if e = f.pool.QueryRow(ctx, "SELECT status,credentials_enc FROM channel_instances WHERE organization_id=$1 AND id=$2", f.org, instance).Scan(&status, &token); e != nil {
		t.Fatal(e)
	}
	if status != "disconnected" || token != "" {
		t.Fatal("provider credentials retained")
	}
	if e = f.svc.ProcessMetaDeletion(ctx, code); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.ProcessMetaDeletion(ctx, code); e != nil {
		t.Fatal(e)
	}
	receipt, e := f.svc.Repo.Q.MetaDeletionStatus(ctx, code)
	if e != nil || receipt.Status != "completed" {
		t.Fatalf("receipt=%+v %v", receipt, e)
	}
	c, e := f.svc.Repo.Q.ComplianceContact(ctx, db.ComplianceContactParams{OrganizationID: f.org, Uuid: f.contactUUID})
	if e != nil {
		t.Fatal(e)
	}
	if c.Email.String != "" || c.Notes != "" {
		t.Fatal("contact not erased")
	}
	var name string
	if e = f.pool.QueryRow(ctx, "SELECT name FROM contacts WHERE organization_id=$1", f.other).Scan(&name); e != nil || name != "Other app" {
		t.Fatal("cross-app erasure")
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(ctx, "DELETE FROM meta_deletion_receipts WHERE code=$1", code) })
}
func TestMetaUnknownAndDeauthorize(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// Unknown subjects receive a real, independently queryable confirmation too.
	code, e := f.svc.MetaCallback(ctx, "instagram", "987654321", true)
	if e != nil || len(code) != 32 {
		t.Fatalf("unknown: %s %v", code, e)
	}
	if e = f.svc.ProcessMetaDeletion(ctx, code); e != nil {
		t.Fatal(e)
	}
	r, e := f.svc.Repo.Q.MetaDeletionStatus(ctx, code)
	if e != nil || r.Status != "completed" {
		t.Fatal("unknown receipt invalid")
	}
	instance := f.id(t, "INSERT INTO channel_instances(organization_id,channel_type,name,external_id,credentials_enc) VALUES($1,'instagram','Instagram','987654321','secret') RETURNING id", f.org)
	for range 2 {
		if _, e = f.svc.MetaCallback(ctx, "instagram", "987654321", false); e != nil {
			t.Fatal(e)
		}
	}
	var credentials, status string
	if e = f.pool.QueryRow(ctx, "SELECT credentials_enc,status FROM channel_instances WHERE organization_id=$1 AND id=$2", f.org, instance).Scan(&credentials, &status); e != nil || credentials != "" || status != "disconnected" {
		t.Fatal("deauthorization failed")
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(ctx, "DELETE FROM meta_deletion_receipts WHERE code=$1", code) })
}

type unavailableMetaQueue struct{ called bool }

func (q *unavailableMetaQueue) EnqueueContext(_ context.Context, t *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	q.called = t.Type() == usecase.TaskMetaDeletion
	for _, o := range opts {
		if o.Type() == asynq.QueueOpt && o.Value() != "maintenance" {
			return nil, fmt.Errorf("unexpected queue %v", o.Value())
		}
	}
	return nil, errors.New("redis unavailable")
}
func TestMetaCallbackSurvivesRedisOutage(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	queue := &unavailableMetaQueue{}
	f.svc.Jobs = queue
	code, err := f.svc.MetaCallback(ctx, "facebook", "987654321987654", true)
	if err != nil || !queue.called {
		t.Fatalf("callback lost: %v", err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(ctx, "DELETE FROM meta_deletion_receipts WHERE code=$1", code) })
	// Recovery uses the same compliance sweep in both worker modes.
	if err = f.svc.Sweep(ctx, nil); err != nil {
		t.Fatal(err)
	}
	status, err := f.svc.Repo.Q.MetaDeletionStatus(ctx, code)
	if err != nil || status.Status != "completed" {
		t.Fatalf("recovery: %+v %v", status, err)
	}
}

func TestListRequestsIsOrganizationScoped(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req := f.request(t, "export")
	if e := f.svc.Process(ctx, f.org, req.UUID); e != nil {
		t.Fatal(e)
	}
	f.request(t, "erase")
	items, total, e := f.svc.ListRequests(ctx, f.org, 10, 0)
	if e != nil || total != 2 || len(items) != 2 {
		t.Fatalf("list: %d %d %v", total, len(items), e)
	}
	// Newest first; the export carries the API-owned download URL and its subject.
	if items[0].Kind != "erase" || items[1].UUID != req.UUID || items[1].DownloadURL == "" || items[1].CompletedAt == nil {
		t.Fatalf("items: %+v", items)
	}
	if items[1].Contact.UUID != f.contactUUID || items[1].Contact.Email != "private@example.test" || items[1].RequestedBy != nil {
		t.Fatalf("subject: %+v", items[1])
	}
	if items, total, e = f.svc.ListRequests(ctx, f.other, 10, 0); e != nil || total != 0 || len(items) != 0 {
		t.Fatalf("cross-org list: %d %v", total, e)
	}
}
