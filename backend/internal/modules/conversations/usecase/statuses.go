package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
)

// ErrStatusExists: another custom status has the name.
var ErrStatusExists = errors.New("conversations: status name already used")

// CustomStatus is one of the organization's own conversation statuses
// (CF-249): a name and color on top of a base category.
type CustomStatus struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Color      string    `json:"color"`
	Category   string    `json:"category"`
	SortOrder  int32     `json:"sort_order"`
	UsageCount int64     `json:"usage_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CustomStatusInput creates or changes a custom status. Nil fields stay on
// update; name and category are required on create.
type CustomStatusInput struct {
	Name      *string
	Color     *string
	Category  *string
	SortOrder *int32
}

func customStatusOf(r db.ConversationStatus, usage int64) CustomStatus {
	return CustomStatus{
		UUID: r.Uuid, Name: r.Name, Slug: r.Slug, Color: r.Color, Category: r.Category,
		SortOrder: r.SortOrder, UsageCount: usage, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// CustomStatuses lists the organization's custom statuses in display order.
func (s *Service) CustomStatuses(ctx context.Context, orgID int64) ([]CustomStatus, error) {
	rows, err := s.store.Q().ListConversationStatusesWithUsage(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]CustomStatus, 0, len(rows))
	for _, r := range rows {
		out = append(out, customStatusOf(db.ConversationStatus{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, Name: r.Name, Slug: r.Slug, Category: r.Category,
			Color: r.Color, SortOrder: r.SortOrder, IsDefault: r.IsDefault, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, r.UsageCount))
	}
	return out, nil
}

// ValidCustomStatus checks a custom status name, color and category.
func ValidCustomStatus(name, color, category string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return "", "", fmt.Errorf("%w: status name must be 1-64 characters", ErrInvalid)
	}
	color = strings.TrimSpace(color)
	if len(color) > 32 {
		return "", "", fmt.Errorf("%w: color is too long", ErrInvalid)
	}
	if !model.Valid(category, model.CustomStatusCategories) {
		return "", "", fmt.Errorf("%w: category must be open, pending or resolved", ErrInvalid)
	}
	return name, color, nil
}

var trStatusSlug = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "İ", "i", "ö", "o", "ş", "s", "ü", "u", "Ç", "c", "Ğ", "g", "Ö", "o", "Ş", "s", "Ü", "u")

// StatusSlug turns a status name into its slug ("İşleniyor" -> "isleniyor").
func StatusSlug(name string) string {
	src := strings.ToLower(trStatusSlug.Replace(strings.TrimSpace(name)))
	var b strings.Builder
	dash := false
	for _, r := range src {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if len(out) > 60 {
		out = strings.TrimSuffix(out[:60], "-")
	}
	if out == "" {
		out = "status"
	}
	return out
}

func sameName(rows []db.ConversationStatus, name string, except int64) bool {
	for _, r := range rows {
		if r.ID != except && strings.EqualFold(strings.ToLower(r.Name), strings.ToLower(name)) {
			return true
		}
	}
	return false
}

// CreateCustomStatus adds a custom status at the end of the list.
func (s *Service) CreateCustomStatus(ctx context.Context, orgID int64, in CustomStatusInput) (CustomStatus, error) {
	if in.Name == nil || in.Category == nil {
		return CustomStatus{}, fmt.Errorf("%w: name and category are required", ErrInvalid)
	}
	color := ""
	if in.Color != nil {
		color = *in.Color
	}
	name, color, err := ValidCustomStatus(*in.Name, color, *in.Category)
	if err != nil {
		return CustomStatus{}, err
	}
	var out db.ConversationStatus
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		rows, err := tx.Q.ListConversationStatuses(ctx, orgID)
		if err != nil {
			return err
		}
		if sameName(rows, name, 0) {
			return ErrStatusExists
		}
		base := StatusSlug(name)
		slug := base
		for i := 2; ; i++ {
			taken, err := tx.Q.ConversationStatusSlugTaken(ctx, db.ConversationStatusSlugTakenParams{OrganizationID: orgID, Slug: slug})
			if err != nil {
				return err
			}
			if !taken {
				break
			}
			slug = base + "-" + strconv.Itoa(i)
		}
		order := int32(0)
		if in.SortOrder != nil {
			order = *in.SortOrder
		} else {
			next, err := tx.Q.NextConversationStatusOrder(ctx, orgID)
			if err != nil {
				return err
			}
			order = next
		}
		out, err = tx.Q.InsertConversationStatus(ctx, db.InsertConversationStatusParams{
			OrganizationID: orgID, Name: name, Slug: slug, Category: *in.Category, Color: color, SortOrder: order,
		})
		return err
	})
	if err != nil {
		return CustomStatus{}, err
	}
	return customStatusOf(out, 0), nil
}

// UpdateCustomStatus renames, recolors, reorders or moves a custom status to
// another base category. Moving it takes it off conversations whose base
// status no longer matches; their base status stays.
func (s *Service) UpdateCustomStatus(ctx context.Context, orgID int64, id uuid.UUID, in CustomStatusInput) (CustomStatus, error) {
	var out db.ConversationStatus
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		cur, err := tx.Q.GetConversationStatusByUUID(ctx, db.GetConversationStatusByUUIDParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return err
		}
		name, color, category, order := cur.Name, cur.Color, cur.Category, cur.SortOrder
		if in.Name != nil {
			name = *in.Name
		}
		if in.Color != nil {
			color = *in.Color
		}
		if in.Category != nil {
			category = *in.Category
		}
		if in.SortOrder != nil {
			order = *in.SortOrder
		}
		if name, color, err = ValidCustomStatus(name, color, category); err != nil {
			return err
		}
		rows, err := tx.Q.ListConversationStatuses(ctx, orgID)
		if err != nil {
			return err
		}
		if sameName(rows, name, cur.ID) {
			return ErrStatusExists
		}
		out, err = tx.Q.UpdateConversationStatusDef(ctx, db.UpdateConversationStatusDefParams{
			Name: name, Color: color, Category: category, SortOrder: order, OrganizationID: orgID, ID: cur.ID,
		})
		if err != nil {
			return err
		}
		if category != cur.Category {
			if _, err := tx.Q.ClearConversationStatusMismatch(ctx, db.ClearConversationStatusMismatchParams{
				OrganizationID: orgID, StatusID: pgInt8(cur.ID), Category: category,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return CustomStatus{}, notFound(err)
	}
	return customStatusOf(out, 0), nil
}

// DeleteCustomStatus removes a custom status; conversations that used it
// keep their base status.
func (s *Service) DeleteCustomStatus(ctx context.Context, orgID int64, id uuid.UUID) error {
	cur, err := s.store.Q().GetConversationStatusByUUID(ctx, db.GetConversationStatusByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return notFound(err)
	}
	if _, err := s.store.Q().DeleteConversationStatusDef(ctx, db.DeleteConversationStatusDefParams{OrganizationID: orgID, ID: cur.ID}); err != nil {
		return err
	}
	return nil
}

// statusRef is the payload shape of a custom status on events and the
// timeline; nil when the conversation has none.
func statusRef(ctx context.Context, tx *repository.Tx, orgID int64, id pgtype.Int8) (*model.CustomStatusRef, error) {
	if !id.Valid {
		return nil, nil
	}
	st, err := tx.Q.GetConversationStatusByID(ctx, db.GetConversationStatusByIDParams{OrganizationID: orgID, ID: id.Int64})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &model.CustomStatusRef{UUID: st.Uuid, Name: st.Name, Color: st.Color}, nil
}

func customStatusRef(id pgtype.UUID, name, color pgtype.Text) *model.CustomStatusRef {
	if !id.Valid {
		return nil
	}
	return &model.CustomStatusRef{UUID: uuid.UUID(id.Bytes), Name: name.String, Color: color.String}
}

// timelineExtra is what a status change event message keeps of the custom
// statuses: their names, so the timeline reads "Açık → İşleniyor".
func timelineExtra(extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, key := range map[string]string{"from_custom_status": "from_custom", "to_custom_status": "to_custom"} {
		if ref, ok := extra[k].(*model.CustomStatusRef); ok && ref != nil {
			out[key] = ref.Name
		}
	}
	return out
}
