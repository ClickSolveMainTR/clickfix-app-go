"use client";

import { useQuery } from "@tanstack/react-query";
import {
  ConnectionQuality,
  Room,
  RoomEvent,
  Track,
  type Participant,
  type RemoteTrack,
  type RemoteTrackPublication,
  type RemoteParticipant,
} from "livekit-client";
import {
  Hand,
  Loader2,
  Maximize,
  Minimize,
  MousePointer2,
  ScanSearch,
  ShieldAlert,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { controlAllowed, errorKey } from "../lib/access";
import {
  CONTROL_TOPIC,
  POINTER_MOVE_MAX_PER_SECOND,
  createSequencer,
  encodeEvent,
  isBlockedCombo,
  normalizePoint,
  wheelDelta,
  type ControlEventInput,
  type PointerButton,
} from "../lib/control";
import {
  remoteSupportService,
  type RemoteSupportSession,
} from "../services/remote-support.service";

type Fit = "fit" | "actual";
type Connection = "connecting" | "connected" | "reconnecting" | "disconnected";

const CUSTOMER_PREFIX = "rs:customer:";
/** Two Escape presses within this window give the keyboard back. */
const RELEASE_WINDOW_MS = 600;

/**
 * Watches the customer's screen share and, when every control gate is open
 * (permission, consent, live session, LiveKit data grant), sends input
 * events to the customer only. Losing any gate locks input immediately.
 */
export function RemoteViewer({
  slug,
  session,
  canControlPermission,
}: {
  slug: string;
  session: RemoteSupportSession;
  canControlPermission: boolean;
}) {
  const { t } = useLocale();
  const hintId = useId();
  const grant = useQuery({
    queryKey: ["remote-support", slug, "grant", session.uuid],
    queryFn: () => remoteSupportService.token(slug, session.uuid),
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
  });

  const wrap = useRef<HTMLDivElement | null>(null);
  const video = useRef<HTMLVideoElement | null>(null);
  const roomRef = useRef<Room | null>(null);
  const customer = useRef<string>(CUSTOMER_PREFIX + session.uuid);
  const [connection, setConnection] = useState<Connection>("connecting");
  const [quality, setQuality] = useState<ConnectionQuality>(
    ConnectionQuality.Unknown,
  );
  const [hasTrack, setHasTrack] = useState(false);
  const [dims, setDims] = useState<{ w: number; h: number } | null>(null);
  const [fit, setFit] = useState<Fit>("fit");
  const [fullscreen, setFullscreen] = useState(false);
  // LiveKit's live answer (permission updates) overrides the token's grant.
  const [publishOverride, setPublishOverride] = useState<boolean | null>(null);
  const canPublishData =
    publishOverride ?? grant.data?.can_publish_data ?? false;
  // The control consent the agent took control under: a withdrawn and
  // re-given consent needs a new press, input never resumes by itself.
  const [capturingFor, setCapturingFor] = useState<string | null>(null);

  const allowed = controlAllowed(
    { use: true, control: canControlPermission },
    session,
    canPublishData,
  );

  // ---- LiveKit connection ----
  useEffect(() => {
    const g = grant.data;
    if (!g?.token || !g.server_url) return;
    const room = new Room({ adaptiveStream: false, dynacast: false });
    roomRef.current = room;

    const attach = (
      track: RemoteTrack,
      _pub: RemoteTrackPublication,
      participant: RemoteParticipant,
    ) => {
      if (
        track.kind !== Track.Kind.Video ||
        track.source !== Track.Source.ScreenShare
      )
        return;
      customer.current = participant.identity;
      if (video.current) track.attach(video.current);
      setHasTrack(true);
    };
    const detach = (track: RemoteTrack) => {
      if (track.source !== Track.Source.ScreenShare) return;
      track.detach();
      setHasTrack(false);
      setDims(null);
    };
    const onPermissions = (_prev: unknown, participant: Participant) => {
      if (participant.identity === room.localParticipant.identity) {
        setPublishOverride(
          Boolean(room.localParticipant.permissions?.canPublishData),
        );
      }
    };
    const onQuality = (q: ConnectionQuality, participant: Participant) => {
      // The customer's uplink decides what the agent sees.
      if (participant.identity.startsWith(CUSTOMER_PREFIX)) setQuality(q);
    };

    room
      .on(RoomEvent.TrackSubscribed, attach)
      .on(RoomEvent.TrackUnsubscribed, detach)
      .on(RoomEvent.ParticipantPermissionsChanged, onPermissions)
      .on(RoomEvent.ConnectionQualityChanged, onQuality)
      .on(RoomEvent.Reconnecting, () => setConnection("reconnecting"))
      .on(RoomEvent.Reconnected, () => setConnection("connected"))
      .on(RoomEvent.Disconnected, () => setConnection("disconnected"));

    room
      .connect(g.server_url, g.token, { autoSubscribe: true })
      .then(() => {
        setConnection("connected");
        // A track published before we joined.
        for (const p of room.remoteParticipants.values()) {
          for (const pub of p.trackPublications.values()) {
            if (pub.track && pub.isSubscribed)
              attach(
                pub.track as RemoteTrack,
                pub as RemoteTrackPublication,
                p,
              );
          }
        }
      })
      .catch(() => setConnection("disconnected"));

    return () => {
      room.removeAllListeners();
      void room.disconnect();
      roomRef.current = null;
    };
  }, [grant.data]);

  // Track size (the shared screen's resolution) for normalization.
  useEffect(() => {
    const el = video.current;
    if (!el) return;
    const read = () =>
      el.videoWidth && el.videoHeight
        ? setDims({ w: el.videoWidth, h: el.videoHeight })
        : undefined;
    el.addEventListener("loadedmetadata", read);
    el.addEventListener("resize", read);
    return () => {
      el.removeEventListener("loadedmetadata", read);
      el.removeEventListener("resize", read);
    };
  }, []);

  // ---- fullscreen ----
  useEffect(() => {
    const on = () => setFullscreen(document.fullscreenElement === wrap.current);
    document.addEventListener("fullscreenchange", on);
    return () => document.removeEventListener("fullscreenchange", on);
  }, []);
  const toggleFullscreen = () => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void wrap.current?.requestFullscreen?.();
  };

  // ---- sending ----
  const stamp = useRef(createSequencer());
  const send = useCallback(
    (event: ControlEventInput) => {
      const room = roomRef.current;
      if (!room || !allowed) return;
      room.localParticipant
        .publishData(
          encodeEvent(stamp.current(event)) as Uint8Array<ArrayBuffer>,
          {
            reliable: true,
            topic: CONTROL_TOPIC,
            destinationIdentities: [customer.current],
          },
        )
        .catch(() => setPublishOverride(false));
    },
    [allowed],
  );

  // Losing any gate locks input at once (active turns false) and says why.
  const consentAt = session.control_consent_at ?? "";
  const active = allowed && capturingFor !== null && capturingFor === consentAt;
  const lockedNotice = capturingFor !== null && !active;

  const point = (clientX: number, clientY: number) => {
    const el = video.current;
    if (!el || !dims) return null;
    const r = el.getBoundingClientRect();
    return normalizePoint(
      clientX - r.left,
      clientY - r.top,
      r.width,
      r.height,
      dims.w,
      dims.h,
    );
  };

  // Pointer moves: throttled with a trailing send so the last position
  // always arrives (POINTER_MOVE_MAX_PER_SECOND).
  const pendingMove = useRef<{ x: number; y: number } | null>(null);
  const lastMove = useRef(0);
  const moveTimer = useRef(0);
  useEffect(() => () => window.clearTimeout(moveTimer.current), []);

  function handlePointerMove(e: ReactPointerEvent<HTMLVideoElement>) {
    if (!active) return;
    const p = point(e.clientX, e.clientY);
    if (!p) return;
    pendingMove.current = p;
    if (moveTimer.current) return;
    const interval = 1000 / POINTER_MOVE_MAX_PER_SECOND;
    const wait = Math.max(0, interval - (performance.now() - lastMove.current));
    moveTimer.current = window.setTimeout(() => {
      moveTimer.current = 0;
      lastMove.current = performance.now();
      const q = pendingMove.current;
      pendingMove.current = null;
      if (q) send({ type: "pointer.move", x: q.x, y: q.y });
    }, wait);
  }
  function handlePointerButton(
    type: "pointer.down" | "pointer.up",
    e: ReactPointerEvent<HTMLVideoElement>,
  ) {
    if (!active) return;
    const p = point(e.clientX, e.clientY);
    if (!p || e.button > 2) return;
    e.preventDefault();
    wrap.current?.focus();
    if (type === "pointer.down") e.currentTarget.setPointerCapture(e.pointerId);
    send({ type, x: p.x, y: p.y, button: e.button as PointerButton });
  }

  // Wheel needs a non-passive listener to keep the page from scrolling.
  useEffect(() => {
    const el = video.current;
    if (!el || !active) return;
    const onWheel = (e: WheelEvent) => {
      const p = point(e.clientX, e.clientY);
      if (!p) return;
      e.preventDefault();
      const d = wheelDelta(e.deltaX, e.deltaY, e.deltaMode);
      if (d.dx || d.dy) send({ type: "wheel", x: p.x, y: p.y, ...d });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
    // point() reads refs and dims only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, send, dims]);

  const lastEscape = useRef(0);
  const releaseKeyboard = () => {
    send({ type: "release" });
    setCapturingFor(null);
  };
  function handleKey(
    type: "key.down" | "key.up",
    e: ReactKeyboardEvent<HTMLDivElement>,
  ) {
    if (!active) return;
    if (type === "key.down" && e.code === "Escape") {
      const now = Date.now();
      if (now - lastEscape.current < RELEASE_WINDOW_MS) {
        e.preventDefault();
        releaseKeyboard();
        return;
      }
      lastEscape.current = now;
    }
    const mods = {
      alt: e.altKey,
      ctrl: e.ctrlKey,
      meta: e.metaKey,
      shift: e.shiftKey,
    };
    e.preventDefault();
    e.stopPropagation();
    if (!e.code || isBlockedCombo(e.code, mods)) return;
    send({
      type,
      code: e.code,
      key: e.key.length <= 32 ? e.key : "",
      mods,
      repeat: e.repeat,
    });
  }

  if (grant.isError) {
    return (
      <div
        className="flex items-center gap-2 rounded-lg border p-4 text-sm"
        role="alert"
      >
        <ShieldAlert className="text-destructive size-5" />
        {t(`remote_support.errors.${errorKey(grant.error)}`)}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div
        className="flex flex-wrap items-center gap-2"
        role="toolbar"
        aria-label={t("remote_support.viewer.toolbar")}
      >
        <div
          className="flex gap-1"
          role="radiogroup"
          aria-label={t("remote_support.viewer.size")}
        >
          {(["fit", "actual"] as const).map((f) => (
            <Button
              key={f}
              type="button"
              size="sm"
              role="radio"
              aria-checked={fit === f}
              variant={fit === f ? "secondary" : "ghost"}
              onClick={() => setFit(f)}
            >
              {f === "fit" ? (
                <ScanSearch className="size-4" />
              ) : (
                <MousePointer2 className="size-4" />
              )}
              {t(`remote_support.viewer.${f}`)}
            </Button>
          ))}
        </div>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={toggleFullscreen}
          aria-pressed={fullscreen}
        >
          {fullscreen ? (
            <Minimize className="size-4" />
          ) : (
            <Maximize className="size-4" />
          )}
          {t(
            fullscreen
              ? "remote_support.viewer.exit_fullscreen"
              : "remote_support.viewer.fullscreen",
          )}
        </Button>
        {session.mode_requested === "control" && canControlPermission ? (
          <Button
            type="button"
            size="sm"
            variant={active ? "default" : "outline"}
            disabled={!allowed && !lockedNotice}
            aria-pressed={active}
            onClick={() => {
              if (active || lockedNotice) releaseKeyboard();
              else {
                setCapturingFor(consentAt);
                wrap.current?.focus();
              }
            }}
          >
            <Hand className="size-4" />
            {t(
              active || lockedNotice
                ? "remote_support.viewer.control_stop"
                : "remote_support.viewer.control_start",
            )}
          </Button>
        ) : null}
        <span
          className="text-muted-foreground ml-auto flex items-center gap-1.5 text-xs"
          role="status"
        >
          <span
            aria-hidden
            className={cn(
              "size-2 rounded-full",
              connection !== "connected"
                ? "bg-amber-500"
                : quality === ConnectionQuality.Poor ||
                    quality === ConnectionQuality.Lost
                  ? "bg-red-500"
                  : "bg-emerald-500",
            )}
          />
          {connection === "connected"
            ? t("remote_support.viewer.quality", {
                quality: t(`remote_support.quality.${quality}`),
              })
            : t(`remote_support.connection.${connection}`)}
          {dims ? ` · ${dims.w}×${dims.h}` : ""}
        </span>
      </div>

      <div aria-live="polite" className="empty:hidden">
        {active ? (
          <p className="inline-flex items-center gap-1.5 rounded-md bg-emerald-600 px-2.5 py-1 text-xs font-medium text-white">
            <Hand className="size-3.5" />
            {t("remote_support.viewer.control_yours")}
          </p>
        ) : null}
        {lockedNotice ? (
          <p
            className="rounded-md border border-amber-500/40 bg-amber-500/5 px-2.5 py-1 text-xs"
            role="alert"
          >
            {t("remote_support.viewer.control_locked")}
          </p>
        ) : null}
      </div>

      <div
        ref={wrap}
        tabIndex={0}
        aria-label={t("remote_support.viewer.screen")}
        aria-describedby={hintId}
        onKeyDown={(e) => handleKey("key.down", e)}
        onKeyUp={(e) => handleKey("key.up", e)}
        onBlur={() => active && send({ type: "release" })}
        className={cn(
          "focus-visible:ring-ring relative rounded-lg border bg-black outline-none focus-visible:ring-2",
          fit === "fit" || fullscreen ? "overflow-hidden" : "overflow-auto",
          fullscreen ? "flex items-center justify-center" : "max-h-[70vh]",
          active && "ring-2 ring-emerald-500",
        )}
      >
        <video
          ref={video}
          autoPlay
          playsInline
          muted
          onPointerMove={handlePointerMove}
          onPointerDown={(e) => handlePointerButton("pointer.down", e)}
          onPointerUp={(e) => handlePointerButton("pointer.up", e)}
          onContextMenu={(e) => active && e.preventDefault()}
          className={cn(
            "block",
            fit === "fit" || fullscreen
              ? "h-full max-h-[70vh] w-full object-contain"
              : "max-w-none",
            fullscreen && "max-h-screen",
            active ? "cursor-none touch-none" : "cursor-default",
          )}
          style={
            fit === "actual" && dims && !fullscreen
              ? { width: dims.w, height: dims.h }
              : undefined
          }
        />
        {!hasTrack ? (
          <div className="text-muted-foreground absolute inset-0 flex min-h-48 flex-col items-center justify-center gap-2 p-4 text-center text-sm">
            <Loader2 className="size-5 animate-spin" />
            {t(
              grant.isPending || connection === "connecting"
                ? "remote_support.connection.connecting"
                : "remote_support.viewer.waiting_screen",
            )}
          </div>
        ) : null}
      </div>
      <p id={hintId} className="text-muted-foreground text-xs">
        {active
          ? t("remote_support.viewer.keyboard_hint")
          : t("remote_support.viewer.view_hint")}
      </p>
    </div>
  );
}
