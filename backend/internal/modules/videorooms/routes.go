package videorooms

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Limiter is the shared Redis rate limiter (public join, PIN attempts).
type Limiter func(r *http.Request, action, subject string, limit int, window time.Duration) bool

type routes struct {
	rt       supportops.Router
	s        *Service
	activity *activity.Recorder
	limit    Limiter
}

// RegisterRoutes mounts tenant, platform and public video routes.
func RegisterRoutes(rt supportops.Router, s *Service, limit Limiter) {
	h := routes{rt: rt, s: s, activity: rt.Activity, limit: limit}
	h.tenant()
	h.platform()
	h.public()
}

type endpoint func(*http.Request) (any, error)

// handle writes the envelope and records the audit action for writes.
func (h routes) handle(action string, fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		out, err := fn(r)
		if err != nil {
			fail(w, r, err)
			return
		}
		if action != "" && h.activity != nil {
			actor := supportops.Actor(r.Context())
			payload := map[string]any{}
			if scope, ok := orgctx.From(r.Context()); ok {
				payload["organization_id"] = scope.ID
			}
			var id *uuid.UUID
			if v := supportops.ID(r); v != uuid.Nil {
				id = &v
			}
			h.activity.Record(r.Context(), &actor, action, "video_room", id, payload, r)
		}
		response.JSON(w, r, http.StatusOK, out)
	}
}

func (h routes) tenantRoute(pattern, perm, action string, fn endpoint) {
	if h.rt.Resolver == nil {
		return
	}
	h.rt.Mux.Handle(pattern, middleware.Chain(h.handle(action, fn), middleware.Authenticate(h.rt.Tokens, h.rt.Loader),
		middleware.RequireOrganization(h.rt.Resolver), middleware.RequireTenantPermission(perm)))
}

func org(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func manager(r *http.Request) bool {
	return orgctx.Must(r.Context()).Has(rbac.PermTenantVideoManage)
}

// hostOrManager guards end / remove / recording: the room host or a video manager.
func (h routes) hostOrManager(r *http.Request) error {
	if manager(r) {
		return nil
	}
	room, err := h.s.Q.VideoRoom(r.Context(), db.VideoRoomParams{OrganizationID: org(r), Uuid: supportops.ID(r)})
	if err != nil {
		return err
	}
	if !room.HostUserID.Valid || room.HostUserID.Int64 != supportops.Actor(r.Context()) {
		return ErrDenied
	}
	return nil
}

func page(r *http.Request) (int32, int32, error) {
	q, err := supportops.List(r)
	return q.Limit, q.Offset, err
}

func (h routes) tenant() {
	use, manage := rbac.PermTenantVideoUse, rbac.PermTenantVideoManage
	h.tenantRoute("GET /v1/tenant/video/availability", use, "", func(r *http.Request) (any, error) {
		return h.s.Availability(r.Context(), org(r))
	})
	h.tenantRoute("GET /v1/tenant/video/rooms", use, "", func(r *http.Request) (any, error) {
		limit, offset, err := page(r)
		if err != nil {
			return nil, err
		}
		var status *string
		if v := r.URL.Query().Get("status"); v != "" {
			if v != "open" && v != "live" && v != "ended" {
				return nil, ErrInvalid
			}
			status = &v
		}
		rows, n, err := h.s.Rooms(r.Context(), org(r), status, limit, offset)
		return map[string]any{"items": rows, "total": n, "limit": limit, "offset": offset}, err
	})
	h.tenantRoute("POST /v1/tenant/video/rooms", use, "video.room_created", func(r *http.Request) (any, error) {
		in, err := supportops.Read[CreateRoomInput](r)
		if err != nil {
			return nil, err
		}
		return h.s.CreateRoom(r.Context(), org(r), supportops.Actor(r.Context()), in)
	})
	h.tenantRoute("GET /v1/tenant/video/rooms/{uuid}", use, "", func(r *http.Request) (any, error) {
		return h.s.Room(r.Context(), org(r), supportops.ID(r))
	})
	h.tenantRoute("POST /v1/tenant/video/rooms/{uuid}/join", use, "video.room_joined", func(r *http.Request) (any, error) {
		user, err := h.s.Q.GetUserByID(r.Context(), supportops.Actor(r.Context()))
		if err != nil {
			return nil, err
		}
		return h.s.JoinAsMember(r.Context(), org(r), user, supportops.ID(r), manager(r))
	})
	h.tenantRoute("POST /v1/tenant/video/rooms/{uuid}/end", use, "video.room_ended", func(r *http.Request) (any, error) {
		if err := h.hostOrManager(r); err != nil {
			return nil, err
		}
		return h.s.EndRoom(r.Context(), org(r), supportops.ID(r))
	})
	h.tenantRoute("POST /v1/tenant/video/rooms/{uuid}/links", use, "video.link_created", func(r *http.Request) (any, error) {
		in, err := supportops.Read[LinkInput](r)
		if err != nil {
			return nil, err
		}
		return h.s.Link(r.Context(), org(r), supportops.ID(r), in)
	})
	h.tenantRoute("POST /v1/tenant/video/rooms/{uuid}/invite", use, "video.guest_invited", func(r *http.Request) (any, error) {
		in, err := supportops.Read[InviteInput](r)
		if err != nil {
			return nil, err
		}
		return h.s.Invite(r.Context(), org(r), supportops.ID(r), in)
	})
	h.tenantRoute("GET /v1/tenant/video/rooms/{uuid}/participants", use, "", func(r *http.Request) (any, error) {
		rows, err := h.s.Participants(r.Context(), org(r), supportops.ID(r))
		return map[string]any{"items": rows}, err
	})
	for _, action := range []string{"admit", "deny", "remove"} {
		h.tenantRoute("POST /v1/tenant/video/rooms/{uuid}/participants/{participant}/"+action, use, "video.participant_"+action, func(r *http.Request) (any, error) {
			if action == "remove" {
				if err := h.hostOrManager(r); err != nil {
					return nil, err
				}
			}
			p, err := uuid.Parse(r.PathValue("participant"))
			if err != nil {
				return nil, ErrNotFound
			}
			return h.s.SetParticipant(r.Context(), org(r), supportops.Actor(r.Context()), supportops.ID(r), p, action)
		})
	}
	h.tenantRoute("POST /v1/tenant/video/rooms/{uuid}/recording/start", use, "video.recording_started", func(r *http.Request) (any, error) {
		if err := h.hostOrManager(r); err != nil {
			return nil, err
		}
		return h.s.StartRecording(r.Context(), org(r), supportops.Actor(r.Context()), supportops.ID(r))
	})
	h.tenantRoute("POST /v1/tenant/video/rooms/{uuid}/recording/stop", use, "video.recording_stopped", func(r *http.Request) (any, error) {
		if err := h.hostOrManager(r); err != nil {
			return nil, err
		}
		return map[string]bool{"stopping": true}, h.s.StopRecording(r.Context(), org(r), supportops.ID(r))
	})
	h.tenantRoute("PUT /v1/tenant/video/appointments/{appointment}/room", use, "video.appointment_room", func(r *http.Request) (any, error) {
		id, err := uuid.Parse(r.PathValue("appointment"))
		if err != nil {
			return nil, ErrInvalid
		}
		in, err := supportops.Read[AppointmentRoomInput](r)
		if err != nil {
			return nil, err
		}
		return h.s.RoomForAppointment(r.Context(), org(r), supportops.Actor(r.Context()), id, in)
	})
	h.tenantRoute("GET /v1/tenant/video/settings", manage, "", func(r *http.Request) (any, error) {
		return h.s.OrgSettings(r.Context(), org(r))
	})
	h.tenantRoute("PUT /v1/tenant/video/settings", manage, "video.settings_updated", func(r *http.Request) (any, error) {
		in, err := supportops.Read[OrgSettingsInput](r)
		if err != nil {
			return nil, err
		}
		return h.s.SaveOrgSettings(r.Context(), org(r), in)
	})
	h.tenantRoute("GET /v1/tenant/video/recordings", manage, "", func(r *http.Request) (any, error) {
		limit, offset, err := page(r)
		if err != nil {
			return nil, err
		}
		var room *uuid.UUID
		if v := r.URL.Query().Get("room"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return nil, ErrInvalid
			}
			room = &id
		}
		rows, n, err := h.s.Recordings(r.Context(), org(r), room, limit, offset)
		return map[string]any{"items": rows, "total": n, "limit": limit, "offset": offset}, err
	})
	h.tenantRoute("DELETE /v1/tenant/video/recordings/{uuid}", manage, "video.recording_deleted", func(r *http.Request) (any, error) {
		return map[string]bool{"deleted": true}, h.s.DeleteRecording(r.Context(), org(r), supportops.ID(r))
	})
	if h.rt.Resolver != nil {
		h.rt.Mux.Handle("GET /v1/tenant/video/recordings/{uuid}/download", middleware.Chain(http.HandlerFunc(h.download), middleware.Authenticate(h.rt.Tokens, h.rt.Loader),
			middleware.RequireOrganization(h.rt.Resolver), middleware.RequireTenantPermission(manage)))
	}
}

func (h routes) download(w http.ResponseWriter, r *http.Request) {
	body, size, name, err := h.s.OpenRecording(r.Context(), org(r), supportops.ID(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()
	if h.activity != nil {
		actor, id := supportops.Actor(r.Context()), supportops.ID(r)
		h.activity.Record(r.Context(), &actor, "video.recording_downloaded", "video_recording", &id, map[string]any{"organization_id": org(r)}, r)
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	_, _ = io.Copy(w, body)
}

func (h routes) platformRoute(pattern, action string, fn endpoint) {
	h.rt.Mux.Handle(pattern, middleware.Chain(h.handle(action, fn), middleware.Authenticate(h.rt.Tokens, h.rt.Loader), middleware.RequirePermission(rbac.PermPlatformVideoManage)))
}

type platformView struct {
	Policy     Policy       `json:"policy"`
	Plans      []PlanPolicy `json:"plans"`
	Configured bool         `json:"configured"`
}

func (h routes) platform() {
	get := func(r *http.Request) (any, error) {
		p, err := h.s.PlatformPolicy(r.Context())
		if err != nil {
			return nil, err
		}
		plans, err := h.s.PlanPolicies(r.Context())
		return platformView{Policy: p, Plans: plans, Configured: h.s.LiveKit.Enabled()}, err
	}
	h.platformRoute("GET /v1/platform/video/settings", "", get)
	h.platformRoute("PUT /v1/platform/video/settings", "video.platform_settings_updated", func(r *http.Request) (any, error) {
		in, err := supportops.Read[PlatformSettingsInput](r)
		if err != nil {
			return nil, err
		}
		if _, err = h.s.SavePlatformPolicy(r.Context(), supportops.Actor(r.Context()), in.Policy); err != nil {
			return nil, err
		}
		return get(r)
	})
	h.platformRoute("PUT /v1/platform/video/plans/{uuid}", "video.plan_policy_updated", func(r *http.Request) (any, error) {
		in, err := supportops.Read[Partial](r)
		if err != nil {
			return nil, err
		}
		if err = h.s.SavePlanPolicy(r.Context(), supportops.ID(r), in); err != nil {
			return nil, err
		}
		return get(r)
	})
	h.platformRoute("DELETE /v1/platform/video/plans/{uuid}", "video.plan_policy_deleted", func(r *http.Request) (any, error) {
		if _, err := h.s.DeletePlanPolicy(r.Context(), supportops.ID(r)); err != nil {
			return nil, err
		}
		return get(r)
	})
}

// clientIP is the left-most X-Forwarded-For hop (set by the BFF from the
// edge proxy), else the remote address; same rule as the help center and
// QR sign-in limits. Behind the BFF the remote address is the BFF itself.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h routes) public() {
	public := func(fn endpoint) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			out, err := fn(r)
			if err != nil {
				fail(w, r, err)
				return
			}
			response.JSON(w, r, http.StatusOK, out)
		})
	}
	allow := func(r *http.Request, action string, n int) error {
		if h.limit != nil && !h.limit(r, action, clientIP(r), n, time.Minute) {
			return ErrRateLimited
		}
		return nil
	}
	h.rt.Mux.Handle("GET /v1/public/video/rooms/{code}", public(func(r *http.Request) (any, error) {
		if err := allow(r, "video_room_info", 60); err != nil {
			return nil, err
		}
		v, _, _, err := h.s.PublicRoom(r.Context(), r.PathValue("code"))
		return v, err
	}))
	h.rt.Mux.Handle("POST /v1/public/video/rooms/{code}/join", public(func(r *http.Request) (any, error) {
		// PIN guessing is bounded per address and per room.
		if err := allow(r, "video_guest_join", 10); err != nil {
			return nil, err
		}
		if h.limit != nil && !h.limit(r, "video_guest_join_room", r.PathValue("code"), 30, 10*time.Minute) {
			return nil, ErrRateLimited
		}
		in, err := supportops.Read[GuestJoinInput](r)
		if err != nil {
			return nil, err
		}
		return h.s.GuestJoin(r.Context(), r.PathValue("code"), in)
	}))
	h.rt.Mux.Handle("POST /v1/public/video/ticket", public(func(r *http.Request) (any, error) {
		if err := allow(r, "video_ticket", 120); err != nil {
			return nil, err
		}
		in, err := supportops.Read[TicketInput](r)
		if err != nil {
			return nil, err
		}
		return h.s.Ticket(r.Context(), in.Ticket)
	}))
	h.rt.Mux.Handle("POST /v1/public/video/webhook", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h.s.Webhook(r.Context(), r); err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, map[string]bool{"received": true})
	}))
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	var ent *entitlement.Error
	switch {
	case errors.As(err, &ent):
		if !entitlement.WriteError(w, r, err) {
			response.Forbidden(w, r, "Plan does not include video rooms")
		}
	case errors.Is(err, ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "Record not found")
	case errors.Is(err, ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: "Invalid input"}})
	case errors.Is(err, ErrDenied):
		response.Forbidden(w, r, "Access denied")
	case errors.Is(err, ErrLoginRequired):
		response.Error(w, r, http.StatusUnauthorized, "VIDEO_LOGIN_REQUIRED", "Sign in to join as an agent")
	case errors.Is(err, ErrDisabled):
		response.Error(w, r, http.StatusForbidden, "VIDEO_DISABLED", "Video rooms are disabled")
	case errors.Is(err, ErrRecorderUnavailable):
		response.ServiceUnavailable(w, r, "VIDEO_RECORDER_UNAVAILABLE", "Recording service is unavailable")
	case errors.Is(err, ErrNotConfigured), errors.Is(err, livekit.ErrNotConfigured):
		// A platform configuration state (GET /v1/tenant/video/availability
		// reports it), not a server fault.
		response.Conflict(w, r, "VIDEO_NOT_CONFIGURED", "Video server is not configured")
	case errors.Is(err, ErrNoMinutes):
		response.Error(w, r, http.StatusPaymentRequired, "VIDEO_MINUTES_EXHAUSTED", "No video minutes left")
	case errors.Is(err, ErrPINRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, "VIDEO_PIN_REQUIRED", "A PIN is required")
	case errors.Is(err, ErrPINInvalid):
		response.Error(w, r, http.StatusForbidden, "VIDEO_PIN_INVALID", "Wrong PIN")
	case errors.Is(err, ErrRecordingNotAllowed):
		response.Error(w, r, http.StatusForbidden, "VIDEO_RECORDING_NOT_ALLOWED", "Recording is not allowed for this room")
	case errors.Is(err, ErrRoomFull):
		response.Conflict(w, r, "VIDEO_ROOM_FULL", "The room is full")
	case errors.Is(err, ErrRoomEnded):
		response.Error(w, r, http.StatusGone, "VIDEO_ROOM_ENDED", "The room has ended")
	case errors.Is(err, ErrLinkInvalid):
		response.Error(w, r, http.StatusNotFound, "VIDEO_LINK_INVALID", "Invalid link")
	case errors.Is(err, ErrLinkExpired):
		response.Error(w, r, http.StatusGone, "VIDEO_LINK_EXPIRED", "The link has expired")
	case errors.Is(err, ErrRateLimited):
		response.TooManyRequests(w, r, "Too many attempts")
	case errors.Is(err, ErrConflict):
		response.Conflict(w, r, response.CodeConflict, "Conflicting state")
	case errors.Is(err, livekit.ErrBadSignature):
		response.Unauthorized(w, r, "Invalid signature")
	default:
		supportops.Fail(w, r, err)
	}
}
