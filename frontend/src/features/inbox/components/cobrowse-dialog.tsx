"use client";

import "rrweb/dist/style.css";

import { useMutation } from "@tanstack/react-query";
import { Centrifuge } from "centrifuge";
import {
  Hand,
  Loader2,
  MousePointer2,
  ScanEye,
  ShieldCheck,
  Square,
  Target,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type { Replayer } from "rrweb";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Permission } from "@/config/permissions";
import { realtimeConfig } from "@/config/realtime";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  EVENT_FULL_SNAPSHOT,
  EVENT_META,
  createReplayer,
  useFittedViewport,
} from "../lib/replayer";
import {
  replayService,
  type PointerType,
  type ReplayEvent,
  type WidgetLiveTokens,
  type WidgetSession,
} from "../services/replay.service";

/** The widget batches DOM once per second; play slightly behind it. */
const LIVE_BUFFER_MS = 1500;
const POLL_MS = 2500;

type LiveMessage =
  | { type: "cobrowse.dom"; events?: ReplayEvent[] }
  | { type: "cobrowse.status"; session?: WidgetSession }
  | { type: "cobrowse.pointer" };

/**
 * "Ortak tarama" (CF-126): asks the visitor for a live view of their page,
 * waits for consent, then renders the masked DOM stream and lets the agent
 * point or highlight. Nothing of the stream is stored; request, consent,
 * guided clicks and stop are written to the audit log by the API.
 */
export function CobrowseDialog({
  conversationUuid,
  session: initial,
  onClose,
}: {
  conversationUuid: string;
  /** Re-joins an existing request/active session when given. */
  session?: WidgetSession;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = new Set(org?.permissions ?? []);
  const canList = perms.has(Permission.TenantReplaysRead);
  const canControl = perms.has(Permission.TenantCobrowseControl);
  const canAudit = perms.has(Permission.TenantAuditRead);
  const [session, setSession] = useState<WidgetSession | null>(initial ?? null);
  const [tokens, setTokens] = useState<WidgetLiveTokens | null>(null);
  const [wantControl, setWantControl] = useState(false);

  const request = useMutation({
    mutationFn: () =>
      replayService.requestCobrowse(conversationUuid, wantControl),
    onSuccess: setSession,
  });
  const stop = useMutation({
    mutationFn: (uuid: string) => replayService.stop(conversationUuid, uuid),
    onSuccess: setSession,
  });

  const status = session?.status;
  const waiting = status === "requested";
  const live = status === "active";

  // Waiting for consent: the visitor answers in the widget, so watch the
  // session. Without tenant.replays.read the session list is forbidden;
  // then the live-token endpoint (400 until accepted) is the signal.
  const [liveError, setLiveError] = useState<unknown>(null);
  const sessionUuid = session?.uuid;
  const timeoutAt = session?.timeout_at;
  const needPoll = Boolean(sessionUuid) && (waiting || (live && !tokens));
  useEffect(() => {
    if (!needPoll || !sessionUuid) return;
    let cancelled = false;
    let timer = 0;
    const again = () => {
      const open = timeoutAt && new Date(timeoutAt).getTime() > Date.now();
      if (!cancelled && waiting && open)
        timer = window.setTimeout(check, POLL_MS);
    };
    async function check() {
      if (canList && waiting) {
        try {
          const list = await replayService.sessions(conversationUuid);
          if (cancelled) return;
          const next = list.find((s) => s.uuid === sessionUuid);
          if (!next || next.status !== "active") {
            setSession((s) =>
              s ? (next ?? { ...s, status: "ended" as const }) : s,
            );
            if (next?.status === "requested") again();
            return;
          }
        } catch {
          again();
          return;
        }
      }
      try {
        const tk = await replayService.liveToken(
          conversationUuid,
          sessionUuid!,
        );
        if (cancelled) return;
        setTokens(tk);
        setSession((s) => (s ? { ...s, status: "active" } : s));
      } catch (err) {
        if (cancelled) return;
        if (waiting) again();
        else setLiveError(err);
      }
    }
    void check();
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [needPoll, sessionUuid, timeoutAt, waiting, canList, conversationUuid]);

  // The request expires at timeout_at; reflect it without a server round trip.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!waiting && !live) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [waiting, live]);
  const expired =
    session !== null &&
    (waiting || live) &&
    new Date(session.timeout_at).getTime() <= now;
  const remaining = session
    ? Math.max(0, new Date(session.timeout_at).getTime() - now)
    : 0;

  const finish = () => {
    if (session && (waiting || live) && !expired) stop.mutate(session.uuid);
    else onClose();
  };

  const error = request.error ?? stop.error ?? liveError;
  const ended = session !== null && (!(waiting || live) || expired);

  return (
    <AppDialog
      open
      onOpenChange={(o) => {
        if (!o) {
          // Closing the dialog never leaves a visitor's page shared.
          if (session && (waiting || live) && !expired)
            void replayService
              .stop(conversationUuid, session.uuid)
              .catch(() => undefined);
          onClose();
        }
      }}
      title={t("inbox.cobrowse.title")}
      description={t("inbox.cobrowse.description")}
      className="max-h-[94vh] overflow-y-auto sm:max-w-5xl"
    >
      <div className="space-y-4">
        {!session && (
          <div className="space-y-3">
            <p className="text-muted-foreground text-sm">
              {t("inbox.cobrowse.intro")}
            </p>
            {canControl && (
              <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
                <div>
                  <Label htmlFor="cobrowse-control">
                    {t("inbox.cobrowse.control")}
                  </Label>
                  <p className="text-muted-foreground text-xs">
                    {t("inbox.cobrowse.control_hint")}
                  </p>
                </div>
                <Switch
                  id="cobrowse-control"
                  checked={wantControl}
                  onCheckedChange={setWantControl}
                />
              </div>
            )}
          </div>
        )}

        {session && waiting && !expired && (
          <div
            className="flex items-center gap-3 rounded-lg border p-4"
            role="status"
          >
            <Loader2 className="text-muted-foreground size-5 animate-spin" />
            <div className="min-w-0">
              <p className="text-sm font-medium">
                {t("inbox.cobrowse.waiting")}
              </p>
              <p className="text-muted-foreground text-xs">
                {t("inbox.cobrowse.waiting_hint", {
                  minutes: Math.ceil(remaining / 60_000),
                })}
              </p>
            </div>
          </div>
        )}

        {session && live && !expired && tokens && (
          <LiveStage
            conversationUuid={conversationUuid}
            session={session}
            tokens={tokens}
            canControl={canControl && session.control_enabled}
            onStatus={setSession}
          />
        )}
        {session && live && !expired && !tokens && (
          <div
            className="flex items-center gap-3 rounded-lg border p-4"
            role="status"
          >
            <Loader2 className="text-muted-foreground size-5 animate-spin" />
            <p className="text-sm">{t("inbox.cobrowse.connecting")}</p>
          </div>
        )}

        {ended && (
          <p
            className={cn(
              "rounded-lg border p-4 text-sm",
              status === "declined" &&
                "border-amber-500/40 bg-amber-500/5 text-amber-800 dark:text-amber-200",
            )}
            role="status"
          >
            {expired
              ? t("inbox.cobrowse.status_text.timeout")
              : t(`inbox.cobrowse.status_text.${status}`)}
          </p>
        )}

        {Boolean(error) && (
          <p className="text-destructive text-sm" role="alert">
            {isApiError(error) ? error.message : t("inbox.cobrowse.failed")}
          </p>
        )}

        <div className="text-muted-foreground flex items-start gap-2 rounded-md border border-dashed p-2.5 text-xs">
          <ShieldCheck className="mt-0.5 size-3.5 shrink-0" />
          <div className="space-y-1">
            <p>{t("inbox.cobrowse.audit_note")}</p>
            {session && (
              <p>
                {t("inbox.cobrowse.audit_meta", {
                  version: session.consent_version || "—",
                  accepted: session.accepted_at
                    ? datetime(session.accepted_at, "dd.MM.yyyy HH:mm", locale)
                    : "—",
                })}
              </p>
            )}
            {canAudit && (
              <Link
                href={routes.cms.settings.audit}
                className="underline underline-offset-2"
              >
                {t("inbox.cobrowse.audit_link")}
              </Link>
            )}
          </div>
        </div>
      </div>
      <FormActions sticky={false}>
        {!session ? (
          <>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              disabled={request.isPending}
              onClick={() => request.mutate()}
            >
              <ScanEye className="size-4" />
              {t("inbox.cobrowse.send")}
            </Button>
          </>
        ) : ended ? (
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.close")}
          </Button>
        ) : (
          <Button
            type="button"
            variant="destructive"
            disabled={stop.isPending}
            onClick={finish}
          >
            <Square className="size-4" />
            {waiting ? t("inbox.cobrowse.cancel") : t("inbox.cobrowse.end")}
          </Button>
        )}
      </FormActions>
    </AppDialog>
  );
}

type Mark = { id: number; x: number; y: number; type: PointerType };

function LiveStage({
  conversationUuid,
  session,
  tokens,
  canControl,
  onStatus,
}: {
  conversationUuid: string;
  session: WidgetSession;
  tokens: WidgetLiveTokens;
  canControl: boolean;
  onStatus: (s: WidgetSession) => void;
}) {
  const { t } = useLocale();
  const root = useRef<HTMLDivElement | null>(null);
  const { frame, size, scale, bind } = useFittedViewport(560);
  const [connection, setConnection] = useState<
    "connecting" | "connected" | "disconnected"
  >("connecting");
  const [snapshot, setSnapshot] = useState(false);
  const [mode, setMode] = useState<PointerType>("pointer");
  const [marks, setMarks] = useState<Mark[]>([]);
  const statusRef = useRef(onStatus);
  useEffect(() => {
    statusRef.current = onStatus;
  }, [onStatus]);

  useEffect(() => {
    const el = root.current;
    if (!el) return;
    let replayer: Replayer | null = null;
    let starting = false;
    let disposed = false;
    // Live DOM has no history: until a full snapshot arrives (the widget
    // re-checks out every 30 s) incremental events cannot be applied.
    let pending: ReplayEvent[] = [];

    const start = (batch: ReplayEvent[]) => {
      const full = batch.findIndex((e) => e.type === EVENT_FULL_SNAPSHOT);
      if (full < 0) return;
      let from = full;
      for (let i = full; i >= 0; i--) {
        if (batch[i]?.type === EVENT_META) {
          from = i;
          break;
        }
      }
      starting = true;
      const seed = batch.slice(from);
      pending = [];
      void createReplayer(el, [], true).then((r) => {
        if (disposed) {
          r.destroy();
          return;
        }
        replayer = r;
        bind(r);
        r.startLive((seed[0]?.timestamp ?? Date.now()) - LIVE_BUFFER_MS);
        for (const e of [...seed, ...pending]) r.addEvent(e as never);
        pending = [];
        setSnapshot(true);
      });
    };

    const client = new Centrifuge(tokens.ws_url || realtimeConfig.wsUrl, {
      token: tokens.connection_token,
    });
    client.on("connecting", () => setConnection("connecting"));
    client.on("connected", () => setConnection("connected"));
    client.on("disconnected", () => setConnection("disconnected"));
    const sub = client.newSubscription(tokens.channel, {
      token: tokens.subscription_token,
    });
    sub.on("publication", (ctx) => {
      const msg = ctx.data as LiveMessage;
      if (msg.type === "cobrowse.status" && msg.session) {
        statusRef.current(msg.session);
        return;
      }
      if (msg.type !== "cobrowse.dom" || !Array.isArray(msg.events)) return;
      const batch = msg.events;
      if (replayer) {
        for (const e of batch) replayer.addEvent(e as never);
      } else if (starting) {
        pending.push(...batch);
      } else {
        start(batch);
      }
    });
    sub.subscribe();
    client.connect();

    return () => {
      disposed = true;
      sub.unsubscribe();
      sub.removeAllListeners();
      client.disconnect();
      bind(null);
      replayer?.destroy();
      el.replaceChildren();
    };
  }, [tokens, bind]);

  const send = useMutation({
    mutationFn: (m: Mark) =>
      replayService.pointer(conversationUuid, session.uuid, m.type, m.x, m.y),
  });

  const onPoint = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = (e.clientX - rect.left) / scale;
    const y = (e.clientY - rect.top) / scale;
    const mark: Mark = { id: Date.now(), x, y, type: mode };
    setMarks((list) => [...list.slice(-4), mark]);
    window.setTimeout(
      () => setMarks((list) => list.filter((m) => m.id !== mark.id)),
      1500,
    );
    send.mutate(mark);
  };

  const modes: { type: PointerType; icon: typeof Target; label: string }[] = [
    {
      type: "pointer",
      icon: MousePointer2,
      label: t("inbox.cobrowse.pointer"),
    },
    { type: "highlight", icon: Target, label: t("inbox.cobrowse.highlight") },
  ];
  if (canControl)
    modes.push({
      type: "control_click",
      icon: Hand,
      label: t("inbox.cobrowse.control_click"),
    });

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <div
          className="flex gap-1"
          role="radiogroup"
          aria-label={t("inbox.cobrowse.mode")}
        >
          {modes.map((m) => (
            <Button
              key={m.type}
              type="button"
              size="sm"
              role="radio"
              aria-checked={mode === m.type}
              variant={mode === m.type ? "secondary" : "ghost"}
              onClick={() => setMode(m.type)}
            >
              <m.icon className="size-4" />
              {m.label}
            </Button>
          ))}
        </div>
        <span
          className={cn(
            "ml-auto flex items-center gap-1.5 text-xs",
            connection === "connected"
              ? "text-emerald-700 dark:text-emerald-300"
              : "text-muted-foreground",
          )}
          role="status"
        >
          <span
            className={cn(
              "size-2 rounded-full",
              connection === "connected" ? "bg-emerald-500" : "bg-amber-500",
            )}
          />
          {t(`inbox.cobrowse.connection.${connection}`)}
        </span>
      </div>
      {send.isError && (
        <p className="text-destructive text-xs" role="alert">
          {isApiError(send.error)
            ? send.error.message
            : t("inbox.cobrowse.pointer_failed")}
        </p>
      )}
      <div
        ref={frame}
        className="bg-muted/40 relative w-full overflow-hidden rounded-lg border"
        style={{ height: snapshot && size ? size.height * scale : 240 }}
      >
        <div ref={root} className="absolute top-0 left-0" />
        {!snapshot && (
          <div className="text-muted-foreground absolute inset-0 flex flex-col items-center justify-center gap-2 p-4 text-center text-sm">
            <Loader2 className="size-5 animate-spin" />
            {t("inbox.cobrowse.waiting_snapshot")}
          </div>
        )}
        {snapshot && size && (
          <div
            className={cn(
              "absolute top-0 left-0",
              mode === "control_click" ? "cursor-pointer" : "cursor-crosshair",
            )}
            style={{ width: size.width * scale, height: size.height * scale }}
            onClick={onPoint}
            role="presentation"
          >
            {marks.map((m) => (
              <span
                key={m.id}
                aria-hidden
                className={cn(
                  "border-primary pointer-events-none absolute -translate-x-1/2 -translate-y-1/2 animate-ping rounded-full border-2",
                  m.type === "highlight" ? "size-10" : "size-4",
                )}
                style={{ left: m.x * scale, top: m.y * scale }}
              />
            ))}
          </div>
        )}
      </div>
      <p className="text-muted-foreground text-xs">
        {t("inbox.cobrowse.masked_note")}
      </p>
    </div>
  );
}
