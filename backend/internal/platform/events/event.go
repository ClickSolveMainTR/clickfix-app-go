package events

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

// idempotencyNamespace derives deterministic event ids from producer keys.
var idempotencyNamespace = uuid.MustParse("6f1d2c1e-9b0a-4f53-9a4e-2f8c7d0e5b11")

// Event is the standard envelope published on the platform bus.
type Event struct {
	RequestID string
	Name      string
	// Version is the payload schema version (1 when unset). Consumers that
	// decode typed payloads reject versions newer than they know.
	Version int
	// TenantID is the owning organization (organizations are the tenants).
	TenantID    *int64
	ActorUserID *int64
	EntityType  string
	EntityID    *int64
	EntityUUID  *uuid.UUID
	Payload     map[string]any
	OccurredAt  time.Time
	EventID     uuid.UUID
}

// New builds an Event with a fresh EventID and UTC OccurredAt.
func New(name string) Event {
	return Event{
		Name:       name,
		Version:    1,
		Payload:    map[string]any{},
		OccurredAt: time.Now().UTC(),
		EventID:    uuid.New(),
	}
}

// WithOrganization stamps the event with the organization it belongs to
// (tenant events); handlers pass it on, e.g. to organization-stamped
// notifications. Platform events leave it nil.
func (e Event) WithOrganization(organizationID int64) Event {
	e.TenantID = &organizationID
	return e
}

// WithActor sets ActorUserID.
func (e Event) WithActor(userID int64) Event {
	e.ActorUserID = &userID
	return e
}

// WithEntity sets EntityType and optional numeric / UUID identifiers.
func (e Event) WithEntity(entityType string, entityID *int64, entityUUID *uuid.UUID) Event {
	e.EntityType = entityType
	e.EntityID = entityID
	e.EntityUUID = entityUUID
	return e
}

// WithPayload replaces the payload map.
func (e Event) WithPayload(payload map[string]any) Event {
	if payload == nil {
		payload = map[string]any{}
	}
	e.Payload = payload
	return e
}

// WithIdempotencyKey derives EventID from the event name, organization and a
// producer key (e.g. a channel's external message id). Enqueueing the same
// logical event twice then keeps a single outbox row. Call it after
// WithOrganization.
func (e Event) WithIdempotencyKey(key string) Event {
	org := ""
	if e.TenantID != nil {
		org = strconv.FormatInt(*e.TenantID, 10)
	}
	e.EventID = uuid.NewSHA1(idempotencyNamespace, []byte(e.Name+"\x00"+org+"\x00"+key))
	return e
}

// NormalizeNumbers converts whole float64 values (from encoding/json) back to
// int64 so consumers see the same types before and after a JSON round trip.
func NormalizeNumbers(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case map[string]any:
		for k, vv := range x {
			x[k] = NormalizeNumbers(vv)
		}
		return x
	case []any:
		for i, vv := range x {
			x[i] = NormalizeNumbers(vv)
		}
		return x
	default:
		return v
	}
}

// WithTenant is the boilerplate name of WithOrganization.
func (e Event) WithTenant(organizationID int64) Event {
	return e.WithOrganization(organizationID)
}
