package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/jackc/pgx/v5"
)

// ErrDomainTaken: another organization registered the sender domain.
var ErrDomainTaken = errors.New("channels: the domain belongs to another organization")

// ClaimEmailDomain records that an organization registered a sender domain
// (first come; a domain owned elsewhere returns ErrDomainTaken).
func (s *Service) ClaimEmailDomain(ctx context.Context, orgID, actorID int64, domain, providerID string) error {
	_, err := s.q.ClaimEmailDomain(ctx, db.ClaimEmailDomainParams{
		OrganizationID: orgID, Domain: strings.ToLower(domain), ResendDomainID: providerID, CreatedBy: optInt8(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDomainTaken
	}
	return err
}

// OwnsEmailDomain reports whether the organization registered the domain.
func (s *Service) OwnsEmailDomain(ctx context.Context, orgID int64, domain string) (bool, error) {
	_, err := s.q.GetEmailDomain(ctx, db.GetEmailDomainParams{OrganizationID: orgID, Domain: strings.ToLower(domain)})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CredentialGuard checks credentials against organization owned state
// before the provider is asked (Create, Update).
type CredentialGuard func(ctx context.Context, orgID int64, channelType string, creds channels.Credentials) error

// WithCredentialGuard installs a guard.
func (s *Service) WithCredentialGuard(g CredentialGuard) *Service {
	s.guard = g
	return s
}

func (s *Service) checkCredentials(ctx context.Context, orgID int64, channelType string, creds channels.Credentials) error {
	if s.guard == nil {
		return nil
	}
	if err := s.guard(ctx, orgID, channelType, creds); err != nil {
		if errors.Is(err, ErrInvalid) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}
