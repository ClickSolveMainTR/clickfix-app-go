package inbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
)

func ptr[T any](v T) *T { return &v }

// CF-249: custom statuses ("İşleniyor") are seeded, managed, set on a
// conversation on top of the base status, kept by unrelated changes and
// carried by the status_changed event to the channel notifier.
func TestCustomConversationStatuses(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	conv := e.ib.Conversations

	list, err := conv.CustomStatuses(ctx, e.org)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "İşleniyor" || list[0].Category != convmodel.StatusOpen || list[0].Slug != "isleniyor" {
		t.Fatalf("seeded statuses %+v", list)
	}
	processing := list[0]

	if _, err := conv.CreateCustomStatus(ctx, e.org, convusecase.CustomStatusInput{Name: ptr("Ertelendi gibi"), Category: ptr(convmodel.StatusSnoozed)}); !errors.Is(err, convusecase.ErrInvalid) {
		t.Fatalf("snoozed category err = %v", err)
	}
	if _, err := conv.CreateCustomStatus(ctx, e.org, convusecase.CustomStatusInput{Name: ptr("işleniyor"), Category: ptr(convmodel.StatusOpen)}); !errors.Is(err, convusecase.ErrStatusExists) {
		t.Fatalf("duplicate name err = %v", err)
	}
	supplier, err := conv.CreateCustomStatus(ctx, e.org, convusecase.CustomStatusInput{Name: ptr("Tedarikçi bekleniyor"), Color: ptr("#f59e0b"), Category: ptr(convmodel.StatusPending)})
	if err != nil || supplier.Slug != "tedarikci-bekleniyor" || supplier.SortOrder != 1 {
		t.Fatalf("create %+v %v", supplier, err)
	}

	if code := e.post(t, `{"messages":[{"id":"cs-1","from":{"id":"v-1","name":"Ayşe"},"text":"Kargom nerede?"}]}`, ""); code != 200 {
		t.Fatalf("webhook = %d", code)
	}
	var convUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id = $1", e.org).Scan(&convUUID); err != nil {
		t.Fatal(err)
	}
	set := func(in convusecase.UpdateInput) error {
		in.OrganizationID, in.ConversationUUID = e.org, convUUID
		_, err := conv.Update(ctx, in)
		return err
	}
	get := func() convmodel.Conversation {
		d, err := conv.Get(ctx, e.org, e.agent, convUUID)
		if err != nil {
			t.Fatal(err)
		}
		return d.Conversation
	}
	ref := func(id uuid.UUID) **uuid.UUID { p := &id; return &p }

	// İşleniyor: base status stays open, the custom status is shown.
	if err := set(convusecase.UpdateInput{CustomStatus: ref(processing.UUID)}); err != nil {
		t.Fatal(err)
	}
	if c := get(); c.Status != convmodel.StatusOpen || c.CustomStatus == nil || c.CustomStatus.UUID != processing.UUID || c.CustomStatus.Color != "#3b82f6" {
		t.Fatalf("after İşleniyor %+v %+v", c.Status, c.CustomStatus)
	}
	var raw []byte
	if err := e.pool.QueryRow(ctx, `SELECT payload FROM outbox_events WHERE organization_id = $1 AND event_name = 'conversations.updated'
		AND payload->'data'->>'change' = 'status_changed' ORDER BY id DESC LIMIT 1`, e.org).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	ev, err := outbox.UnmarshalEnvelope("conversations.updated", raw)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := channelsmodule.StatusEventFrom(ev)
	if !ok || st.From != "open" || st.To != "open" || st.FromCustom != nil || st.ToCustom == nil || st.ToCustom.Name != "İşleniyor" {
		t.Fatalf("status event %+v ok=%v", st, ok)
	}

	// Unrelated changes and the same base status keep it.
	if err := set(convusecase.UpdateInput{Priority: ptr(convmodel.PriorityHigh), Status: ptr(convmodel.StatusOpen)}); err != nil {
		t.Fatal(err)
	}
	if c := get(); c.CustomStatus == nil || c.CustomStatus.UUID != processing.UUID {
		t.Fatalf("priority change dropped the custom status %+v", c.CustomStatus)
	}

	// A custom status must match an explicit base status.
	if err := set(convusecase.UpdateInput{Status: ptr(convmodel.StatusResolved), CustomStatus: ref(supplier.UUID)}); !errors.Is(err, convusecase.ErrInvalid) {
		t.Fatalf("mismatched custom status err = %v", err)
	}
	// Supplier: base status moves to pending with it.
	if err := set(convusecase.UpdateInput{CustomStatus: ref(supplier.UUID)}); err != nil {
		t.Fatal(err)
	}
	if c := get(); c.Status != convmodel.StatusPending || c.CustomStatus == nil || c.CustomStatus.Name != "Tedarikçi bekleniyor" {
		t.Fatalf("after supplier %s %+v", c.Status, c.CustomStatus)
	}

	// Moving the status to another category takes it off the conversation.
	if _, err := conv.UpdateCustomStatus(ctx, e.org, supplier.UUID, convusecase.CustomStatusInput{Category: ptr(convmodel.StatusResolved)}); err != nil {
		t.Fatal(err)
	}
	if c := get(); c.Status != convmodel.StatusPending || c.CustomStatus != nil {
		t.Fatalf("after category move %s %+v", c.Status, c.CustomStatus)
	}

	// Another base status drops the custom status.
	if err := set(convusecase.UpdateInput{CustomStatus: ref(processing.UUID)}); err != nil {
		t.Fatal(err)
	}
	if err := set(convusecase.UpdateInput{Status: ptr(convmodel.StatusPending)}); err != nil {
		t.Fatal(err)
	}
	if c := get(); c.Status != convmodel.StatusPending || c.CustomStatus != nil {
		t.Fatalf("after pending %s %+v", c.Status, c.CustomStatus)
	}

	// Deleting a status in use keeps the base status.
	if err := set(convusecase.UpdateInput{CustomStatus: ref(processing.UUID)}); err != nil {
		t.Fatal(err)
	}
	if err := conv.DeleteCustomStatus(ctx, e.org, processing.UUID); err != nil {
		t.Fatal(err)
	}
	if c := get(); c.Status != convmodel.StatusOpen || c.CustomStatus != nil {
		t.Fatalf("after delete %s %+v", c.Status, c.CustomStatus)
	}
	if err := conv.DeleteCustomStatus(ctx, e.org, processing.UUID); !errors.Is(err, convusecase.ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}
