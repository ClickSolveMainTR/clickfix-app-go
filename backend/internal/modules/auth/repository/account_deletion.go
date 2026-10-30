package repository

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccountSearchDeleter removes denormalized personal data from the search index.
type AccountSearchDeleter interface {
	DeleteDocuments(context.Context, string, []string) error
}

func (r *Postgres) SetDeletionSearch(search AccountSearchDeleter) { r.deletionSearch = search }

// ErrSoleOwner protects an organization from losing its last active owner.
var ErrSoleOwner = errors.New("organizasyonun tek sahibisiniz. Hesabınızı silmeden önce sahipliği devredin veya organizasyonu kapatın")

func (r *Postgres) SetDeletionGrace(grace time.Duration) {
	if grace > 0 {
		r.deletionGrace = grace
	}
}

func checkDeletionOwnership(ctx context.Context, q *db.Queries, userID int64) error {
	members, err := q.AccountDeletionOrganizations(ctx, userID)
	if err != nil {
		return err
	}
	for _, m := range members {
		// Use the same organization lock as ownership transfer/member removal.
		if _, err = q.AccountDeletionLockOrganization(ctx, m); errors.Is(err, pgx.ErrNoRows) {
			continue
		} else if err != nil {
			return err
		}
		sole, err := q.AccountDeletionSoleOwner(ctx, db.AccountDeletionSoleOwnerParams{OrganizationID: m, UserID: userID})
		if err != nil {
			return err
		}
		if sole {
			return ErrSoleOwner
		}
	}
	return nil
}

// PurgeDueAccounts adapts takwim's transactional account purge. Keeping an
// anonymous tombstone preserves organization messages and notes and their author.
func (r *Postgres) PurgeDueAccounts(ctx context.Context) error {
	due, err := r.q.AccountDeletionDue(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, candidate := range due {
		err = r.withTx(ctx, func(q *db.Queries) error {
			u, err := q.AccountDeletionLock(ctx, candidate.ID)
			if err != nil {
				return err
			}
			if u.DeletedAt.Valid || !u.DeletionDueAt.Valid || u.DeletionDueAt.Time.After(time.Now()) {
				return nil
			}
			if err = checkDeletionOwnership(ctx, q, u.ID); err != nil {
				return err
			}
			if r.deletionSearch != nil {
				if err = r.deletionSearch.DeleteDocuments(ctx, "users", []string{u.Uuid.String()}); err != nil {
					return err
				}
				messages, err := q.AccountDeletionSearchMessages(ctx, pgtype.Int8{Int64: u.ID, Valid: true})
				if err != nil {
					return err
				}
				// Batch index work so long-lived support accounts do not spend
				// one asynchronous search task per authored message.
				for offset := 0; offset < len(messages); offset += 500 {
					end := min(offset+500, len(messages))
					ids := make([]string, 0, end-offset)
					for _, m := range messages[offset:end] {
						ids = append(ids, m.Uuid.String())
					}
					if err = r.deletionSearch.DeleteDocuments(ctx, "messages", ids); err != nil {
						return err
					}
				}
			}
			if err = q.PurgeUserSignInData(ctx, u.ID); err != nil {
				return err
			}
			if err = q.PurgeUserPersonalData(ctx, u.ID); err != nil {
				return err
			}
			members, err := q.AccountDeletionOrganizations(ctx, u.ID)
			if err != nil {
				return err
			}
			for _, m := range members {
				if err = q.AccountDeletionPurgeOrganization(ctx, db.AccountDeletionPurgeOrganizationParams{OrganizationID: m, UserID: u.ID}); err != nil {
					return err
				}
			}
			if err = q.AccountDeletionScrubAudit(ctx, pgtype.Int8{Int64: u.ID, Valid: true}); err != nil {
				return err
			}
			return q.AccountDeletionAnonymize(ctx, u.ID)
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
