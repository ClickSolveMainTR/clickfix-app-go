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
	counts  db.OrganizationSetupCountsRow
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

func (f *fakeQueries) OrganizationSetupCounts(context.Context, int64) (db.OrganizationSetupCountsRow, error) {
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
	if !st.Active || st.Total != 5 || st.Completed != 0 || Next(st.Steps) != StepCustomer {
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

// CF-198: the steps follow the set-up order, each with its "done"
// conditions; a skipped step stays as a warning until its conditions hold.
func TestStepsFollowTheSetupOrderAndConditions(t *testing.T) {
	q := newFake()
	_ = q.CreateOrganizationOnboarding(context.Background(), 1)
	q.counts = db.OrganizationSetupCountsRow{Widgets: 1, CompanyProfile: true, Articles: 3}
	identity := false
	svc := New(q, &fakeOrgs{}, Config{}).WithIdentityKeys(func(context.Context, int64) (bool, error) { return identity, nil })

	st, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(st.Steps))
	for _, s := range st.Steps {
		keys = append(keys, s.Key)
	}
	if want := []string{StepCustomer, StepSystem, StepAI, StepKnowledge, StepAutomation}; !equal(keys, want) {
		t.Fatalf("order %v, want %v", keys, want)
	}
	// Articles alone do not finish the customer step: the order is the
	// point (the customer is introduced first).
	if stepDone(st, StepCustomer) || !stepDone(st, StepKnowledge) || st.Completed != 1 || Next(st.Steps) != StepCustomer {
		t.Fatalf("partial facts: %+v", st)
	}
	customer := st.Steps[0]
	want := map[string]bool{CondIdentityKey: false, CondExternalIdentity: false, CondPackage: false, CondCompany: true}
	for _, c := range customer.Conditions {
		if want[c.Key] != c.Done {
			t.Fatalf("condition %s = %v, want %v", c.Key, c.Done, want[c.Key])
		}
	}
	if got := MissingKeys(st.Steps); got[0] != "customer.identity_key" || got[len(got)-1] != "automation.automation" {
		t.Fatalf("missing keys: %v", got)
	}

	// Every customer condition holds once the key, an external id and a
	// package exist.
	identity = true
	q.counts.ExternalContacts, q.counts.PackagedCompanies = 1, 1
	st, _ = svc.Get(context.Background(), 1)
	if !stepDone(st, StepCustomer) || Next(st.Steps) != StepSystem {
		t.Fatalf("customer step: %+v", st.Steps[0])
	}

	// Skipping the system step leaves it as a warning and moves on.
	if _, err := svc.SkipStep(context.Background(), 1, "channel"); !errors.Is(err, ErrUnknownStep) {
		t.Fatalf("old step keys are gone: want ErrUnknownStep, got %v", err)
	}
	st, err = svc.SkipStep(context.Background(), 1, StepSystem)
	if err != nil || !st.Steps[1].Skipped || st.Steps[1].Done || st.Skipped != 1 || Next(st.Steps) != StepAI {
		t.Fatalf("skip: %+v %v", st, err)
	}
	if got := MissingKeys(st.Steps); got[0] != "system.system" {
		t.Fatalf("skipped steps stay in the missing list: %v", got)
	}
	// A skipped step that later gets done is done, not skipped.
	q.counts.Systems = 1
	st, _ = svc.Get(context.Background(), 1)
	if !st.Steps[1].Done || st.Steps[1].Skipped || st.Skipped != 0 {
		t.Fatalf("done after skip: %+v", st.Steps[1])
	}
	// AI needs both the switch and a published procedure; automation
	// needs an SLA policy and a rule or workflow.
	q.counts.AiEnabled, q.counts.Procedures, q.counts.SlaPolicies, q.counts.Automations = true, 0, 1, 0
	st, _ = svc.Get(context.Background(), 1)
	if stepDone(st, StepAI) || stepDone(st, StepAutomation) {
		t.Fatalf("half-met steps must not be done: %+v", st)
	}
	q.counts.Procedures, q.counts.Automations = 1, 1
	st, _ = svc.Get(context.Background(), 1)
	if st.Completed != 5 || Next(st.Steps) != "" || len(MissingKeys(st.Steps)) != 0 {
		t.Fatalf("all done: %+v", st)
	}
	// Checklist evaluates organizations without a checklist row too.
	cl, err := svc.Checklist(context.Background(), 99)
	if err != nil || cl.Active || len(cl.Steps) != 5 {
		t.Fatalf("legacy organization checklist: %+v %v", cl, err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	if _, err := svc.SkipStep(context.Background(), 1, StepSystem); !errors.Is(err, ErrNotActive) {
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
