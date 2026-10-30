import JsSIP from "jssip";
import type { RTCSession } from "jssip/lib/RTCSession.js";
import type {
  CallSnapshot,
  SoftphoneEngine,
  SoftphoneEngineConfig,
  SoftphoneEvents,
} from "./engine";
import { sipTarget } from "./engine";

/** JsSIP `failed` event: `message` is the final SIP response (outgoing) or null (local/transport failures). */
interface SessionFailedEvent {
  cause?: string;
  originator?: string;
  message?: {
    status_code?: number;
    reason_phrase?: string;
    getHeader?: (name: string) => string | undefined;
  } | null;
}

export const microphonePermissionCause = "Microphone Permission";

/**
 * JsSIP implementation of the softphone engine: SIP over WebSocket registration, one call at a time
 * (a second incoming call is rejected with 486 Busy like the legacy widget), remote audio exposed as a
 * MediaStream for the page's <audio> element. UA/call options mirror the legacy `sipClient.js`
 * (display_name, user_agent, register_expires 600, STUN pcConfig, offerToReceiveAudio, mic permission first).
 */
export function createJsSipEngine(
  config: SoftphoneEngineConfig,
  events: SoftphoneEvents,
): SoftphoneEngine {
  const socket = new JsSIP.WebSocketInterface(config.websocketUrl);
  const ua = new JsSIP.UA({
    sockets: [socket],
    uri: `sip:${config.username}@${config.domain}`,
    password: config.password,
    display_name: config.displayName || "ClickFix",
    register: true,
    register_expires: 600,
    session_timers: false,
    user_agent: "ClickFix Softphone",
    connection_recovery_min_interval: 2,
    connection_recovery_max_interval: 30,
  });
  let session: RTCSession | null = null;
  let snapshot: CallSnapshot | null = null;
  let sequence = 0;
  let dialing = false;
  let disposed = false;

  const emit = (patch: Partial<CallSnapshot> | null) => {
    snapshot =
      patch === null || !snapshot
        ? (patch as CallSnapshot | null)
        : { ...snapshot, ...patch };
    events.onCall(snapshot);
  };

  const attachAudio = (current: RTCSession) => {
    const connection = current.connection;
    if (!connection) return;
    const publish = () => {
      const receivers = connection
        .getReceivers()
        .filter((receiver) => receiver.track?.kind === "audio");
      events.onRemoteStream(
        receivers.length
          ? new MediaStream(receivers.map((receiver) => receiver.track))
          : null,
      );
    };
    connection.addEventListener("track", publish);
    publish();
  };

  const track = (current: RTCSession, direction: CallSnapshot["direction"]) => {
    session = current;
    sequence += 1;
    const identity = current.remote_identity;
    emit({
      id: `call_${sequence}`,
      direction,
      remote: identity?.uri?.user ?? "",
      remoteName: identity?.display_name || null,
      phase: direction === "incoming" ? "ringing" : "connecting",
      held: false,
      muted: false,
      startedAt: null,
      endReason: null,
      statusCode: null,
      reasonHeader: null,
      error: false,
    });
    current.on("peerconnection", () => attachAudio(current));
    current.on("progress", () =>
      emit({ phase: direction === "incoming" ? "connecting" : "ringing" }),
    );
    current.on("confirmed", () => {
      attachAudio(current);
      emit({ phase: "active", startedAt: Date.now() });
    });
    current.on("hold", () => emit({ held: true }));
    current.on("unhold", () => emit({ held: false }));
    current.on("muted", () => emit({ muted: true }));
    current.on("unmuted", () => emit({ muted: false }));
    const finish = (event: SessionFailedEvent, error = false) => {
      if (session === current) session = null;
      const message = event.message ?? null;
      emit({
        phase: "ended",
        endReason: event.cause ?? null,
        statusCode:
          typeof message?.status_code === "number" ? message.status_code : null,
        reasonHeader: message?.getHeader?.("Reason") ?? null,
        error,
      });
      events.onRemoteStream(null);
    };
    current.on("ended", (event: SessionFailedEvent) => finish(event));
    current.on("failed", (event: SessionFailedEvent) =>
      finish(
        event,
        ![
          "Busy",
          "No Answer",
          "Canceled",
          "Rejected",
          "Not Found",
          "Unavailable",
        ].includes(event.cause ?? ""),
      ),
    );
  };

  ua.on("connecting", () => events.onRegistration("connecting"));
  ua.on("registered", () => events.onRegistration("registered"));
  ua.on("unregistered", () => events.onRegistration("disconnected"));
  ua.on("disconnected", () => events.onRegistration("disconnected"));
  ua.on("registrationFailed", (event: { cause?: string }) =>
    events.onRegistration("failed", event.cause),
  );
  ua.on(
    "newRTCSession",
    (event: { originator: string; session: RTCSession }) => {
      if (event.originator !== "remote") return;
      if (session && !session.isEnded()) {
        event.session.terminate({
          status_code: 486,
          reason_phrase: "Busy Here",
        });
        return;
      }
      track(event.session, "incoming");
    },
  );

  const mediaConstraints = { audio: true, video: false };
  const pcConfig = { iceServers: config.iceServers };
  const rtcOfferConstraints = {
    offerToReceiveAudio: true,
    offerToReceiveVideo: false,
  };

  const mediaDevices =
    typeof navigator !== "undefined" ? navigator.mediaDevices : undefined;

  return {
    start: () => ua.start(),
    stop: () => {
      disposed = true;
      session?.terminate();
      ua.stop();
    },
    call: (target) => {
      const uri = sipTarget(target, config.domain);
      if (disposed || !uri || dialing || (session && !session.isEnded()))
        return;
      const dial = () => {
        if (disposed) return;
        if (session && !session.isEnded()) return;
        track(
          ua.call(uri, { mediaConstraints, pcConfig, rtcOfferConstraints }),
          "outgoing",
        );
      };
      if (!mediaDevices?.getUserMedia) {
        dial();
        return;
      }
      // Legacy parity: ask for the microphone before the INVITE so a denied permission is a clear local
      // error ("Mikrofon izni gerekli") instead of a SIP failure after the PBX already rang the trunk.
      dialing = true;
      void mediaDevices
        .getUserMedia(mediaConstraints)
        .then((stream) => {
          stream.getTracks().forEach((item) => item.stop());
          dial();
        })
        .catch(() => {
          sequence += 1;
          emit({
            id: `call_${sequence}`,
            direction: "outgoing",
            remote: target.trim(),
            remoteName: null,
            phase: "ended",
            held: false,
            muted: false,
            startedAt: null,
            endReason: microphonePermissionCause,
            statusCode: null,
            reasonHeader: null,
            error: true,
          });
        })
        .finally(() => {
          dialing = false;
        });
    },
    answer: () => {
      if (
        session &&
        snapshot?.direction === "incoming" &&
        snapshot.phase === "ringing"
      )
        session.answer({ mediaConstraints, pcConfig });
    },
    hangup: () => {
      if (session && !session.isEnded()) session.terminate();
    },
    setHold: (held) => {
      if (!session || snapshot?.phase !== "active") return;
      if (held) session.hold();
      else session.unhold();
    },
    setMuted: (muted) => {
      if (!session) return;
      if (muted) session.mute({ audio: true });
      else session.unmute({ audio: true });
    },
    transfer: (target) => {
      const uri = sipTarget(target, config.domain);
      if (session && uri && snapshot?.phase === "active") session.refer(uri);
    },
    sendDtmf: (tone) => {
      if (session && snapshot?.phase === "active") session.sendDTMF(tone);
    },
  };
}
