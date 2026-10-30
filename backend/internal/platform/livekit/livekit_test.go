package livekit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func testClient(apiURL string) *Client {
	return New(Config{URL: "wss://video.example.com", APIURL: apiURL, APIKey: "key", APISecret: "secret-secret-secret-secret-1234"})
}

func TestJoinTokenClaims(t *testing.T) {
	c := testClient("http://lk")
	no := false
	raw, err := c.JoinToken("g:1", "Ayşe", `{"role":"guest"}`, VideoGrant{Room: "cf-room", CanPublishData: &no, CanPublishSources: []string{SourceMicrophone}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var cl claims
	if _, err := jwt.ParseWithClaims(raw, &cl, func(*jwt.Token) (any, error) { return []byte("secret-secret-secret-secret-1234"), nil }); err != nil {
		t.Fatal(err)
	}
	if cl.Issuer != "key" || cl.Subject != "g:1" || cl.Name != "Ayşe" || !cl.Video.RoomJoin || cl.Video.Room != "cf-room" || *cl.Video.CanPublishData || len(cl.Video.CanPublishSources) != 1 {
		t.Fatalf("unexpected claims %+v %+v", cl, cl.Video)
	}
	if _, err := New(Config{}).JoinToken("a", "", "", VideoGrant{Room: "r"}, 0); err != ErrNotConfigured {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func signedWebhook(t *testing.T, body, secret string) *http.Request {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	cl := claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: "key", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}, Sha256: base64.StdEncoding.EncodeToString(sum[:])}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, cl).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Authorization", tok)
	return r
}

func TestVerifyWebhook(t *testing.T) {
	c := testClient("http://lk")
	body := `{"event":"participant_joined","id":"EV_1","room":{"name":"cf-x"},"participant":{"sid":"PA_1","identity":"g:1","joinedAt":"1700000000"}}`
	got, err := c.VerifyWebhook(signedWebhook(t, body, "secret-secret-secret-secret-1234"))
	if err != nil || string(got) != body {
		t.Fatalf("verify: %v", err)
	}
	if _, err := c.VerifyWebhook(signedWebhook(t, body, "wrong-secret-wrong-secret-123456")); err != ErrBadSignature {
		t.Fatalf("wrong secret accepted: %v", err)
	}
	r := signedWebhook(t, body, "secret-secret-secret-secret-1234")
	r.Body = io.NopCloser(strings.NewReader(body + " "))
	if _, err := c.VerifyWebhook(r); err != ErrBadSignature {
		t.Fatalf("tampered body accepted: %v", err)
	}
	noExp, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": "key", "sha256": base64.StdEncoding.EncodeToString(func() []byte { s := sha256.Sum256([]byte(body)); return s[:] }())}).SignedString([]byte("secret-secret-secret-secret-1234"))
	r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Authorization", noExp)
	if _, err := c.VerifyWebhook(r); err != ErrBadSignature {
		t.Fatalf("token without exp accepted: %v", err)
	}
	var ev Event
	if err := Decode(got, &ev); err != nil || ev.Participant.JoinedAt != 1700000000 || ev.Room.Name != "cf-x" {
		t.Fatalf("decode: %v %+v", err, ev)
	}
}

func TestDecodeSnakeCaseAndEnumNumbers(t *testing.T) {
	var ev Event
	err := Decode([]byte(`{"event":"egress_ended","egress_info":{"egress_id":"EG_1","status":3,"file_results":[{"filename":"a.mp4","size":42,"duration":"61000000000"}]}}`), &ev)
	if err != nil || ev.EgressInfo.EgressID != "EG_1" || ev.EgressInfo.Status != "EGRESS_COMPLETE" || ev.EgressInfo.FileResults[0].Size != 42 || ev.EgressInfo.FileResults[0].Duration != 61000000000 {
		t.Fatalf("decode: %v %+v", err, ev.EgressInfo)
	}
}

func TestTwirpCalls(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing bearer")
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch {
		case strings.HasSuffix(r.URL.Path, "StartRoomCompositeEgress"):
			_, _ = w.Write([]byte(`{"egressId":"EG_9"}`))
		case strings.HasSuffix(r.URL.Path, "DeleteRoom"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","msg":"room not found"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	c := testClient(srv.URL)
	ctx := context.Background()
	if err := c.CreateRoom(ctx, CreateRoomInput{Name: "cf-1", MaxParticipants: 4}); err != nil {
		t.Fatal(err)
	}
	id, err := c.StartRecording(ctx, "cf-1", "video/x.mp4", S3Output{Bucket: "b", AccessKey: "a", Secret: "s"})
	if err != nil || id != "EG_9" {
		t.Fatalf("egress: %v %q", err, id)
	}
	if err := c.DeleteRoom(ctx, "cf-1"); err != nil {
		t.Fatalf("missing room must not fail: %v", err)
	}
	if len(paths) != 3 || paths[0] != "/twirp/livekit.RoomService/CreateRoom" || paths[1] != "/twirp/livekit.Egress/StartRoomCompositeEgress" {
		t.Fatalf("paths %v", paths)
	}
}
