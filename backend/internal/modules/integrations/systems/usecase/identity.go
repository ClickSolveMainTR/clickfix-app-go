package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
)

// CF-195 merge points. Everything that knows how ClickFix stores a
// customer's external id lives in this file, so that the CF-195 change
// (external id as a first-class field of contacts and companies) is
// adapted here and nowhere else:
//
//   - person: today the widget keys a signed-in user by a contact
//     identity "user:<external id>" (platform/channels/widget Claims
//     .ExternalID). After CF-195 read contacts.external_id directly in
//     the IntegrationContactIdentity / IntegrationContactByExternalID
//     queries and drop widgetUserPrefix.
//   - company: today companies.external_id (set by widget identify
//     company.id and the companies API). The connected system calls it
//     {company_external_id}; if CF-195 renames the column, change the
//     two queries only.
//
// The placeholder names {external_id} and {company_external_id} are part
// of saved path templates and must stay stable.

const widgetUserPrefix = "user:"

// externalIDOf turns a stored identity into the customer's external id.
func externalIDOf(identity string) string {
	return strings.TrimPrefix(identity, widgetUserPrefix)
}

// storedIdentityOf is the stored form of an external id.
func storedIdentityOf(externalID string) string {
	return widgetUserPrefix + externalID
}

// identityOfContact reads the external ids of a contact.
func (s *Service) identityOfContact(ctx context.Context, org int64, contact uuid.UUID) (model.Identity, error) {
	row, err := s.Q.IntegrationContactIdentity(ctx, db.IntegrationContactIdentityParams{OrganizationID: org, Uuid: contact})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Identity{}, model.ErrNotFound
	}
	if err != nil {
		return model.Identity{}, err
	}
	id := row.Uuid
	out := model.Identity{ContactUUID: &id, ContactName: row.Name, ExternalID: externalIDOf(row.UserIdentity),
		CompanyName: row.CompanyName, CompanyExternalID: row.CompanyExternalID}
	if row.CompanyUuid.Valid {
		c := uuid.UUID(row.CompanyUuid.Bytes)
		out.CompanyUUID = &c
	}
	return out, nil
}

// contactByExternalID finds the contact a customer's external id belongs
// to (uuid.Nil when none has written yet).
func (s *Service) contactByExternalID(ctx context.Context, org int64, externalID string) (uuid.UUID, error) {
	id, err := s.Q.IntegrationContactByExternalID(ctx, db.IntegrationContactByExternalIDParams{OrganizationID: org, Identity: storedIdentityOf(externalID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return id, err
}

// ResolveIdentity returns what the connected system is asked with: the
// contact's stored ids, or the given external id (with the contact it
// belongs to, when one exists).
func (s *Service) ResolveIdentity(ctx context.Context, org int64, contact *uuid.UUID, externalID string) (model.Identity, error) {
	if contact != nil && *contact != uuid.Nil {
		return s.identityOfContact(ctx, org, *contact)
	}
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return model.Identity{}, nil
	}
	found, err := s.contactByExternalID(ctx, org, externalID)
	if err != nil {
		return model.Identity{}, err
	}
	if found != uuid.Nil {
		id, err := s.identityOfContact(ctx, org, found)
		if err == nil && id.ExternalID != "" {
			return id, nil
		}
	}
	return model.Identity{ExternalID: externalID}, nil
}

// identityOfConversation resolves the identity of a conversation's contact.
func (s *Service) identityOfConversation(ctx context.Context, org, conversation int64) (model.Identity, error) {
	contact, err := s.Q.IntegrationConversationContact(ctx, db.IntegrationConversationContactParams{OrganizationID: org, ID: conversation})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Identity{}, nil
	}
	if err != nil {
		return model.Identity{}, err
	}
	return s.identityOfContact(ctx, org, contact)
}

// vars are the template values of an identity.
func vars(id model.Identity) map[string]string {
	return map[string]string{model.KeyExternalID: id.ExternalID, model.KeyCompanyExternalID: id.CompanyExternalID}
}
