package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TaskMetaDeletion = "app:compliance:meta-deletion"

func metaTypes(app string) []string {
	if app == "instagram" {
		return []string{"instagram"}
	}
	return []string{"messenger", "whatsapp"}
}

// MetaCallback persists all erasure requests before acknowledging Meta. It
// intentionally stores neither signed_request nor the provider's subject id.
func (s *Service) MetaCallback(ctx context.Context, app, subject string, erase bool) (string, error) {
	code := strings.ReplaceAll(uuid.NewString(), "-", "")
	err := s.Repo.Transaction(ctx, func(q *db.Queries) error {
		instances, err := q.MetaSubjectInstances(ctx, db.MetaSubjectInstancesParams{Subject: subject, ChannelTypes: metaTypes(app)})
		if err != nil {
			return err
		}
		for _, i := range instances {
			if err = q.MetaDeauthorizeInstance(ctx, db.MetaDeauthorizeInstanceParams{OrganizationID: i.OrganizationID, ID: i.ID, Erase: erase}); err != nil {
				return err
			}
		}
		if !erase {
			return nil
		}
		receipt, err := q.MetaDeletionReceipt(ctx, code)
		if err != nil {
			return err
		}
		contactTypes := []string{"messenger"}
		if app == "instagram" {
			contactTypes = []string{"instagram"}
		}
		// A WhatsApp identity is a phone number, not a Facebook app-scoped id.
		// Never equate those two numeric namespaces.
		contacts, err := q.MetaSubjectContacts(ctx, db.MetaSubjectContactsParams{Subject: subject, ChannelTypes: contactTypes})
		if err != nil {
			return err
		}
		for _, c := range contacts {
			contact, err := q.ComplianceContact(ctx, db.ComplianceContactParams(c))
			if err != nil {
				return err
			}
			request, err := q.ComplianceRequest(ctx, db.ComplianceRequestParams{OrganizationID: c.OrganizationID, ContactID: contact.ID, Kind: "erase"})
			if err != nil {
				return err
			}
			if err = q.MetaDeletionLink(ctx, db.MetaDeletionLinkParams{OrganizationID: c.OrganizationID, ReceiptID: receipt.ID, RequestUuid: request.Uuid}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if erase && s.Jobs != nil {
		payload, _ := json.Marshal(code)
		// The receipt and daily recovery sweep make a Redis outage safe.
		_, _ = s.Jobs.EnqueueContext(ctx, asynq.NewTask(TaskMetaDeletion, payload), asynq.Queue(Queue), asynq.MaxRetry(8), asynq.Timeout(30*time.Minute))
	}
	return code, nil
}

func (s *Service) ProcessMetaDeletion(ctx context.Context, code string) error {
	rows, err := s.Repo.Q.MetaDeletionRequests(ctx, code)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err = s.Process(orgctx.WithOrganizationID(ctx, r.OrganizationID), r.OrganizationID, r.RequestUuid); err != nil {
			return err
		}
	}
	return s.Repo.Q.MetaDeletionComplete(ctx, code)
}
func (s *Service) MetaDeletionTask(ctx context.Context, t *asynq.Task) error {
	var code string
	if json.Unmarshal(t.Payload(), &code) != nil || len(code) != 32 {
		return fmt.Errorf("invalid receipt: %w", asynq.SkipRetry)
	}
	return s.ProcessMetaDeletion(ctx, code)
}
func (s *Service) sweepMeta(ctx context.Context) error {
	codes, err := s.Repo.Q.MetaDeletionPending(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, code := range codes {
		if err = s.ProcessMetaDeletion(ctx, code); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
