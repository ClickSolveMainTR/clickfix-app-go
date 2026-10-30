package demoseed

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/password"
)

func TestGeneratePasswordPassesPolicy(t *testing.T) {
	for range 50 {
		p, err := GeneratePassword()
		if err != nil {
			t.Fatal(err)
		}
		if err := password.ValidatePassword(p); err != nil {
			t.Fatalf("%q: %v", p, err)
		}
	}
}

func TestFixturesAreFictional(t *testing.T) {
	for _, c := range demoCompanies {
		if !strings.Contains(c.domain, ".example.") {
			t.Errorf("company %s uses a real-looking domain %s", c.name, c.domain)
		}
	}
	if !strings.HasSuffix(AgentDomain, ".example.com") {
		t.Errorf("agent domain %s is not an example domain", AgentDomain)
	}
	for i, s := range scenarios {
		if len(s.msgs) == 0 || strings.HasPrefix(s.msgs[0], "#") {
			t.Errorf("scenario %d must start with a customer message", i)
		}
	}
}

// TestRunIsIdempotent seeds twice and expects the same demo content, the
// rotated password and untouched knowledge history.
func TestRunIsIdempotent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	type counts struct {
		conversations, messages, contacts, members, published, versions int
		ratings, views, notes, analytics, invalidNotes                  int
		appointments, appointmentTypes, appointmentMonths               int
	}
	read := func() counts {
		var c counts
		err := pool.QueryRow(ctx, `
			WITH o AS (SELECT id FROM organizations WHERE slug = $1)
			SELECT (SELECT count(*) FROM conversations WHERE organization_id = o.id),
			       (SELECT count(*) FROM messages WHERE organization_id = o.id),
			       (SELECT count(*) FROM contacts WHERE organization_id = o.id),
			       (SELECT count(*) FROM organization_members WHERE organization_id = o.id),
			       (SELECT count(*) FROM knowledge_articles WHERE organization_id = o.id AND published_version IS NOT NULL),
			       (SELECT count(*) FROM knowledge_article_versions WHERE organization_id = o.id),
			       (SELECT count(*) FROM conversation_ratings WHERE organization_id = o.id),
			       (SELECT count(*) FROM conversation_views WHERE organization_id = o.id),
			       (SELECT count(*) FROM messages WHERE organization_id = o.id AND private AND direction = 'internal' AND sender_type = 'agent'),
			       (SELECT count(*) FROM analytics_conversations WHERE organization_id = o.id),
			       (SELECT count(*) FROM messages WHERE organization_id = o.id AND (content LIKE '#%' OR (kind = 'note' AND (NOT private OR direction <> 'internal' OR sender_type <> 'agent')))),
			       (SELECT count(*) FROM appointments WHERE organization_id = o.id),
			       (SELECT count(*) FROM appointment_types WHERE organization_id = o.id),
			       (SELECT count(DISTINCT date_trunc('month', starts_at AT TIME ZONE 'Europe/Istanbul')) FROM appointments WHERE organization_id = o.id)
			FROM o`, OrgSlug).Scan(&c.conversations, &c.messages, &c.contacts, &c.members, &c.published, &c.versions, &c.ratings, &c.views, &c.notes, &c.analytics, &c.invalidNotes, &c.appointments, &c.appointmentTypes, &c.appointmentMonths)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	first, err := Run(ctx, pool, Options{Conversations: 30})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	a := read()
	hasFixtureCounts := a.conversations == 30 && a.members == len(demoUsers) && a.published == len(demoArticles)
	hasEngagement := a.views > 0 && a.ratings > 0 && a.notes > 0
	hasReports := a.analytics == a.conversations
	if !hasFixtureCounts || !hasEngagement || !hasReports || a.invalidNotes != 0 || a.appointments < 100 || a.appointmentTypes != 3 || a.appointmentMonths != 2 {
		t.Fatalf("unexpected demo content: %+v", a)
	}

	second, err := Run(ctx, pool, Options{Conversations: 30})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Created || second.OrganizationUUID != first.OrganizationUUID {
		t.Fatalf("second run must refresh the same organization: %+v", second)
	}
	if second.Password == first.Password {
		t.Fatal("a refresh must rotate the demo password")
	}
	if b := read(); b != a {
		t.Fatalf("refresh changed the demo content: before %+v after %+v", a, b)
	}

	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email = $1`, second.Accounts[0].Email).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, err := password.Verify(hash, second.Password); err != nil || !ok {
		t.Fatalf("demo owner cannot log in with the printed password (ok=%v err=%v)", ok, err)
	}
}
