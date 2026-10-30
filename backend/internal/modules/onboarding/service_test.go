package onboarding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
)

type fakeQueries struct {
	rows    map[int64]*db.OrganizationOnboarding
	counts  db.OrganizationOnboardingCountsRow
	owned   int64
	allowed map[uuid.UUID]bool
}

func newFake() *fakeQueries { return &fakeQueries{rows: map[int64]*db.OrganizationOnboarding{}} }

func (f *fakeQueries) CreateOrganizationOnboarding(_ context.Context, id int64) error {
	if _, ok := f.rows[id]; !ok {
		f.rows[id] = &db.OrganizationOnboarding{OrganizationID: id, Steps: []string{}}
	}
	return nil
}

func (f *fakeQueries) GetOrganizationOnboarding(_ context.Context, id int64) (db.OrganizationOnboarding, error) {
	row, ok := f.rows[id]
	if !ok {
		return db.OrganizationOnboarding{}, pgx.ErrNoRows
	}
	return *row, nil
}

func (f *fakeQueries) AddOrganizationOnboardingStep(_ context.Context, arg db.AddOrganizationOnboardingStepParams) (int64, error) {
	row := f.rows[arg.OrganizationID]
	for _, s := range row.Steps {
		if s == arg.Step {
			return 0, nil
		}
	}
	row.Steps = append(row.Steps, arg.Step)
	return 1, nil
}

func (f *fakeQueries) FinishOrganizationOnboarding(_ context.Context, id int64) (int64, error) {
	f.rows[id].FinishedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return 1, nil
}

func (f *fakeQueries) OrganizationOnboardingCounts(context.Context, int64) (db.OrganizationOnboardingCountsRow, error) {
	return f.counts, nil
}

func (f *fakeQueries) CountOwnedOrganizations(context.Context, int64) (int64, error) {
	return f.owned, nil
}

func (f *fakeQueries) SelfServeSignupPriceAllowed(_ context.Context, price uuid.UUID) (bool, error) {
	return f.allowed[price], nil
}

type fakeOrgs struct {
	created []orgmodel.CreateOrganizationInput
}

func (f *fakeOrgs) CreateOrganization(_ context.Context, in orgmodel.CreateOrganizationInput) (orgmodel.Organization, error) {
	f.created = append(f.created, in)
	return orgmodel.Organization{ID: 42, Name: in.Name, Slug: "acme"}, nil
}

func stepDone(st State, key string) bool {
	for _, s := range st.Steps {
		if s.Key == key {
			return s.Done
		}
	}
	return false
}

func TestCreateOrganizationRequiresSelfServe(t *testing.T) {
	svc := New(newFake(), &fakeOrgs{}, Config{SelfServe: false})
	if _, err := svc.CreateOrganization(context.Background(), CreateInput{UserID: 1, Name: "Acme"}); !errors.Is(err, ErrSelfServeDisabled) {
		t.Fatalf("want ErrSelfServeDisabled, got %v", err)
	}
}

func TestCreateOrganizationValidatesAndCaps(t *testing.T) {
	q := newFake()
	orgs := &fakeOrgs{}
	svc := New(q, orgs, Config{SelfServe: true})
	if _, err := svc.CreateOrganization(context.Background(), CreateInput{UserID: 1, Name: " a "}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("short name: want ErrInvalidName, got %v", err)
	}
	q.owned = MaxOwnedOrganizations
	if _, err := svc.CreateOrganization(context.Background(), CreateInput{UserID: 1, Name: "Acme Destek"}); !errors.Is(err, ErrOrganizationLimit) {
		t.Fatalf("want ErrOrganizationLimit, got %v", err)
	}
	if len(orgs.created) != 0 {
		t.Fatalf("no organization may be created, got %d", len(orgs.created))
	}
}

func TestCreateOrganizationStartsChecklist(t *testing.T) {
	q := newFake()
	orgs := &fakeOrgs{}
	svc := New(q, orgs, Config{SelfServe: true})
	org, err := svc.CreateOrganization(context.Background(), CreateInput{UserID: 7, Name: "  Anadolu Teknik  "})
	if err != nil {
		t.Fatal(err)
	}
	if orgs.created[0].OwnerUserID != 7 || orgs.created[0].Name != "Anadolu Teknik" {
		t.Fatalf("unexpected create input %+v", orgs.created[0])
	}
	st, err := svc.Get(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || st.Total != 5 || st.Completed != 1 || !stepDone(st, StepOrganization) {
		t.Fatalf("fresh checklist: %+v", st)
	}
}

func TestCreateOrganizationTrialPrice(t *testing.T) {
	q := newFake()
	good, bad := uuid.New(), uuid.New()
	q.allowed = map[uuid.UUID]bool{good: true}
	orgs := &fakeOrgs{}
	svc := New(q, orgs, Config{SelfServe: true})
	if _, err := svc.CreateOrganization(context.Background(), CreateInput{UserID: 7, Name: "Anadolu Teknik", PriceUUID: bad}); !errors.Is(err, ErrInvalidPrice) {
		t.Fatalf("unknown price err=%v", err)
	}
	if len(orgs.created) != 0 {
		t.Fatal("organization created for a rejected price")
	}
	if _, err := svc.CreateOrganization(context.Background(), CreateInput{UserID: 7, Name: "Anadolu Teknik", PriceUUID: good}); err != nil {
		t.Fatal(err)
	}
	if orgs.created[0].TrialPriceUUID != good {
		t.Fatalf("trial price not forwarded: %+v", orgs.created[0])
	}
}

func TestStepsFollowCountsAndManualConfirmations(t *testing.T) {
	q := newFake()
	_ = q.CreateOrganizationOnboarding(context.Background(), 1)
	q.counts = db.OrganizationOnboardingCountsRow{Channels: 1, Members: 1}
	svc := New(q, &fakeOrgs{}, Config{})

	st, _ := svc.Get(context.Background(), 1)
	if !stepDone(st, StepChannel) || stepDone(st, StepTeam) || stepDone(st, StepWidget) {
		t.Fatalf("counts not applied: %+v", st)
	}
	if _, err := svc.CompleteStep(context.Background(), 1, StepChannel); !errors.Is(err, ErrUnknownStep) {
		t.Fatalf("channel is automatic: want ErrUnknownStep, got %v", err)
	}
	st, err := svc.CompleteStep(context.Background(), 1, StepWidget)
	if err != nil || !stepDone(st, StepWidget) {
		t.Fatalf("widget confirm: %+v %v", st, err)
	}
	q.counts.Invitations = 2
	st, _ = svc.Get(context.Background(), 1)
	if !stepDone(st, StepTeam) || st.Completed != 4 {
		t.Fatalf("pending invitation should tick team: %+v", st)
	}
}

func TestFinishClosesChecklist(t *testing.T) {
	q := newFake()
	_ = q.CreateOrganizationOnboarding(context.Background(), 1)
	svc := New(q, &fakeOrgs{}, Config{})
	st, err := svc.Finish(context.Background(), 1)
	if err != nil || st.Active || st.FinishedAt == nil {
		t.Fatalf("finish: %+v %v", st, err)
	}
	if _, err := svc.Finish(context.Background(), 1); !errors.Is(err, ErrNotActive) {
		t.Fatalf("second finish: want ErrNotActive, got %v", err)
	}
	if _, err := svc.CompleteStep(context.Background(), 1, StepWidget); !errors.Is(err, ErrNotActive) {
		t.Fatalf("step after finish: want ErrNotActive, got %v", err)
	}
}

func TestOrganizationWithoutChecklist(t *testing.T) {
	svc := New(newFake(), &fakeOrgs{}, Config{})
	st, err := svc.Get(context.Background(), 99)
	if err != nil || st.Active || len(st.Steps) != 0 {
		t.Fatalf("legacy organization: %+v %v", st, err)
	}
	if _, err := svc.Finish(context.Background(), 99); !errors.Is(err, ErrNotActive) {
		t.Fatalf("want ErrNotActive, got %v", err)
	}
}
