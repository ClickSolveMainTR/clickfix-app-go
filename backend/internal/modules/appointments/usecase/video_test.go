package usecase_test

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/videorooms"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type videoEnt struct{ enabled bool }

func (e videoEnt) Resolve(context.Context, int64) (entitlements.Snapshot, error) {
	return entitlements.Snapshot{Writable: true, Values: map[string]entitlement.Value{
		"video.enabled": {Kind: "boolean", Enabled: e.enabled},
	}}, nil
}

// videoWallet takes the billing lock on its own connection, as
// billing.MeteredBalance does: a booking that read the balance inside its
// transaction used to wait on itself forever.
type videoWallet struct {
	balance int64
	pool    *pgxpool.Pool
}

func (w *videoWallet) MeteredBalance(ctx context.Context, org int64, _ string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); err != nil {
		return 0, err
	}
	return w.balance, nil
}
func (w *videoWallet) MeterUsage(context.Context, int64, string, int64, string, string) (int64, int64, error) {
	return 0, 0, nil
}

func wireVideo(t *testing.T, f *fixture) (*videorooms.Service, *videoWallet) {
	t.Helper()
	q := db.New(f.pool)
	if _, err := f.pool.Exec(context.Background(), "UPDATE users SET status='active' WHERE id=ANY($1::bigint[])", f.hostID); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	t.Cleanup(server.Close)
	box, err := crypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	wallet := &videoWallet{balance: 500, pool: f.pool}
	v := &videorooms.Service{
		Q: q, Box: box, Signer: videorooms.NewSigner("test-video"), Wallet: wallet, Entitlements: videoEnt{enabled: true},
		LiveKit:     livekit.New(livekit.Config{URL: "wss://video.example.test", APIURL: server.URL, APIKey: "test", APISecret: "test-livekit-secret-0123456789abcdef"}),
		FrontendURL: "https://app.example.com", Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return f.now },
		AppointmentOpenBefore: 10 * time.Minute, AppointmentGrace: 15 * time.Minute,
	}
	policy, err := v.PlatformPolicy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	policy.Enabled = true
	if _, err = v.SavePlatformPolicy(context.Background(), f.hostID[0], policy); err != nil {
		t.Fatal(err)
	}
	pin := true
	if _, err = v.SaveOrgSettings(context.Background(), f.org, videorooms.OrgSettingsInput{Enabled: true, Policy: videorooms.Partial{RequirePIN: &pin}}); err != nil {
		t.Fatal(err)
	}
	f.svc = usecase.New(usecase.Deps{Pool: f.pool, Queries: q, Entitlements: allowAll{}, Notifier: f.notes, Secret: "s3cret", BaseURL: "https://app.example.com", Video: v, Activity: activity.NewRecorder(q, nil)})
	f.svc.SetClock(func() time.Time { return f.now })
	return v, wallet
}

func TestVideoBookingSourcesLinksAndReminders(t *testing.T) {
	for _, source := range []string{usecase.SourceInternal, usecase.SourcePublic, usecase.SourceWidget} {
		t.Run(source, func(t *testing.T) {
			f := setup(t)
			v, _ := wireVideo(t, f)
			ctx := context.Background()
			typ := f.demoType(t, usecase.HostAgents, 0)
			start := f.now.Add(2 * time.Hour)
			var a usecase.Appointment
			if source == usecase.SourceInternal {
				var err error
				a, err = f.svc.Book(ctx, f.org, f.hostID[0], true, usecase.BookInput{TypeUUID: typ.UUID, StartsAt: start, InviteeName: "Guest", InviteeEmail: "guest@example.test"}, nil)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				public, err := f.svc.PublicBook(ctx, f.slug, typ.Slug, usecase.PublicBookInput{StartsAt: start, Name: "Guest", Email: "guest@example.test", Source: source})
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(public)
				if public.Video == nil || public.Video.PIN == "" || strings.Contains(string(encoded), "host_link") {
					t.Fatalf("public projection %s", encoded)
				}
				a, err = f.svc.Get(ctx, f.org, public.UUID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if a.VideoRoomUUID == nil || a.Video == nil || a.Video.GuestLink == "" || a.Video.HostLink == "" || len(a.Video.PIN) != 6 {
				t.Fatalf("video %+v", a)
			}
			room, err := v.Room(ctx, f.org, *a.VideoRoomUUID)
			if err != nil {
				t.Fatal(err)
			}
			if !room.ScheduledStartAt.Equal(start.Add(-10*time.Minute)) || !room.ScheduledEndAt.Equal(a.EndsAt.Add(15*time.Minute)) {
				t.Fatalf("window %+v", room)
			}
			code := strings.TrimPrefix(a.Video.GuestLink, "https://app.example.com/r/")
			_, _, signed, err := v.PublicRoom(ctx, code)
			if err != nil || signed.Role != videorooms.LinkGuest {
				t.Fatalf("signed link %v", err)
			}
			if _, err = v.GuestJoin(ctx, code, videorooms.GuestJoinInput{Name: "Guest", PIN: a.Video.PIN}); err == nil {
				t.Fatal("early join allowed")
			}
			ics, err := f.svc.ICSFor(ctx, f.org, a.UUID)
			if err != nil {
				t.Fatal(err)
			}
			unfolded := strings.ReplaceAll(string(ics), "\r\n ", "")
			if !strings.Contains(unfolded, a.Video.GuestLink) || !strings.Contains(unfolded, "PIN: "+a.Video.PIN) || strings.Contains(unfolded, a.Video.HostLink) {
				t.Fatalf("ICS %s", unfolded)
			}
			found := false
			for _, n := range f.notes.sent {
				if n.SourceEvent == "appointment.booked" && n.Recipient != nil {
					found = strings.Contains(n.Body, a.Video.GuestLink) && strings.Contains(n.Body, a.Video.PIN)
				}
			}
			if !found {
				t.Fatal("confirmation missing link/PIN")
			}
			// The calendar contains the authenticated member link.
			ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: f.org, Permissions: []string{rbac.PermTenantAppointmentsRead}})
			feed, err := f.svc.GetFeed(ctx, f.org, "2026-11-02", "2026-11-03", usecase.FeedFilter{Layers: []string{usecase.LayerAppointments}})
			if err != nil || len(feed.Items) != 1 || feed.Items[0].Appointment.Video.HostLink == "" {
				t.Fatalf("feed %v %v", feed, err)
			}
			f.now = start.Add(-time.Hour)
			if _, err = f.svc.SweepReminders(ctx); err != nil {
				t.Fatal(err)
			}
			found = false
			for _, n := range f.notes.sent {
				if n.SourceEvent == "appointment.reminder" && n.Recipient != nil {
					found = strings.Contains(n.Body, a.Video.GuestLink) && strings.Contains(n.Body, a.Video.PIN)
				}
			}
			if !found {
				t.Fatal("reminder missing link/PIN")
			}
		})
	}
}

func TestVideoFallbackStillBooksAndAudits(t *testing.T) {
	for _, reason := range []string{"org_disabled", "no_credit", "plan_disabled", "unconfigured", "guest_disabled"} {
		t.Run(reason, func(t *testing.T) {
			f := setup(t)
			v, wallet := wireVideo(t, f)
			ctx := context.Background()
			typ := f.demoType(t, usecase.HostAgents, 0)
			switch reason {
			case "org_disabled":
				_, _ = v.SaveOrgSettings(ctx, f.org, videorooms.OrgSettingsInput{Enabled: false})
			case "no_credit":
				wallet.balance = 0
			case "plan_disabled":
				v.Entitlements = videoEnt{}
			case "unconfigured":
				v.LiveKit = livekit.New(livekit.Config{})
			case "guest_disabled":
				off := false
				_, _ = v.SaveOrgSettings(ctx, f.org, videorooms.OrgSettingsInput{Enabled: true, Policy: videorooms.Partial{AllowGuestJoin: &off}})
			}
			a, err := f.svc.Book(ctx, f.org, f.hostID[0], true, usecase.BookInput{TypeUUID: typ.UUID, StartsAt: f.now.Add(2 * time.Hour), InviteeName: "Guest", InviteeEmail: "guest@example.test"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if a.VideoRoomUUID != nil || a.VideoWarning == "" || a.LocationValue == "" {
				t.Fatalf("fallback %+v", a)
			}
			var n int
			if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM activity_events WHERE organization_id=$1 AND action='appointments.updated' AND payload->>'event'='appointment.video_unavailable'", f.org).Scan(&n); err != nil || n != 1 {
				t.Fatalf("audit %d %v", n, err)
			}
		})
	}
}

func TestVideoMoveCancelAndAttendance(t *testing.T) {
	for _, joined := range []string{"both", "host", "guest", "neither", "lobby"} {
		t.Run(joined, func(t *testing.T) {
			f := setup(t)
			v, _ := wireVideo(t, f)
			ctx := context.Background()
			typ := f.demoType(t, usecase.HostAgents, 0)
			public, err := f.svc.PublicBook(ctx, f.slug, typ.Slug, usecase.PublicBookInput{StartsAt: f.now.Add(2 * time.Hour), Name: "Guest", Email: "guest@example.test"})
			if err != nil {
				t.Fatal(err)
			}
			a, err := f.svc.Get(ctx, f.org, public.UUID)
			if err != nil {
				t.Fatal(err)
			}
			moved, err := f.svc.PublicReschedule(ctx, public.Token, usecase.PublicRescheduleInput{StartsAt: f.now.Add(3 * time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			room, err := v.Room(ctx, f.org, *a.VideoRoomUUID)
			if err != nil {
				t.Fatal(err)
			}
			if !room.ScheduledStartAt.Equal(moved.StartsAt.Add(-10 * time.Minute)) {
				t.Fatal("move did not update room")
			}
			q := db.New(f.pool)
			r, err := q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: f.org, Uuid: room.UUID})
			if err != nil {
				t.Fatal(err)
			}
			for _, role := range []string{"host", "guest"} {
				identity := role + ":" + uuid.NewString()
				_, err = q.VideoCreateParticipant(ctx, db.VideoCreateParticipantParams{OrganizationID: f.org, RoomID: r.ID, Identity: identity, Role: role, DisplayName: role, Status: "admitted", TicketHash: uuid.NewString(), AdmittedAt: pgtype.Timestamptz{Time: moved.StartsAt, Valid: true}})
				if err != nil {
					t.Fatal(err)
				}
				if joined == "both" || joined == role {
					err = q.VideoOpenSession(ctx, db.VideoOpenSessionParams{OrganizationID: f.org, RoomID: r.ID, Identity: identity, LivekitSid: uuid.NewString(), JoinedAt: pgtype.Timestamptz{Time: moved.StartsAt, Valid: true}})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			f.now = moved.EndsAt.Add(16 * time.Minute)
			if err = f.svc.SweepVideoOutcomes(ctx); err != nil {
				t.Fatal(err)
			}
			a, err = f.svc.Get(ctx, f.org, a.UUID)
			if err != nil {
				t.Fatal(err)
			}
			want := usecase.StatusNoShow
			if joined == "both" {
				want = usecase.StatusCompleted
			}
			if a.Status != want {
				t.Fatalf("%s got %s want %s", joined, a.Status, want)
			}
		})
	}
	t.Run("cancel", func(t *testing.T) {
		f := setup(t)
		v, _ := wireVideo(t, f)
		ctx := context.Background()
		typ := f.demoType(t, usecase.HostAgents, 0)
		a, err := f.svc.Book(ctx, f.org, f.hostID[0], true, usecase.BookInput{TypeUUID: typ.UUID, StartsAt: f.now.Add(2 * time.Hour)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.svc.Cancel(ctx, f.org, f.hostID[0], a.UUID, "cancel"); err != nil {
			t.Fatal(err)
		}
		room, err := v.Room(ctx, f.org, *a.VideoRoomUUID)
		if err != nil || room.Status != "ended" {
			t.Fatalf("cancel %v %v", room, err)
		}
	})
}

func TestVideoCalendarMoveRollbackAndPublicCancel(t *testing.T) {
	f := setup(t)
	v, _ := wireVideo(t, f)
	ctx := context.Background()
	typ := f.demoType(t, usecase.HostAgents, 0)
	a, err := f.svc.Book(ctx, f.org, f.hostID[0], true, usecase.BookInput{TypeUUID: typ.UUID, StartsAt: f.now.Add(2 * time.Hour)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: f.org, Permissions: []string{rbac.PermTenantAppointmentsRead, rbac.PermTenantAppointmentsWrite}})
	start := a.StartsAt.Add(time.Hour)
	moved, err := f.svc.Move(ctx, f.org, f.hostID[0], usecase.MoveInput{Layer: usecase.LayerAppointments, RefUUID: a.UUID, StartsAt: start, EndsAt: start.Add(time.Hour), HostUUID: &f.hosts[1]})
	if err != nil {
		t.Fatal(err)
	}
	room, err := db.New(f.pool).VideoRoom(ctx, db.VideoRoomParams{OrganizationID: f.org, Uuid: *a.VideoRoomUUID})
	if err != nil {
		t.Fatal(err)
	}
	if !room.ScheduledStartAt.Time.Equal(start.Add(-10*time.Minute)) || room.HostUserID.Int64 != f.hostID[1] || room.MaxDurationMinutes != 85 {
		t.Fatalf("moved room %+v", room)
	}
	if err = db.New(f.pool).VideoStartRoom(ctx, db.VideoStartRoomParams{OrganizationID: f.org, ID: room.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Reschedule(ctx, f.org, f.hostID[0], true, a.UUID, usecase.RescheduleInput{StartsAt: start.Add(2 * time.Hour)}); err == nil {
		t.Fatal("live room moved")
	}
	unchanged, err := f.svc.Get(ctx, f.org, a.UUID)
	if err != nil || !unchanged.StartsAt.Equal(moved.StartsAt) {
		t.Fatalf("failed move must roll back %v %v", unchanged, err)
	}
	// Public cancellation also closes a live room and removes all links.
	token := strings.TrimPrefix(unchanged.Links.Manage, "https://app.example.com/book/manage/")
	cancelled, err := f.svc.PublicCancel(ctx, token, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != usecase.StatusCancelled || cancelled.Video.GuestLink != "" {
		t.Fatalf("cancelled %+v", cancelled)
	}
	roomAfter, err := v.Room(ctx, f.org, room.Uuid)
	if err != nil || roomAfter.Status != "ended" {
		t.Fatalf("room %v %v", roomAfter, err)
	}
}

func TestVideoWindowPINAndTenantIsolation(t *testing.T) {
	f := setup(t)
	v, _ := wireVideo(t, f)
	ctx := context.Background()
	typ := f.demoType(t, usecase.HostAgents, 0)
	a, err := f.svc.Book(ctx, f.org, f.hostID[0], true, usecase.BookInput{TypeUUID: typ.UUID, StartsAt: f.now.Add(2 * time.Hour)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.now = a.StartsAt
	if _, err = f.svc.SetStatus(ctx, f.org, f.hostID[0], a.UUID, usecase.StatusInput{Status: usecase.StatusCompleted}); err == nil {
		t.Fatal("completed without participation")
	}
	if _, err = f.svc.SetStatus(ctx, f.org, f.hostID[0], a.UUID, usecase.StatusInput{Status: usecase.StatusNoShow}); err == nil {
		t.Fatal("early no-show")
	}
	code := strings.TrimPrefix(a.Video.GuestLink, "https://app.example.com/r/")
	if _, err = v.GuestJoin(ctx, code, videorooms.GuestJoinInput{Name: "Guest", PIN: "wrong"}); err == nil {
		t.Fatal("wrong PIN accepted")
	}
	grant, err := v.GuestJoin(ctx, code, videorooms.GuestJoinInput{Name: "Guest", PIN: a.Video.PIN})
	if err != nil || grant.Status != "waiting" {
		t.Fatalf("lobby %v %v", grant, err)
	}
	f.now = a.EndsAt.Add(15 * time.Minute)
	if _, err = v.Ticket(ctx, grant.Ticket); err == nil {
		t.Fatal("ticket usable past window")
	}
	if err = v.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	room, err := v.Room(ctx, f.org, *a.VideoRoomUUID)
	if err != nil || room.Status != "ended" {
		t.Fatalf("expired room %v %v", room, err)
	}
	if _, err = v.Room(ctx, f.org+999999, *a.VideoRoomUUID); err == nil {
		t.Fatal("cross-org room exposed")
	}
}

func TestVideoPublicAndWidgetFallback(t *testing.T) {
	for _, source := range []string{usecase.SourcePublic, usecase.SourceWidget} {
		t.Run(source, func(t *testing.T) {
			f := setup(t)
			_, wallet := wireVideo(t, f)
			wallet.balance = 0
			ctx := context.Background()
			typ := f.demoType(t, usecase.HostAgents, 0)
			a, err := f.svc.PublicBook(ctx, f.slug, typ.Slug, usecase.PublicBookInput{StartsAt: f.now.Add(2 * time.Hour), Name: "Guest", Email: "guest@example.test", Source: source})
			if err != nil || a.VideoWarning == "" || a.Video != nil {
				t.Fatalf("fallback %v %v", a, err)
			}
		})
	}
}
