// Package onboarding brings a new organization live (CF-13, CF-198): a
// signed-in account opens its own organization (self-serve, platform
// sign-up mode "open") and the owner follows a persistent checklist in the
// order the product must be set up: introduce the customer, connect the
// system, teach the AI, fill the knowledge base, then automation and SLA.
//
// Progress lives in organization_onboarding. Every step's conditions are
// read from the tables they live in (checklist.go), so a step done
// anywhere in the product ticks itself; a step the owner skips stays as a
// warning. Only organizations opened through self-serve get a row, so
// older organizations never see the checklist, but Checklist evaluates
// any organization for the assistant.
package onboarding

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
)

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
	// ErrUnknownStep: not a checklist step.
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
	OrganizationSetupCounts(ctx context.Context, organizationID int64) (db.OrganizationSetupCountsRow, error)
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

// IdentityKeys reports whether the organization's web widget has an
// identity key (the secret that signs verified visitors); the key is
// encrypted, so the channels module answers.
type IdentityKeys func(ctx context.Context, orgID int64) (bool, error)

// Service runs the onboarding flow.
type Service struct {
	q    Queries
	orgs Organizations
	cfg  Config
	// grant and credits: the onboarding AI allowance (CF-140).
	grant    GrantCredits
	credits  int64
	identity IdentityKeys
}

// WithIdentityKeys sets the identity-key check of the customer step.
func (s *Service) WithIdentityKeys(fn IdentityKeys) *Service { s.identity = fn; return s }

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

// Condition is one "is this step done" check of a step.
type Condition struct {
	Key  string `json:"key"`
	Done bool   `json:"done"`
}

// Step is one checklist row: done when every condition holds; skipped
// steps stay as warnings and count towards finishing.
type Step struct {
	Key        string      `json:"key"`
	Done       bool        `json:"done"`
	Skipped    bool        `json:"skipped"`
	Conditions []Condition `json:"conditions"`
}

// State is the checklist of one organization.
type State struct {
	// Active: the organization has a checklist that is not finished.
	Active     bool       `json:"active"`
	FinishedAt *time.Time `json:"finished_at"`
	Steps      []Step     `json:"steps"`
	// Completed counts done steps; Skipped the skipped ones.
	Completed int `json:"completed"`
	Skipped   int `json:"skipped"`
	Total     int `json:"total"`
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
	facts, err := s.facts(ctx, orgID)
	if err != nil {
		return State{}, err
	}
	return buildState(row, facts), nil
}

// Checklist evaluates the steps of any organization, with or without a
// checklist row (the assistant's "what did you leave out" reads it).
func (s *Service) Checklist(ctx context.Context, orgID int64) (State, error) {
	row, err := s.q.GetOrganizationOnboarding(ctx, orgID)
	hasRow := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		row = db.OrganizationOnboarding{OrganizationID: orgID, Steps: []string{}}
	} else if err != nil {
		return State{}, fmt.Errorf("onboarding: get: %w", err)
	}
	facts, err := s.facts(ctx, orgID)
	if err != nil {
		return State{}, err
	}
	st := buildState(row, facts)
	st.Active = st.Active && hasRow
	return st, nil
}

func (s *Service) facts(ctx context.Context, orgID int64) (Facts, error) {
	counts, err := s.q.OrganizationSetupCounts(ctx, orgID)
	if err != nil {
		return Facts{}, fmt.Errorf("onboarding: counts: %w", err)
	}
	f := FactsOf(counts)
	if s.identity != nil && counts.Widgets > 0 {
		if f.IdentityKey, err = s.identity(ctx, orgID); err != nil {
			return Facts{}, fmt.Errorf("onboarding: identity key: %w", err)
		}
	}
	return f, nil
}

// SkipStep marks a step as skipped (it stays as a warning) and returns the
// new checklist.
func (s *Service) SkipStep(ctx context.Context, orgID int64, step string) (State, error) {
	if !slices.Contains(StepOrder, step) {
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

func buildState(row db.OrganizationOnboarding, f Facts) State {
	skipped := make(map[string]bool, len(row.Steps))
	for _, k := range row.Steps {
		skipped[k] = true
	}
	st := State{Active: !row.FinishedAt.Valid, Steps: Evaluate(f, skipped)}
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		st.FinishedAt = &t
	}
	for _, step := range st.Steps {
		switch {
		case step.Done:
			st.Completed++
		case step.Skipped:
			st.Skipped++
		}
	}
	st.Total = len(st.Steps)
	return st
}
