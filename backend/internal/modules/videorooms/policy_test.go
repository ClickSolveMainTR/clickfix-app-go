package videorooms

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func base() Policy {
	return Policy{Enabled: true, MaxDurationMinutes: 120, MaxParticipants: 10, AllowCamera: true, AllowScreenShare: true, AllowChat: true,
		AllowRecording: true, AllowGuestJoin: true, RecordingRetentionHours: 24, MaxConcurrentRecordings: 2, MaxRecordingMinutes: 60, LowBalanceMinutes: 60}
}

func ptr[T any](v T) *T { return &v }

func TestPlanOverrideCanLoosenOrgCanOnlyTighten(t *testing.T) {
	p := base().Override(Partial{MaxParticipants: ptr[int32](50), AllowRecording: ptr(false)})
	if p.MaxParticipants != 50 || p.AllowRecording {
		t.Fatalf("override not applied: %+v", p)
	}
	org := p.Tighten(Partial{MaxParticipants: ptr[int32](200), AllowRecording: ptr(true), AllowCamera: ptr(false), RequirePIN: ptr(true), MaxDurationMinutes: ptr[int32](30)})
	if org.MaxParticipants != 50 || org.AllowRecording || org.AllowCamera || !org.RequirePIN || org.MaxDurationMinutes != 30 {
		t.Fatalf("tighten loosened or ignored: %+v", org)
	}
	if e2ee := base().Tighten(Partial{E2EERequired: ptr(true)}); e2ee.AllowRecording {
		t.Fatal("E2EE-required policy must refuse recording")
	}
}

func TestResolveRecordingTurnsE2EEOff(t *testing.T) {
	s, err := base().Resolve(RoomOptions{Record: true})
	if err != nil || !s.Recording || s.E2EE {
		t.Fatalf("recording room: %+v %v", s, err)
	}
	s, err = base().Resolve(RoomOptions{})
	if err != nil || s.Recording || !s.E2EE || !s.Lobby {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	p := base()
	p.E2EERequired = true
	if _, err := p.Resolve(RoomOptions{Record: true}); !errors.Is(err, ErrRecordingNotAllowed) {
		t.Fatalf("want ErrRecordingNotAllowed, got %v", err)
	}
	p = base()
	p.RequirePIN, p.RequireLobby, p.AllowGuestJoin, p.AllowCamera = true, true, false, false
	if _, err := p.Resolve(RoomOptions{}); !errors.Is(err, ErrPINRequired) {
		t.Fatalf("pin required: %v", err)
	}
	s, _ = p.Resolve(RoomOptions{PIN: "1234", Lobby: ptr(false), GuestJoin: ptr(true), AllowCamera: ptr(true), MaxParticipants: ptr[int32](99)})
	if !s.Lobby || s.GuestJoin || s.AllowCamera || s.MaxParticipants != 10 {
		t.Fatalf("policy not enforced: %+v", s)
	}
}

func TestPolicyValidation(t *testing.T) {
	if !base().Valid() {
		t.Fatal("base invalid")
	}
	bad := base()
	bad.MaxParticipants = 1
	if bad.Valid() || (Partial{MaxDurationMinutes: ptr[int32](0)}).Valid() {
		t.Fatal("out-of-range accepted")
	}
}

func TestSignedLinks(t *testing.T) {
	s := NewSigner("secret")
	room := uuid.New()
	now := time.Now()
	code, err := s.Sign(Link{Room: room, Role: LinkGuest, ExpiresAt: now.Add(time.Hour)})
	if err != nil || len(code) != 51 || strings.ContainsAny(code, "+/=") {
		t.Fatalf("code %q %v", code, err)
	}
	l, err := s.Verify(code, now)
	if err != nil || l.Room != room || l.Role != LinkGuest {
		t.Fatalf("verify %+v %v", l, err)
	}
	if _, err := s.Verify(code, now.Add(2*time.Hour)); !errors.Is(err, ErrLinkExpired) {
		t.Fatalf("expiry: %v", err)
	}
	if _, err := NewSigner("other").Verify(code, now); !errors.Is(err, ErrLinkInvalid) {
		t.Fatalf("foreign key accepted: %v", err)
	}
	tampered := []byte(code)
	tampered[10] ^= 1
	if _, err := s.Verify(string(tampered), now); err == nil {
		t.Fatal("tampered code accepted")
	}
	if _, err := s.Sign(Link{Room: room, Role: "admin", ExpiresAt: now}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown role signed")
	}
}
