package remote

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/livekit/protocol/livekit"
	protoLogger "github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
)

// ConnectTimeout, tek bir oda bağlantısının en uzun süresidir.
const ConnectTimeout = 15 * time.Second

// h264 izinin yeteneği (CF-235: Constrained Baseline, packetization-mode 1).
var h264 = webrtc.RTPCodecCapability{
	MimeType:    webrtc.MimeTypeH264,
	ClockRate:   90000,
	SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
}

type lkLink struct {
	room  *lksdk.Room
	track *lksdk.LocalTrack
	once  sync.Once
}

func (l *lkLink) WriteFrame(f capture.Frame, d time.Duration) error {
	return l.track.WriteSample(media.Sample{Data: f.Data, Duration: d}, nil)
}

func (l *lkLink) Close() { l.once.Do(l.room.Disconnect) }

// classify, oda kopma nedenini sınıflar. SDK bazı nedenleri (ROOM_DELETED)
// OtherReason'a topladığı için ham protokol değerine bakılır.
func classify(r livekit.DisconnectReason) disconnectKind {
	switch r {
	case livekit.DisconnectReason_ROOM_DELETED, livekit.DisconnectReason_ROOM_CLOSED,
		livekit.DisconnectReason_PARTICIPANT_REMOVED, livekit.DisconnectReason_DUPLICATE_IDENTITY:
		return dropClosed
	case livekit.DisconnectReason_CLIENT_INITIATED:
		return dropLocal
	}
	return dropTransient
}

func dialLiveKit(ctx context.Context, g api.Grant, cfg capture.Config, ev linkEvents) (link, []string, error) {
	track, err := lksdk.NewLocalSampleTrack(h264, lksdk.WithRTCPHandler(func(p rtcp.Packet) {
		switch p.(type) {
		case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
			ev.keyframe()
		}
	}))
	if err != nil {
		return nil, nil, fmt.Errorf("iz oluşturulamadı: %w", err)
	}
	var room *lksdk.Room
	var roomMu sync.Mutex
	cb := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnDataPacket: func(d lksdk.DataPacket, p lksdk.DataReceiveParams) {
				if u, ok := d.(*lksdk.UserDataPacket); ok {
					ev.data(p.SenderIdentity, u.Topic, u.Payload)
				}
			},
		},
		OnParticipantConnected:    func(p *lksdk.RemoteParticipant) { ev.participant(p.Identity(), true) },
		OnParticipantDisconnected: func(p *lksdk.RemoteParticipant) { ev.participant(p.Identity(), false) },
		OnReconnecting:            ev.reconnecting,
		OnReconnected:             ev.reconnected,
		OnDisconnected: func() {
			roomMu.Lock()
			r := room
			roomMu.Unlock()
			kind := dropTransient
			if r != nil {
				kind = classify(r.DisconnectReason())
			}
			ev.disconnected(kind)
		},
	}

	type result struct {
		room *lksdk.Room
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		r, err := lksdk.ConnectToRoomWithToken(g.ServerURL, g.Token, cb,
			lksdk.WithAutoSubscribe(false), lksdk.WithConnectTimeout(ConnectTimeout))
		ch <- result{r, err}
	}()
	var res result
	select {
	case res = <-ch:
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.room != nil {
				r.room.Disconnect()
			}
		}()
		return nil, nil, ctx.Err()
	}
	if res.err != nil {
		// SDK hatası adresi içerebilir; günlükte maskelenir.
		return nil, nil, fmt.Errorf("odaya bağlanılamadı: %w", res.err)
	}
	roomMu.Lock()
	room = res.room
	roomMu.Unlock()

	if _, err := room.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{
		Name: "screen", Source: livekit.TrackSource_SCREEN_SHARE, VideoWidth: cfg.Width, VideoHeight: cfg.Height,
	}); err != nil {
		room.Disconnect()
		return nil, nil, fmt.Errorf("iz yayınlanamadı: %w", err)
	}
	var present []string
	for _, p := range room.GetRemoteParticipants() {
		present = append(present, p.Identity())
	}
	return &lkLink{room: room, track: track}, present, nil
}

// SetLogger, LiveKit SDK'nın günlüklerini verilen slog işleyicisine
// yönlendirir. SDK varsayılan olarak log.Default()'a, yani maskelenmeyen
// stderr'e yazar; sidecar maskeleyen işleyiciyi burada verir.
func SetLogger(l *slog.Logger) {
	lksdk.SetLogger(protoLogger.LogRLogger(logr.FromSlogHandler(l.With("component", "livekit").Handler())))
}
