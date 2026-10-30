// Package onboarding brings a new organization to its first customer
// message (CF-13): a signed-in account opens its own organization
// (self-serve, platform sign-up mode "open") and the owner follows a persistent
// checklist: connect a first channel, invite the team, install the web
// widget and receive the first conversation.
//
// Progress lives in organization_onboarding. Channels, members,
// invitations and conversations are counted from their own tables, so a
// step done anywhere in the product ticks itself; the steps the product
// cannot observe (the widget snippet pasted on a site) are confirmed by
// hand. Only organizations opened through self-serve get a row, so older
// organizations never see the checklist.
package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
)

// Step keys in checklist order.
const (
	StepOrganization      = "organization"
	StepChannel           = "channel"
	StepTeam              = "team"
	StepWidget            = "widget"
	StepFirstConversation = "first_conversation"
)

// stepOrder is the order the checklist shows.
var stepOrder = []string{StepOrganization, StepChannel, StepTeam, StepWidget, StepFirstConversation}

// manualSteps can be confirmed by hand (POST …/steps/{step}). The team
// step also accepts "I work alone" so a one-person organization can finish.
var manualSteps = map[string]bool{StepWidget: true, StepTeam: true}

// MaxOwnedOrganizations caps how many organizations one account opens itself.
const MaxOwnedOrganizations = 3

const (
	maxNameLength = 120
	minNameLength = 2
)

// Errors.
var (
	// ErrSelfServeDisabled: the platform does not let accounts open an
	// organization themselves.
	ErrSelfServeDisabled = errors.New("opening an organization is not available")
	// ErrOrganizationLimit: the account already owns MaxOwnedOrganizations.
	ErrOrganizationLimit = errors.New("this account already owns the maximum number of organizations")
	// ErrNotActive: the organization has no checklist or it is finished.
	ErrNotActive = errors.New("the guided set-up is not active for this organization")
	// ErrUnknownStep: not a step that can be confirmed by hand.
	ErrUnknownStep = errors.New("unknown step")
	// ErrInvalidName: the organization name is empty or too long.
	ErrInvalidName = errors.New("organization name must be 2-120 characters")
	// ErrInvalidPrice: the chosen price is not on a published public plan.
	ErrInvalidPrice = errors.New("price must belong to a published public plan")
)

// Queries is the sqlc subset the service reads and writes.
type Queries interface {
	CreateOrganizationOnboarding(ctx context.Context, organizationID int64) error
	GetOrganizationOnboarding(ctx context.Context, organizationID int64) (db.OrganizationOnboarding, error)
	AddOrganizationOnboardingStep(ctx context.Context, arg db.AddOrganizationOnboardingStepParams) (int64, error)
	FinishOrganizationOnboarding(ctx context.Context, organizationID int64) (int64, error)
	OrganizationOnboardingCounts(ctx context.Context, organizationID int64) (db.OrganizationOnboardingCountsRow, error)
	CountOwnedOrganizations(ctx context.Context, userID int64) (int64, error)
	SelfServeSignupPriceAllowed(ctx context.Context, price uuid.UUID) (bool, error)
}

// Organizations creates organizations (organizations usecase).
type Organizations interface {
	CreateOrganization(ctx context.Context, in orgmodel.CreateOrganizationInput) (orgmodel.Organization, error)
}

// Config are the platform switches of the flow.
type Config struct {
	// SelfServe lets signed-in accounts open an organization.
	SelfServe bool
	// SelfServeOpen, when set, decides instead of SelfServe: the platform
	// sign-up mode (open = self-serve on), read on every request so the
	// platform admin can switch it without a redeploy (CF-143).
	SelfServeOpen func(context.Context) (bool, error)
}

// GrantCredits tops up the organization's AI wallet (billing
// WalletCommand); the key makes the grant idempotent.
type GrantCredits func(ctx context.Context, org int64, key string, credits int64, expires time.Time) error

// Service runs the onboarding flow.
type Service struct {
	q    Queries
	orgs Organizations
	cfg  Config
	// grant and credits: the onboarding AI allowance (CF-140).
	grant   GrantCredits
	credits int64
}

// WithAIAllowance grants every self-serve organization a one-time AI
// credit allowance (valid 30 days) so the AI-assisted set-up works on day
// one; 0 credits or a nil grant turns it off.
func (s *Service) WithAIAllowance(grant GrantCredits, credits int64) *Service {
	s.grant, s.credits = grant, credits
	return s
}

// New builds the service.
func New(q Queries, orgs Organizations, cfg Config) *Service {
	return &Service{q: q, orgs: orgs, cfg: cfg}
}

// Step is one checklist row.
type Step struct {
	Key  string `json:"key"`
	Done bool   `json:"done"`
	// Manual: the owner can tick it by hand.
	Manual bool `json:"manual"`
}

// State is the checklist of one organization.
type State struct {
	// Active: the organization has a checklist that is not finished.
	Active     bool       `json:"active"`
	FinishedAt *time.Time `json:"finished_at"`
	Steps      []Step     `json:"steps"`
	Completed  int        `json:"completed"`
	Total      int        `json:"total"`
}

// CreateInput opens an organization for the caller.
type CreateInput struct {
	UserID int64
	Name   string
	Slug   string
	// PriceUUID (optional, CF-42) is the plan price picked on the pricing
	// page; the signup trial runs on it instead of the default trial plan.
	PriceUUID uuid.UUID
}

// CreateOrganization opens an organization owned by the caller and starts
// its checklist.
func (s *Service) CreateOrganization(ctx context.Context, in CreateInput) (orgmodel.Organization, error) {
	open := s.cfg.SelfServe
	if s.cfg.SelfServeOpen != nil {
		var err error
		if open, err = s.cfg.SelfServeOpen(ctx); err != nil {
			return orgmodel.Organization{}, fmt.Errorf("onboarding: sign-up mode: %w", err)
		}
	}
	if !open {
		return orgmodel.Organization{}, ErrSelfServeDisabled
	}
	name := strings.TrimSpace(in.Name)
	if n := len([]rune(name)); n < minNameLength || n > maxNameLength {
		return orgmodel.Organization{}, ErrInvalidName
	}
	owned, err := s.q.CountOwnedOrganizations(ctx, in.UserID)
	if err != nil {
		return orgmodel.Organization{}, fmt.Errorf("onboarding: count owned: %w", err)
	}
	if owned >= MaxOwnedOrganizations {
		return orgmodel.Organization{}, ErrOrganizationLimit
	}
	if in.PriceUUID != uuid.Nil {
		allowed, err := s.q.SelfServeSignupPriceAllowed(ctx, in.PriceUUID)
		if err != nil {
			return orgmodel.Organization{}, fmt.Errorf("onboarding: trial price: %w", err)
		}
		if !allowed {
			return orgmodel.Organization{}, ErrInvalidPrice
		}
	}
	org, err := s.orgs.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
		Name: name, Slug: strings.TrimSpace(in.Slug), OwnerUserID: in.UserID, TrialPriceUUID: in.PriceUUID,
	})
	if err != nil {
		return orgmodel.Organization{}, err
	}
	if err := s.q.CreateOrganizationOnboarding(ctx, org.ID); err != nil {
		return orgmodel.Organization{}, fmt.Errorf("onboarding: start checklist: %w", err)
	}
	if s.grant != nil && s.credits > 0 {
		// Best effort: without a subscription (or wallet) the set-up still
		// works and falls back to templates.
		_ = s.grant(ctx, org.ID, "onboarding-allowance:"+org.UUID.String(), s.credits, time.Now().Add(30*24*time.Hour))
	}
	return org, nil
}

// Get returns the checklist of the organization.
func (s *Service) Get(ctx context.Context, orgID int64) (State, error) {
	row, err := s.q.GetOrganizationOnboarding(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return State{Steps: []Step{}}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("onboarding: get: %w", err)
	}
	counts, err := s.q.OrganizationOnboardingCounts(ctx, orgID)
	if err != nil {
		return State{}, fmt.Errorf("onboarding: counts: %w", err)
	}
	return buildState(row, counts), nil
}

// CompleteStep ticks a manual step and returns the new checklist.
func (s *Service) CompleteStep(ctx context.Context, orgID int64, step string) (State, error) {
	if !manualSteps[step] {
		return State{}, ErrUnknownStep
	}
	if err := s.requireActive(ctx, orgID); err != nil {
		return State{}, err
	}
	if _, err := s.q.AddOrganizationOnboardingStep(ctx, db.AddOrganizationOnboardingStepParams{
		OrganizationID: orgID, Step: step,
	}); err != nil {
		return State{}, fmt.Errorf("onboarding: add step: %w", err)
	}
	return s.Get(ctx, orgID)
}

// Finish closes the checklist (completed or dismissed).
func (s *Service) Finish(ctx context.Context, orgID int64) (State, error) {
	if err := s.requireActive(ctx, orgID); err != nil {
		return State{}, err
	}
	if _, err := s.q.FinishOrganizationOnboarding(ctx, orgID); err != nil {
		return State{}, fmt.Errorf("onboarding: finish: %w", err)
	}
	return s.Get(ctx, orgID)
}

func (s *Service) requireActive(ctx context.Context, orgID int64) error {
	row, err := s.q.GetOrganizationOnboarding(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotActive
	}
	if err != nil {
		return fmt.Errorf("onboarding: get: %w", err)
	}
	if row.FinishedAt.Valid {
		return ErrNotActive
	}
	return nil
}

func buildState(row db.OrganizationOnboarding, c db.OrganizationOnboardingCountsRow) State {
	manual := make(map[string]bool, len(row.Steps))
	for _, s := range row.Steps {
		manual[s] = true
	}
	auto := map[string]bool{
		StepOrganization:      true,
		StepChannel:           c.Channels > 0,
		StepTeam:              c.Members > 1 || c.Invitations > 0,
		StepWidget:            c.Widgets > 0,
		StepFirstConversation: c.Conversations > 0,
	}
	st := State{Active: !row.FinishedAt.Valid, Steps: make([]Step, 0, len(stepOrder))}
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		st.FinishedAt = &t
	}
	for _, key := range stepOrder {
		done := auto[key] || manual[key]
		st.Steps = append(st.Steps, Step{Key: key, Done: done, Manual: manualSteps[key]})
		if done {
			st.Completed++
		}
	}
	st.Total = len(stepOrder)
	return st
}
