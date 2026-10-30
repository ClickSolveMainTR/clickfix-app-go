// Package demoseed creates or refreshes the demo organization "Demo · Anadolu
// Yazılım" with fictional Turkish sample data (CF-134). Rows are written
// through the existing sqlc queries and usecases; raw SQL is used only to
// wipe the previous demo content and to backdate conversations.
package demoseed

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	mrand "math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	analyticsrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/repository"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/usecase"
	knowledgemodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	knowledgerepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	knowledgeusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/password"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
)

const (
	// OrgSlug identifies the demo organization; only this organization is
	// ever wiped by a refresh.
	OrgSlug = "demo-anadolu-yazilim"
	// OrgName is the display name of the demo organization.
	OrgName = "Demo · Anadolu Yazılım"
	// DefaultConversations is how many conversations a seed creates.
	DefaultConversations = 120
)

// Options tune a seed run.
type Options struct {
	// Password for every demo account; empty generates a random one.
	Password string
	// Conversations to create (default DefaultConversations).
	Conversations int
	// DevCatalog publishes a trial plan when the catalog is empty (local / CI
	// only) so the demo organization gets a writable subscription.
	DevCatalog bool
	// Now anchors every relative timestamp (default time.Now).
	Now time.Time
	Log *slog.Logger
}

// Account is one demo login.
type Account struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// Result describes a finished seed.
type Result struct {
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationSlug string    `json:"organization_slug"`
	Created          bool      `json:"created"`
	Password         string    `json:"password"`
	Accounts         []Account `json:"accounts"`
	WidgetPublicID   uuid.UUID `json:"widget_public_id"`
	Conversations    int       `json:"conversations"`
	Articles         int       `json:"articles"`
}

// seeder carries the ids created along the way.
type seeder struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	opts  Options
	rnd   *mrand.Rand
	org   db.Organization
	users map[string]db.User // by demoUser.key
	teams map[string]db.Team
	// team name -> member user ids
	teamMembers map[string][]int64
	tags        map[string]db.Tag
	channels    map[string]db.ChannelInstance
	companies   []seededCompany
	individuals []db.Contact
	policies    map[string]db.SlaPolicy
	targets     map[string]map[string]int32 // policy -> priority -> first response minutes
	resolved    []db.Conversation
	convByScn   map[int][]db.Conversation
}

type seededCompany struct {
	row      db.Company
	tier     string
	contacts []db.Contact
}

// Run creates or refreshes the demo organization.
func Run(ctx context.Context, pool *pgxpool.Pool, opts Options) (Result, error) {
	if opts.Conversations <= 0 {
		opts.Conversations = DefaultConversations
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	opts.Now = opts.Now.UTC()
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if opts.Password == "" {
		p, err := GeneratePassword()
		if err != nil {
			return Result{}, err
		}
		opts.Password = p
	}
	if err := password.ValidatePassword(opts.Password); err != nil {
		return Result{}, fmt.Errorf("demo password: %w", err)
	}
	s := &seeder{
		pool: pool, q: db.New(pool), opts: opts,
		// A fixed seed keeps the sample data the same on every refresh.
		rnd:   mrand.New(mrand.NewPCG(134, 2026)), // #nosec G404 -- reproducible fixture data, not a secret
		users: map[string]db.User{}, teams: map[string]db.Team{}, teamMembers: map[string][]int64{},
		tags: map[string]db.Tag{}, channels: map[string]db.ChannelInstance{}, policies: map[string]db.SlaPolicy{},
		targets: map[string]map[string]int32{}, convByScn: map[int][]db.Conversation{},
	}
	res := Result{Password: opts.Password, OrganizationSlug: OrgSlug}

	if err := s.upsertUsers(ctx); err != nil {
		return res, fmt.Errorf("users: %w", err)
	}
	billing := billingusecase.New(billingrepo.New(pool, s.q))
	if opts.DevCatalog {
		if err := s.ensureDevCatalog(ctx, billing); err != nil {
			return res, fmt.Errorf("dev catalog: %w", err)
		}
	}
	created, err := s.ensureOrganization(ctx, billing)
	if err != nil {
		return res, fmt.Errorf("organization: %w", err)
	}
	if err := s.ensureSubscription(ctx, billing); err != nil {
		return res, fmt.Errorf("subscription: %w", err)
	}
	res.Created = created
	res.OrganizationUUID = s.org.Uuid

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		s.q = db.New(tx)
		if !created {
			if err := wipe(ctx, tx, s.org.ID); err != nil {
				return fmt.Errorf("wipe: %w", err)
			}
		}
		steps := []struct {
			name string
			fn   func(context.Context) error
		}{
			{"memberships", s.memberships},
			{"teams", s.seedTeams},
			{"sla", s.seedSLA},
			{"tags", s.seedTags},
			{"channels", s.seedChannels},
			{"companies", s.seedCompanies},
			{"conversations", func(ctx context.Context) error { return s.seedConversations(ctx, tx) }},
			{"macros", s.seedMacros},
			{"views", s.seedViews},
			{"surveys", s.seedSurveys},
			{"engineering", s.seedEngineering},
			{"appointments", s.seedAppointments},
		}
		for _, st := range steps {
			if err := st.fn(ctx); err != nil {
				return fmt.Errorf("%s: %w", st.name, err)
			}
		}
		return nil
	})
	s.q = db.New(pool)
	if err != nil {
		return res, err
	}
	articles, err := s.seedKnowledge(ctx)
	if err != nil {
		return res, fmt.Errorf("knowledge: %w", err)
	}
	if err := s.seedHealth(ctx); err != nil {
		return res, fmt.Errorf("health: %w", err)
	}

	// Fixtures bypass domain events, so project reports explicitly after all ratings/SLA data exist.
	if err := analyticsrepo.New(pool, s.q).Backfill(ctx, s.org.ID); err != nil {
		return res, fmt.Errorf("analytics: %w", err)
	}

	res.Articles = articles
	res.Conversations = opts.Conversations
	res.WidgetPublicID = s.channels["widget"].Uuid
	for _, u := range demoUsers {
		res.Accounts = append(res.Accounts, Account{Email: userEmail(u), Name: u.name + " " + u.surname, Role: u.role})
	}
	return res, nil
}

// GeneratePassword returns a random password that passes the password
// policy (upper, lower and digit).
func GeneratePassword() (string, error) {
	const (
		upper = "ABCDEFGHJKLMNPQRSTUVWXYZ"
		lower = "abcdefghijkmnopqrstuvwxyz"
		digit = "23456789"
	)
	all := upper + lower + digit
	out := make([]byte, 0, 18)
	for _, set := range []string{upper, lower, digit} {
		c, err := pick(set)
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}
	for len(out) < 18 {
		c, err := pick(all)
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}
	return string(out), nil
}

func pick(set string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
	if err != nil {
		return 0, err
	}
	return set[n.Int64()], nil
}

func userEmail(u demoUser) string { return u.key + "@" + AgentDomain }

func (s *seeder) upsertUsers(ctx context.Context) error {
	hash, err := password.Hash(s.opts.Password)
	if err != nil {
		return err
	}
	verified := ts(s.opts.Now)
	for _, u := range demoUsers {
		email := userEmail(u)
		row, err := s.q.GetUserByEmail(ctx, email)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			row, err = s.q.CreateUser(ctx, db.CreateUserParams{
				Email: email, PasswordHash: hash, Name: u.name, Surname: u.surname,
				Status: "active", EmailVerifiedAt: verified,
			})
			if err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if err := s.q.UpdateUserPasswordByID(ctx, db.UpdateUserPasswordByIDParams{ID: row.ID, PasswordHash: hash}); err != nil {
				return err
			}
		}
		s.users[u.key] = row
	}
	return nil
}

// ensureOrganization creates the organization through the organizations
// usecase (owner membership + billing trial hook) or loads the existing one.
func (s *seeder) ensureOrganization(ctx context.Context, billing *billingusecase.Service) (bool, error) {
	org, err := s.q.GetOrganizationBySlug(ctx, OrgSlug)
	if err == nil {
		if org.Name != OrgName {
			return false, fmt.Errorf("slug %q belongs to %q, not the demo organization; refusing to touch it", OrgSlug, org.Name)
		}
		s.org = org
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	orgs := orgusecase.New(orgrepo.NewPostgres(s.pool, s.q), billingusecase.TrialHooks{Service: billing})
	created, err := orgs.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
		Name: OrgName, Slug: OrgSlug, OwnerUserID: s.users[demoUsers[0].key].ID,
	})
	if err != nil {
		return false, err
	}
	if s.org, err = s.q.GetOrganizationByUUID(ctx, created.UUID); err != nil {
		return false, err
	}
	if s.org.Slug != OrgSlug {
		return false, fmt.Errorf("organization slug %q was allocated instead of %q", s.org.Slug, OrgSlug)
	}
	// The demo starts with the setup wizard done.
	if err := s.q.CreateOrganizationOnboarding(ctx, s.org.ID); err != nil {
		return false, err
	}
	if _, err := s.q.FinishOrganizationOnboarding(ctx, s.org.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *seeder) memberships(ctx context.Context) error {
	members := orgrepo.NewPostgres(s.pool, s.q)
	for _, u := range demoUsers {
		user := s.users[u.key]
		role, err := s.q.GetTenantRoleByKey(ctx, db.GetTenantRoleByKeyParams{OrganizationID: i8(s.org.ID), RoleKey: u.role})
		if err != nil {
			return fmt.Errorf("role %s: %w", u.role, err)
		}
		m, err := members.GetOrganizationMember(ctx, s.q, s.org.ID, user.ID)
		if errors.Is(err, orgrepo.ErrNotFound) {
			if err := members.AddOrganizationMember(ctx, s.q, db.AddOrganizationMemberParams{
				OrganizationID: s.org.ID, UserID: user.ID, RoleID: role.ID,
			}); err != nil {
				return err
			}
			if m, err = members.GetOrganizationMember(ctx, s.q, s.org.ID, user.ID); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if err := members.ReplaceMemberRoles(ctx, s.q, s.org.ID, m.ID, []int64{role.ID}, role.ID); err != nil {
			return err
		}
		if len(u.skills) > 0 {
			if _, err := s.q.SetMemberSkills(ctx, db.SetMemberSkillsParams{OrganizationID: s.org.ID, UserID: user.ID, Skills: u.skills}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *seeder) seedTeams(ctx context.Context) error {
	hours := mustJSON(weekdays("09:00", "18:00"))
	for _, t := range demoTeams {
		row, err := s.q.CreateTeam(ctx, db.CreateTeamParams{
			OrganizationID: s.org.ID, Name: t.name, Description: t.description, Color: t.color,
			Timezone: pgtype.Text{String: "Europe/Istanbul", Valid: true}, WorkingHours: hours, Skills: []string{},
		})
		if err != nil {
			return err
		}
		s.teams[t.name] = row
	}
	for _, u := range demoUsers {
		for _, name := range u.teams {
			role := "member"
			if u.lead {
				role = "lead"
			}
			if err := s.q.UpsertTeamMember(ctx, db.UpsertTeamMemberParams{
				OrganizationID: s.org.ID, TeamID: s.teams[name].ID, UserID: s.users[u.key].ID, Role: role,
			}); err != nil {
				return err
			}
			s.teamMembers[name] = append(s.teamMembers[name], s.users[u.key].ID)
		}
	}
	return nil
}

func (s *seeder) seedSLA(ctx context.Context) error {
	bh, err := s.q.InsertBusinessHours(ctx, db.InsertBusinessHoursParams{
		OrganizationID: s.org.ID, Name: "Mesai saatleri", Timezone: "Europe/Istanbul",
		WorkingHours: mustJSON(weekdays("09:00", "18:00")), UsePublicHolidays: true, HolidayCountry: "TR", IsDefault: true,
	})
	if err != nil {
		return err
	}
	type target struct{ first, next, resolution int32 }
	policies := []struct {
		name, description string
		conditions        map[string]any
		targets           map[string]target
	}{
		{"Kurumsal SLA", "Kurumsal sözleşmeli müşteriler için öncelikli hedefler", map[string]any{"company_tiers": []string{"enterprise"}},
			map[string]target{"urgent": {30, 60, 240}, "high": {60, 120, 480}, "normal": {120, 240, 1440}, "low": {240, 480, 2880}}},
		{"Standart SLA", "Diğer tüm talepler", map[string]any{},
			map[string]target{"urgent": {60, 120, 480}, "high": {120, 240, 1440}, "normal": {240, 480, 2880}, "low": {480, 960, 4320}}},
	}
	for i, p := range policies {
		row, err := s.q.InsertSLAPolicy(ctx, db.InsertSLAPolicyParams{
			OrganizationID: s.org.ID, Name: p.name, Description: p.description, IsActive: true, Position: safeconv.ToInt32(i),
			BusinessHoursID: pgtype.Int8{Int64: bh.ID, Valid: true}, Conditions: mustJSON(p.conditions), WarningPercent: 80,
		})
		if err != nil {
			return err
		}
		s.policies[p.name] = row
		s.targets[p.name] = map[string]int32{}
		for prio, t := range p.targets {
			if err := s.q.InsertSLAPolicyTarget(ctx, db.InsertSLAPolicyTargetParams{
				OrganizationID: s.org.ID, PolicyID: row.ID, Priority: prio,
				FirstResponseMinutes: i4(t.first), NextResponseMinutes: i4(t.next), ResolutionMinutes: i4(t.resolution),
			}); err != nil {
				return err
			}
			s.targets[p.name][prio] = t.first
		}
	}
	return nil
}

func (s *seeder) seedTags(ctx context.Context) error {
	for _, t := range demoTags {
		row, err := s.q.UpsertTag(ctx, db.UpsertTagParams{OrganizationID: s.org.ID, Name: t.name, Color: t.color})
		if err != nil {
			return err
		}
		s.tags[t.name] = row
	}
	return nil
}

func (s *seeder) seedChannels(ctx context.Context) error {
	owner := s.users[demoUsers[0].key].ID
	for _, c := range demoChannels {
		settings := map[string]any{"demo": true}
		if c.kind == "widget" {
			settings = map[string]any{
				"demo": true, "color": "#0f766e", "locale": "tr", "greeting_title": "Merhaba 👋",
				"greeting_text": "Anadolu Yazılım destek ekibi genellikle birkaç dakika içinde yanıt verir.",
				"show_team":     true, "prechat": map[string]any{"enabled": true, "name": "optional", "email": "required"},
				"csat": true, "file_upload": true,
			}
		}
		ext := pgtype.Text{}
		if c.externalID != "" {
			ext = pgtype.Text{String: c.externalID, Valid: true}
		}
		row, err := s.q.InsertChannelInstance(ctx, db.InsertChannelInstanceParams{
			Uuid: uuid.New(), OrganizationID: s.org.ID, ChannelType: c.kind, Name: c.name, Status: "active",
			ExternalID: ext, Settings: mustJSON(settings), CreatedBy: i8(owner),
		})
		if err != nil {
			return err
		}
		s.channels[c.key] = row
	}
	return nil
}

func (s *seeder) seedCompanies(ctx context.Context) error {
	owners := []int64{s.users["zeynep.aydin"].ID, s.users["elif.kara"].ID, s.users["mert.demir"].ID}
	phone := 0
	nextPhone := func() string {
		phone++
		return fmt.Sprintf("+90 555 000 %02d %02d", phone/100, phone%100)
	}
	for i, c := range demoCompanies {
		sla := pgtype.Int8{}
		if c.tier == "enterprise" {
			sla = i8(s.policies["Kurumsal SLA"].ID)
		}
		row, err := s.q.InsertCompany(ctx, db.InsertCompanyParams{
			OrganizationID: s.org.ID, Name: c.name, Domain: txt(c.domain), Notes: c.notes, CustomFields: []byte(`{}`),
			Tier: c.tier, AccountOwnerID: i8(owners[i%len(owners)]), SlaPolicyID: sla,
		})
		if err != nil {
			return err
		}
		sc := seededCompany{row: row, tier: c.tier}
		for _, p := range c.contacts {
			ct, err := s.q.InsertContact(ctx, db.InsertContactParams{
				OrganizationID: s.org.ID, CompanyID: i8(row.ID), Name: p.name, Email: txt(p.local + "@" + c.domain),
				Phone: txt(nextPhone()), Locale: txt("tr"), Timezone: txt("Europe/Istanbul"), CustomFields: []byte(`{}`), JobTitle: p.title,
			})
			if err != nil {
				return err
			}
			sc.contacts = append(sc.contacts, ct)
		}
		s.companies = append(s.companies, sc)
	}
	for _, p := range demoIndividuals {
		ct, err := s.q.InsertContact(ctx, db.InsertContactParams{
			OrganizationID: s.org.ID, Name: p.name, Email: txt(p.local + "@example.com"), Phone: txt(nextPhone()),
			Locale: txt("tr"), Timezone: txt("Europe/Istanbul"), CustomFields: []byte(`{}`),
		})
		if err != nil {
			return err
		}
		s.individuals = append(s.individuals, ct)
	}
	return nil
}

// channelFor spreads conversations: e-mail 45%, widget 30%, WhatsApp 17%,
// Instagram 8%.
func (s *seeder) channelFor(i int) string {
	switch r := (i * 37) % 100; {
	case r < 45:
		return "email"
	case r < 75:
		return "widget"
	case r < 92:
		return "whatsapp"
	default:
		return "instagram"
	}
}

func (s *seeder) seedConversations(ctx context.Context, tx pgx.Tx) error {
	now := s.opts.Now
	n := s.opts.Conversations
	for i := 0; i < n; i++ {
		scnIdx := i % len(scenarios)
		scn := scenarios[scnIdx]
		channelKey := s.channelFor(i)
		ch := s.channels[channelKey]

		var contact db.Contact
		var company *seededCompany
		if channelKey == "email" || (channelKey == "widget" && i%3 != 0) || len(s.individuals) == 0 {
			company = &s.companies[s.rnd.IntN(len(s.companies))]
			contact = company.contacts[s.rnd.IntN(len(company.contacts))]
		} else {
			contact = s.individuals[s.rnd.IntN(len(s.individuals))]
		}
		if err := s.identity(ctx, channelKey, ch, contact); err != nil {
			return err
		}

		// Newest conversations first in index order: i=0 is minutes old,
		// the last one about 60 days old.
		age := time.Duration(float64(60*24*time.Hour) * float64(i) / float64(n))
		age += time.Duration(s.rnd.IntN(90)) * time.Minute
		start := now.Add(-age)
		status := "resolved"
		switch {
		case age < 6*time.Hour:
			status = []string{"open", "open", "pending"}[i%3]
		case age < 3*24*time.Hour:
			status = []string{"open", "pending", "resolved", "open"}[i%4]
		case i%11 == 0:
			status = "open"
		case i%5 == 0:
			status = "closed"
		}
		priority := scn.priority
		team := s.teams[scn.team]
		assignee := pgtype.Int8{}
		if members := s.teamMembers[scn.team]; len(members) > 0 && (status != "open" || i%4 != 0) {
			assignee = i8(members[s.rnd.IntN(len(members))])
		}
		var companyID pgtype.Int8
		if company != nil {
			companyID = i8(company.row.ID)
		}
		number, err := s.q.NextConversationNumber(ctx, s.org.ID)
		if err != nil {
			return err
		}
		thread := pgtype.Text{}
		if channelKey == "email" {
			thread = txt(fmt.Sprintf("<demo-%d-%d@%s>", s.org.ID, number, AgentDomain))
		}
		conv, err := s.q.InsertConversation(ctx, db.InsertConversationParams{
			OrganizationID: s.org.ID, Number: number, ChannelType: ch.ChannelType, ChannelInstanceID: i8(ch.ID),
			ContactID: contact.ID, CompanyID: companyID, Subject: scn.subject, Status: "open", Priority: priority,
			Source: "inbound", AssigneeUserID: assignee, AssigneeTeamID: i8(team.ID), CustomFields: []byte(`{}`),
			ExternalThreadID: thread, LastMessageAt: ts(start),
		})
		if err != nil {
			return err
		}
		// Open conversations stop after the customer's latest message.
		msgs := scn.msgs
		if status == "open" {
			last := 0
			for j := range msgs {
				if j%2 == 0 {
					last = j
				}
			}
			if i%2 == 0 {
				last = 0
			}
			msgs = msgs[:last+1]
		}
		at := start
		agentID := assignee
		if !agentID.Valid {
			agentID = i8(s.teamMembers[scn.team][0])
		}
		for j, body := range msgs {
			at = at.Add(time.Duration(5+s.rnd.IntN(120)) * time.Minute)
			if at.After(now) {
				at = now.Add(-time.Duration(len(msgs)-j) * time.Minute)
			}
			if err := s.message(ctx, conv, ch, contact, agentID, j, body, at); err != nil {
				return err
			}
		}
		for _, tag := range scn.tags {
			if _, err := s.q.AddConversationTag(ctx, db.AddConversationTagParams{
				OrganizationID: s.org.ID, ConversationID: conv.ID, TagID: s.tags[tag].ID, CreatedBy: agentID,
			}); err != nil {
				return err
			}
		}
		policy := "Standart SLA"
		if company != nil && company.tier == "enterprise" {
			policy = "Kurumsal SLA"
		}
		due := start.Add(time.Duration(s.targets[policy][priority]) * time.Minute)
		breached := pgtype.Timestamptz{}
		if due.Before(now) && status == "open" && len(msgs) == 1 {
			breached = ts(due)
		}
		if err := s.q.SetConversationSLA(ctx, db.SetConversationSLAParams{
			SlaPolicyID: i8(s.policies[policy].ID), FirstResponseDueAt: ts(due),
			ResolutionDueAt: ts(start.Add(48 * time.Hour)), SlaBreachedAt: breached, OrganizationID: s.org.ID, ID: conv.ID,
		}); err != nil {
			return err
		}
		state := db.UpdateConversationStateParams{
			Subject: scn.subject, Status: status, Priority: priority, AssigneeUserID: assignee,
			AssigneeTeamID: i8(team.ID), CompanyID: companyID, CustomFields: []byte(`{}`),
			OrganizationID: s.org.ID, ID: conv.ID,
		}
		if status == "resolved" || status == "closed" {
			state.ResolvedAt = ts(at.Add(10 * time.Minute))
		}
		if status == "closed" {
			state.ClosedAt = ts(at.Add(3 * 24 * time.Hour))
		}
		conv, err = s.q.UpdateConversationState(ctx, state)
		if err != nil {
			return err
		}
		// Backdate creation so reports and the inbox show a realistic history.
		if _, err := tx.Exec(ctx, `UPDATE conversations SET created_at = $1, updated_at = $2 WHERE organization_id = $3 AND id = $4`,
			start, at, s.org.ID, conv.ID); err != nil {
			return err
		}
		if status == "resolved" || status == "closed" {
			s.resolved = append(s.resolved, conv)
		}
		s.convByScn[scnIdx] = append(s.convByScn[scnIdx], conv)
	}
	return nil
}

func (s *seeder) identity(ctx context.Context, channelKey string, ch db.ChannelInstance, c db.Contact) error {
	var ext, username string
	switch channelKey {
	case "whatsapp":
		ext = strings.NewReplacer("+", "", " ", "").Replace(c.Phone.String)
	case "instagram":
		username = strings.ReplaceAll(strings.ToLower(strings.Split(c.Email.String, "@")[0]), ".", "_")
		ext = "demo-ig-" + username
	case "email":
		ext = c.Email.String
	default:
		return nil
	}
	_, err := s.q.UpsertContactIdentity(ctx, db.UpsertContactIdentityParams{
		OrganizationID: s.org.ID, ContactID: c.ID, ChannelType: ch.ChannelType, ChannelInstanceID: i8(ch.ID),
		ExternalID: ext, DisplayName: c.Name, Username: username, Meta: []byte(`{}`), LastSeenAt: ts(s.opts.Now),
	})
	return err
}

func (s *seeder) message(ctx context.Context, conv db.Conversation, ch db.ChannelInstance, contact db.Contact, agent pgtype.Int8, j int, body string, at time.Time) error {
	kind := "text"
	if ch.ChannelType == "email" {
		kind = "email"
	}
	p := db.InsertMessageParams{
		OrganizationID: s.org.ID, ConversationID: conv.ID, ChannelInstanceID: i8(ch.ID), Kind: kind,
		Content: body, ContentAttributes: []byte(`{}`), CreatedAt: ts(at),
	}
	preview := body
	if len([]rune(preview)) > 140 {
		preview = string([]rune(preview)[:140])
	}
	switch {
	case strings.HasPrefix(body, "#"):
		p.Direction, p.SenderType, p.SenderUserID, p.Status, p.Kind, p.Private = "internal", "agent", agent, "recorded", "note", true
		p.Content = strings.TrimPrefix(body, "#")
		p.ChannelInstanceID = pgtype.Int8{}
		if _, err := s.q.InsertMessage(ctx, p); err != nil {
			return err
		}
		return s.q.ApplyInternalToConversation(ctx, db.ApplyInternalToConversationParams{OrganizationID: s.org.ID, ID: conv.ID})
	case j%2 == 0:
		p.Direction, p.SenderType, p.SenderContactID, p.Status = "in", "contact", i8(contact.ID), "received"
		if _, err := s.q.InsertMessage(ctx, p); err != nil {
			return err
		}
		_, err := s.q.ApplyInboundToConversation(ctx, db.ApplyInboundToConversationParams{At: ts(at), Preview: preview, OrganizationID: s.org.ID, ID: conv.ID})
		return err
	default:
		p.Direction, p.SenderType, p.SenderUserID, p.Status, p.SentAt = "out", "agent", agent, "read", ts(at)
		if _, err := s.q.InsertMessage(ctx, p); err != nil {
			return err
		}
		_, err := s.q.ApplyOutboundToConversation(ctx, db.ApplyOutboundToConversationParams{At: ts(at), Preview: preview, ByAgent: true, OrganizationID: s.org.ID, ID: conv.ID})
		return err
	}
}

func (s *seeder) seedMacros(ctx context.Context) error {
	admin := i8(s.users["mert.demir"].ID)
	for _, m := range demoMacros {
		if _, err := s.q.InsertMacro(ctx, db.InsertMacroParams{
			OrganizationID: s.org.ID, Scope: "organization", Name: m.name, Description: m.description,
			Actions: mustJSON(m.actions), CreatedBy: admin,
		}); err != nil {
			return err
		}
	}
	for _, c := range demoCanned {
		if _, err := s.q.InsertCannedResponse(ctx, db.InsertCannedResponseParams{
			OrganizationID: s.org.ID, Scope: "organization", Title: c.title, Shortcut: c.shortcut,
			Category: c.category, Content: c.content, CreatedBy: admin,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) seedViews(ctx context.Context) error {
	owner := s.users[demoUsers[0].key].ID
	views := []struct {
		scope, name, icon string
		owner             pgtype.Int8
		filters           map[string]any
	}{
		{"organization", "Acil ve yüksek öncelikli", "flame", pgtype.Int8{}, map[string]any{"statuses": []string{"open", "pending"}, "priorities": []string{"urgent", "high"}}},
		{"organization", "SLA ihlali", "alarm-clock", pgtype.Int8{}, map[string]any{"sla_breached": true}},
		{"organization", "Atanmamış", "inbox", pgtype.Int8{}, map[string]any{"statuses": []string{"open"}, "assignee": "unassigned"}},
		{"personal", "Fatura konuları", "receipt", i8(owner), map[string]any{"tags": []uuid.UUID{s.tags["fatura"].Uuid}}},
	}
	for i, v := range views {
		if _, err := s.q.InsertConversationView(ctx, db.InsertConversationViewParams{
			OrganizationID: s.org.ID, Scope: v.scope, OwnerUserID: v.owner, Name: v.name, Icon: v.icon,
			Filters: mustJSON(v.filters), Position: safeconv.ToInt32(i), CreatedBy: i8(owner),
		}); err != nil {
			return err
		}
	}
	return nil
}

// seedSurveys writes CSAT ratings on resolved conversations (what the widget
// and e-mail rating links store) and NPS answers through the survey templates.
func (s *seeder) seedSurveys(ctx context.Context) error {
	if _, err := s.q.SurveyCreateTemplate(ctx, db.SurveyCreateTemplateParams{
		OrganizationID: s.org.ID, Name: "Çözüm sonrası memnuniyet", Kind: "csat",
		Question: "Destek ekibimizden aldığınız hizmetten ne kadar memnun kaldınız?", OnResolve: true, Enabled: true,
	}); err != nil {
		return err
	}
	nps, err := s.q.SurveyCreateTemplate(ctx, db.SurveyCreateTemplateParams{
		OrganizationID: s.org.ID, Name: "Çeyreklik NPS", Kind: "nps",
		Question: "Anadolu Yazılım'ı bir iş arkadaşınıza tavsiye etme olasılığınız nedir?", Enabled: true,
	})
	if err != nil {
		return err
	}
	comments := []string{"", "Hızlı ve net bir çözüm oldu, teşekkürler.", "", "Biraz geç dönüş aldık ama sorun çözüldü.", "Harika destek!", ""}
	for i, c := range s.resolved {
		// Roughly two thirds of the resolved conversations were rated.
		if i%3 == 2 {
			continue
		}
		score := int16(5)
		switch r := s.rnd.IntN(10); {
		case r < 1:
			score = 2
		case r < 3:
			score = 3
		case r < 6:
			score = 4
		}
		if _, err := s.q.InsertConversationRating(ctx, db.InsertConversationRatingParams{
			OrganizationID: s.org.ID, ConversationID: c.ID, ContactID: i8(c.ContactID), ChannelInstanceID: c.ChannelInstanceID,
			Score: score, Comment: comments[i%len(comments)],
		}); err != nil {
			return err
		}
		if i%4 != 0 {
			continue
		}
		inv, err := s.q.SurveyInvite(ctx, db.SurveyInviteParams{
			PeriodKey: "2026-Q4", ConversationUuid: c.Uuid, OrganizationID: s.org.ID, TemplateUuid: nps.Uuid,
		})
		if err != nil {
			return err
		}
		if _, err := s.q.SurveyAnswer(ctx, db.SurveyAnswerParams{
			OrganizationID: s.org.ID, InvitationID: inv.ID, Kind: "nps", Score: safeconv.ToInt16(6 + s.rnd.IntN(5)), Comment: "",
		}); err != nil {
			return err
		}
		if err := s.q.SurveyMarkAnswered(ctx, db.SurveyMarkAnsweredParams{OrganizationID: s.org.ID, ID: inv.ID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) seedEngineering(ctx context.Context) error {
	project, err := s.q.EngineeringCreateProject(ctx, db.EngineeringCreateProjectParams{OrganizationID: s.org.ID, Name: "Ürün"})
	if err != nil {
		return err
	}
	// Issues link to the conversations of the scenario that raised them.
	links := map[int]int{0: 9, 1: 2, 2: 4, 3: 14}
	for i, is := range demoIssues {
		row, err := s.q.EngineeringCreateIssue(ctx, db.EngineeringCreateIssueParams{
			OrganizationID: s.org.ID, Title: is.title, Description: is.description, Status: is.status, Priority: is.priority,
			Labels: []string{"müşteri-bildirimi"}, Cycle: "2026-Q4", ProjectID: i8(project.ID),
			AssigneeUserID: i8(s.users["mert.demir"].ID), CreatedBy: i8(s.users["elif.kara"].ID), Context: []byte(`{}`),
		})
		if err != nil {
			return err
		}
		convs := s.convByScn[links[i]]
		for k := 0; k < len(convs) && k < 3; k++ {
			if _, err := s.q.EngineeringLink(ctx, db.EngineeringLinkParams{
				OrganizationID: s.org.ID, IssueID: row.ID, ConversationID: convs[k].ID, NotifyCustomer: true,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// seedKnowledge writes the help center through the knowledge usecase so the
// publish events index the articles like an editor's publish would. Article
// versions are immutable, so a refresh updates existing articles by slug and
// publishes a new version only when the content changed.
func (s *seeder) seedKnowledge(ctx context.Context) (int, error) {
	svc := knowledgeusecase.New(knowledgerepo.New(s.pool))
	author := s.users["zeynep.aydin"].ID
	existingCols, _, err := svc.Collections(ctx, s.org.ID, 200, 0)
	if err != nil {
		return 0, err
	}
	collections := map[string]uuid.UUID{}
	for _, c := range existingCols {
		collections[c.Slug] = c.Uuid
	}
	for _, c := range demoCollections {
		id, ok := collections[c.slug]
		if !ok {
			id = uuid.New()
		}
		row, err := svc.SaveCollection(ctx, s.org.ID, id, knowledgemodel.CollectionInput{Name: c.name, Slug: c.slug, Kind: "collection"})
		if err != nil {
			return 0, err
		}
		collections[c.slug] = row.Uuid
	}
	existing, _, err := svc.Articles(ctx, s.org.ID, knowledgeusecase.ArticleFilter{Locale: "tr"}, 500, 0)
	if err != nil {
		return 0, err
	}
	bySlug := map[string]db.KnowledgeArticlesRow{}
	for _, a := range existing {
		bySlug[a.Slug] = a
	}
	for _, a := range demoArticles {
		visibility := "internal"
		if a.public {
			visibility = "public"
		}
		col := collections[a.collection]
		in := knowledgemodel.ArticleInput{
			Collection: &col, Slug: a.slug, Locale: "tr", Title: a.title, Body: a.body, Tags: a.tags, Visibility: visibility,
		}
		id, version := uuid.New(), int32(0)
		if old, ok := bySlug[a.slug]; ok {
			cur, err := svc.Article(ctx, s.org.ID, old.Uuid)
			if err != nil {
				return 0, err
			}
			id, version = old.Uuid, cur.Version
			changed := cur.Title != a.title || cur.Body != a.body || cur.Visibility != visibility ||
				strings.Join(cur.Tags, ",") != strings.Join(a.tags, ",") || cur.CollectionID.Int64 == 0
			if changed {
				in.Version = cur.Version
				row, err := svc.SaveArticle(ctx, s.org.ID, author, id, in, false)
				if err != nil {
					return 0, fmt.Errorf("article %s: %w", a.slug, err)
				}
				version = row.Version
			} else if cur.PublishedVersion.Valid && cur.PublishedVersion.Int32 == cur.Version {
				continue
			}
		} else {
			row, err := svc.SaveArticle(ctx, s.org.ID, author, id, in, true)
			if err != nil {
				return 0, fmt.Errorf("article %s: %w", a.slug, err)
			}
			version = row.Version
		}
		if err := svc.Publish(ctx, s.org.ID, id, version, true); err != nil {
			return 0, fmt.Errorf("publish %s: %w", a.slug, err)
		}
	}
	return len(demoArticles), nil
}

// seedHealth enables account health and scores every company from the
// seeded conversations and CSAT answers.
func (s *seeder) seedHealth(ctx context.Context) error {
	svc := usecase.New(s.pool, s.q, activity.NewRecorder(s.q, s.opts.Log), s.opts.Log).WithClock(func() time.Time { return s.opts.Now })
	enabled, threshold := true, 60
	if _, err := svc.UpdateSettings(ctx, s.org.ID, usecase.SettingsInput{Enabled: &enabled, Threshold: &threshold}); err != nil {
		return err
	}
	_, _, err := svc.ComputeOrganization(ctx, s.org.ID)
	return err
}

func weekdays(open, closeAt string) map[string]any {
	out := map[string]any{}
	for _, d := range []string{"mon", "tue", "wed", "thu", "fri"} {
		out[d] = map[string]any{"closed": false, "open": open, "close": closeAt}
	}
	for _, d := range []string{"sat", "sun"} {
		out[d] = map[string]any{"closed": true, "open": "", "close": ""}
	}
	return out
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func i8(v int64) pgtype.Int8            { return pgtype.Int8{Int64: v, Valid: v != 0} }
func i4(v int32) pgtype.Int4            { return pgtype.Int4{Int32: v, Valid: true} }
func txt(v string) pgtype.Text          { return pgtype.Text{String: v, Valid: v != ""} }
