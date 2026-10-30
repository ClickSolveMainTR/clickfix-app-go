package oncall

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestRotaBoundariesAndOverrides(t *testing.T) {
	at := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	end := at.Add(4 * time.Hour)
	ls := []Layer{{StartsAt: at, EndsAt: &end, RotationMinutes: 60, Agents: []uuid.UUID{a, b}}}
	os := []Override{{StartsAt: at.Add(time.Hour), EndsAt: at.Add(2 * time.Hour), Agent: c}}
	for _, tc := range []struct {
		at   time.Time
		want uuid.UUID
	}{{at.Add(-time.Second), uuid.Nil}, {at, a}, {at.Add(time.Hour), c}, {at.Add(2 * time.Hour), a}, {end, uuid.Nil}} {
		got := Agents(ls, os, tc.at)
		if tc.want == uuid.Nil {
			if len(got) != 0 {
				t.Fatal(got)
			}
		} else if len(got) != 1 || got[0] != tc.want {
			t.Fatalf("%v: %v", tc.at, got)
		}
	}
}

func TestProbeStatusKeywordAndSSRF(t *testing.T) {
	calls := 0
	s := New(nil, nil)
	s.Client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("healthy"))}, nil
	})}
	m := db.UptimeMonitor{Url: "https://example.com/health", ExpectedStatus: 200, Keyword: "healthy"}
	if up, _, _ := s.Probe(context.Background(), m); !up {
		t.Fatal("healthy probe failed")
	}
	m.Keyword = "missing"
	if up, _, _ := s.Probe(context.Background(), m); up {
		t.Fatal("missing keyword accepted")
	}
	m.Url = "http://127.0.0.1/"
	if up, _, _ := s.Probe(context.Background(), m); up || calls != 2 {
		t.Fatal("private network request escaped guard")
	}
}

func fixture(t *testing.T) (*Service, int64, int64, int64) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	var org, other, component int64
	for _, dst := range []*int64{&org, &other} {
		if err = p.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Oncall test') RETURNING id", uuid.NewString()).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = p.Exec(ctx, "DELETE FROM outbox_events WHERE organization_id=ANY($1::bigint[])", []int64{org, other})
		_, _ = p.Exec(ctx, "DELETE FROM organizations WHERE id=ANY($1::bigint[])", []int64{org, other})
	})
	if err = p.QueryRow(ctx, "INSERT INTO status_components(organization_id,name) VALUES($1,'API') RETURNING id", org).Scan(&component); err != nil {
		t.Fatal(err)
	}
	return New(repository.New(p, db.New(p)), nil), org, other, component
}

func TestMonitorIncidentRecoveryScopeAndDedup(t *testing.T) {
	s, org, other, component := fixture(t)
	ctx := context.Background()
	var monitor int64
	if err := s.Repo.Pool.QueryRow(ctx, "INSERT INTO uptime_monitors(organization_id,name,url,interval_seconds,expected_status,keyword,component_id) VALUES($1,'API','https://example.com/health',60,200,'ok',$2) RETURNING id", org, component).Scan(&monitor); err != nil {
		t.Fatal(err)
	}
	up := false
	calls := 0
	s.Client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		status := 503
		if up {
			status = 200
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	if err := s.check(ctx, other, monitor); err == nil {
		t.Fatal("foreign monitor executed")
	}
	if calls != 0 {
		t.Fatal("network called for other org")
	}
	for range 2 {
		if _, err := s.Repo.Pool.Exec(ctx, "UPDATE uptime_monitors SET next_check_at=now() WHERE organization_id=$1 AND id=$2", org, monitor); err != nil {
			t.Fatal(err)
		}
		if err := s.check(ctx, org, monitor); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.Repo.Pool.QueryRow(ctx, "SELECT count(*) FROM status_incidents WHERE organization_id=$1", org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate incident %d %v", count, err)
	}
	up = true
	if _, err := s.Repo.Pool.Exec(ctx, "UPDATE uptime_monitors SET next_check_at=now() WHERE organization_id=$1 AND id=$2", org, monitor); err != nil {
		t.Fatal(err)
	}
	if err := s.check(ctx, org, monitor); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.Repo.Pool.QueryRow(ctx, "SELECT status FROM status_incidents WHERE organization_id=$1", org).Scan(&state); err != nil || state != "resolved" {
		t.Fatalf("recovery %s %v", state, err)
	}
}

func TestUrgentConversationEscalationAcknowledgement(t *testing.T) {
	s, org, _, _ := fixture(t)
	ctx := context.Background()
	var contact, conv, policy, user int64
	var userUUID, convUUID uuid.UUID
	if err := s.Repo.Pool.QueryRow(ctx, "INSERT INTO users(email,name,surname,password_hash,status) VALUES($1,'Agent','Test','test','active') RETURNING id,uuid", uuid.NewString()+"@example.test").Scan(&user, &userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = s.Repo.Pool.Exec(ctx, "DELETE FROM users WHERE id=$1", user) })
	if _, err := s.Repo.Pool.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='owner'", org, user); err != nil {
		t.Fatal(err)
	}
	if err := s.Repo.Pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Contact') RETURNING id", org).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if err := s.Repo.Pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,priority) VALUES($1,1,'email',$2,'urgent') RETURNING id,uuid", org, contact).Scan(&conv, &convUUID); err != nil {
		t.Fatal(err)
	}
	p, err := s.Policy(ctx, org, uuid.Nil, PolicyInput{Name: "Urgent", UrgentConversations: true, Enabled: true, Steps: []Step{{Agent: &userUUID, Channels: []string{"inapp"}}, {AfterMinutes: 1, Agent: &userUUID, Channels: []string{"push"}}}})
	if err != nil {
		t.Fatal(err)
	}
	policy = p.ID
	ev := events.New("conversation.priority_changed").WithOrganization(org).WithEntity("conversation", &conv, &convUUID)
	for range 2 {
		if err = s.Consume(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	var alert int64
	var id uuid.UUID
	if err = s.Repo.Pool.QueryRow(ctx, "SELECT id,uuid FROM oncall_alerts WHERE organization_id=$1 AND policy_id=$2", org, policy).Scan(&alert, &id); err != nil {
		t.Fatal(err)
	}
	if err = s.escalate(ctx, org, alert); err != nil {
		t.Fatal(err)
	}
	n, err := s.Repo.Q.OncallAcknowledge(ctx, db.OncallAcknowledgeParams{OrganizationID: org, Uuid: id, AcknowledgedBy: pgtype.Int8{Int64: user, Valid: true}})
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err = s.escalate(ctx, org, alert); err == nil {
		t.Fatal("acknowledged alert escalated")
	}
}
