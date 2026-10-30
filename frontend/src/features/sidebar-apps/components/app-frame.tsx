"use client";

import { useQueryClient } from "@tanstack/react-query";
import { RefreshCw, ShieldAlert } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  inboundMessage,
  looseRequestId,
  type ActionMessage,
} from "../lib/protocol";
import {
  sidebarAppsService,
  type SidebarApp,
  type SidebarAppLaunch,
} from "../services/sidebar-apps.service";

/** Refresh the 120 s context token this long before it expires. */
const REFRESH_LEAD_MS = 20_000;
/** At most this many actions per frame in a sliding window. */
const ACTION_BURST = 10;
const ACTION_WINDOW_MS = 10_000;

export type AppContext = { conversation?: string; contact?: string };

type Launched = SidebarAppLaunch & { at: number };

/**
 * Hosts one sidebar app iframe.
 *
 * - The frame is sandboxed without top navigation or popups, sends no
 *   referrer and gets no browser permissions.
 * - Messages are accepted only from this frame's window *and* the exact
 *   origin returned by launch; payloads are schema-validated.
 * - The context token is posted only after `clickfix.ready`, only to that
 *   origin, and never put in a URL, log or storage. It is refreshed before
 *   expiry; a context change remounts the frame (old context destroyed).
 * - Actions are forwarded only for allowlisted, granted scopes, through the
 *   tenant API with the host session; the backend re-checks everything.
 */
export function AppFrame({
  app,
  context,
  height = 420,
}: {
  app: SidebarApp;
  context: AppContext;
  height?: number | string;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const frame = useRef<HTMLIFrameElement>(null);
  const [attempt, setAttempt] = useState(0);
  const launchRef = useRef<Launched | null>(null);
  const ready = useRef(false);
  const seen = useRef(new Set<string>());
  const recent = useRef<number[]>([]);

  const { conversation, contact } = context;
  const contextKey = `${app.uuid}:${app.revision}:${conversation ?? ""}|${contact ?? ""}:${attempt}`;
  // State is tagged with the context it belongs to, so a context change
  // shows the loading state at once without resetting state in an effect.
  const [launchState, setLaunchState] = useState<{
    key: string;
    launch: Launched | null;
    error: string | null;
  } | null>(null);
  const current = launchState?.key === contextKey ? launchState : null;
  const launch = current?.launch ?? null;
  const error = current?.error ?? null;
  const orgSlug = org?.slug;

  const postContext = useCallback((l: Launched) => {
    const win = frame.current?.contentWindow;
    if (!win || !ready.current) return;
    win.postMessage(
      {
        type: "clickfix.context",
        context_token: l.context_token,
        scopes: l.scopes,
      },
      l.origin,
    );
  }, []);

  // Launch (and re-launch on context change or retry). The previous frame and
  // its token are dropped before the new one is requested.
  useEffect(() => {
    let cancelled = false;
    launchRef.current = null;
    ready.current = false;
    seen.current = new Set();
    sidebarAppsService
      .launch(app.uuid, {
        ...(conversation ? { conversation_uuid: conversation } : {}),
        ...(contact ? { contact_uuid: contact } : {}),
      })
      .then((l) => {
        if (cancelled) return;
        // Same-origin frames could lift their own sandbox; refuse them.
        if (l.origin === window.location.origin) {
          setLaunchState({
            key: contextKey,
            launch: null,
            error: t("apps.host.same_origin"),
          });
          return;
        }
        const next = { ...l, at: Date.now() };
        launchRef.current = next;
        setLaunchState({ key: contextKey, launch: next, error: null });
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        setLaunchState({
          key: contextKey,
          launch: null,
          error:
            isApiError(e) && e.status === 403
              ? t("apps.host.denied")
              : t("apps.host.failed"),
        });
      });
    return () => {
      cancelled = true;
    };
  }, [app.uuid, conversation, contact, contextKey, t]);

  // Token refresh: same frame, new context message.
  useEffect(() => {
    if (!launch) return;
    const wait = Math.max(5_000, launch.expires_in * 1000 - REFRESH_LEAD_MS);
    const timer = window.setTimeout(() => {
      sidebarAppsService
        .launch(app.uuid, {
          ...(conversation ? { conversation_uuid: conversation } : {}),
          ...(contact ? { contact_uuid: contact } : {}),
        })
        .then((l) => {
          if (launchRef.current?.origin !== l.origin) {
            // Registration changed under us; start over with a fresh frame.
            setAttempt((n) => n + 1);
            return;
          }
          const next = { ...l, at: Date.now() };
          launchRef.current = next;
          setLaunchState({ key: contextKey, launch: next, error: null });
          postContext(next);
        })
        .catch(() =>
          setLaunchState({
            key: contextKey,
            launch: null,
            error: t("apps.host.expired"),
          }),
        );
    }, wait);
    return () => window.clearTimeout(timer);
  }, [app.uuid, contact, contextKey, conversation, launch, postContext, t]);

  const respond = useCallback(
    (requestId: string, success: boolean, err?: string) => {
      const l = launchRef.current;
      const win = frame.current?.contentWindow;
      if (!l || !win) return;
      win.postMessage(
        {
          type: "clickfix.action.result",
          request_id: requestId,
          success,
          ...(err ? { error: err } : {}),
        },
        l.origin,
      );
    },
    [],
  );

  const runAction = useCallback(
    async (msg: ActionMessage) => {
      const l = launchRef.current;
      if (!l) return;
      if (seen.current.has(msg.request_id)) {
        respond(msg.request_id, false, "duplicate_request");
        return;
      }
      seen.current.add(msg.request_id);
      const now = Date.now();
      recent.current = recent.current.filter(
        (at) => now - at < ACTION_WINDOW_MS,
      );
      if (recent.current.length >= ACTION_BURST) {
        respond(msg.request_id, false, "rate_limited");
        return;
      }
      recent.current.push(now);
      if (!conversation) {
        respond(msg.request_id, false, "no_conversation");
        return;
      }
      if (!l.scopes.includes(msg.action)) {
        respond(msg.request_id, false, "scope_denied");
        return;
      }
      try {
        await sidebarAppsService.action(app.uuid, {
          context_token: l.context_token,
          request_id: msg.request_id,
          action: msg.action,
          ...(msg.value !== undefined ? { value: msg.value } : {}),
          ...(msg.content !== undefined ? { content: msg.content } : {}),
          ...(msg.target_uuid ? { target_uuid: msg.target_uuid } : {}),
        });
        respond(msg.request_id, true);
        appToast.success(
          t("apps.host.action_done", {
            app: app.name,
            action: t(`apps.scopes.${msg.action.replace(".", "_")}`),
          }),
        );
        if (orgSlug)
          void qc.invalidateQueries({ queryKey: ["inbox", orgSlug] });
      } catch (e) {
        respond(
          msg.request_id,
          false,
          isApiError(e)
            ? e.status === 403
              ? "forbidden"
              : e.status === 400
                ? "invalid_request"
                : "failed"
            : "failed",
        );
      }
    },
    [app.name, app.uuid, conversation, orgSlug, qc, respond, t],
  );

  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const l = launchRef.current;
      const win = frame.current?.contentWindow;
      // Both checks: another frame on the same origin must not be trusted.
      if (!l || !win || event.source !== win || event.origin !== l.origin)
        return;
      const parsed = inboundMessage.safeParse(event.data);
      if (!parsed.success) {
        const id = looseRequestId(event.data);
        if (id) respond(id, false, "invalid_request");
        return;
      }
      if (parsed.data.type === "clickfix.ready") {
        ready.current = true;
        postContext(l);
        return;
      }
      void runAction(parsed.data);
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [postContext, respond, runAction]);

  if (error)
    return (
      <div className="text-muted-foreground flex flex-col items-center gap-2 rounded-md border border-dashed p-4 text-center text-xs">
        <ShieldAlert className="size-5" />
        {error}
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => setAttempt((n) => n + 1)}
        >
          <RefreshCw className="size-3.5" />
          {t("apps.host.retry")}
        </Button>
      </div>
    );
  if (!launch)
    return (
      <div
        className="bg-muted/40 animate-pulse rounded-md border"
        style={{ height }}
        aria-busy
        aria-label={t("common.loading")}
      />
    );
  return (
    <iframe
      // A new key per launch context destroys the previous frame and token.
      key={contextKey}
      ref={frame}
      src={launch.url}
      title={app.name}
      sandbox="allow-scripts allow-forms allow-same-origin"
      referrerPolicy="no-referrer"
      allow=""
      loading="lazy"
      className="bg-background w-full rounded-md border"
      style={{ height }}
    />
  );
}
