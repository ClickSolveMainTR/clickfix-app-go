"use client";

import { Room, RoomEvent, type RemoteParticipant } from "livekit-client";
import { FlaskConical, Hand, MonitorUp, Square } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale } from "@/providers/locale-provider";

import { errorKey } from "../lib/access";
import { CONTROL_TOPIC, decodeEvent, type ControlEvent } from "../lib/control";
import {
  publicRemoteSupportService as api,
  type RemoteSupportCustomerView,
  type RemoteSupportResolved,
} from "../services/remote-support.service";

const AGENT_PREFIX = "rs:agent:";

/**
 * Development stand-in for ClickFix Destek (CF-239) so the agent console can
 * be tried end to end in two browser tabs: enter the code, give the
 * consents, share a screen with getDisplayMedia and see the control events
 * the agent sends (they are only listed, never applied). The route renders
 * this only when NODE_ENV is development.
 */
export function DevCustomerPage() {
  const { t } = useLocale();
  const [code, setCode] = useState("");
  const [resolved, setResolved] = useState<RemoteSupportResolved | null>(null);
  const [view, setView] = useState<RemoteSupportCustomerView | null>(null);
  const [consents, setConsents] = useState({
    view: false,
    control: false,
    recording: false,
  });
  const [sharing, setSharing] = useState(false);
  const [events, setEvents] = useState<ControlEvent[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const room = useRef<Room | null>(null);

  useEffect(
    () => () => {
      void room.current?.disconnect();
    },
    [],
  );

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (err) {
      setError(errorKey(err));
    } finally {
      setBusy(false);
    }
  };

  const resolve = () =>
    run(async () => {
      setResolved(await api.resolve(code));
    });

  const start = () =>
    run(async () => {
      if (!resolved) return;
      const secret = resolved.client_secret;
      const id = resolved.session_uuid;
      let v = await api.consent(id, { client_secret: secret, view: true });
      if (resolved.mode_requested === "control" && consents.control)
        v = await api.consent(id, { client_secret: secret, control: true });
      if (resolved.recording_requested)
        v = await api.consent(id, {
          client_secret: secret,
          recording: consents.recording,
        });
      setView(v);
      const grant = await api.token(id, secret);
      const r = new Room();
      room.current = r;
      r.on(
        RoomEvent.DataReceived,
        (
          payload: Uint8Array,
          participant?: RemoteParticipant,
          _kind?: unknown,
          topic?: string,
        ) => {
          // The sidecar applies only events from the agent identity.
          if (topic !== CONTROL_TOPIC) return;
          if (!participant?.identity.startsWith(AGENT_PREFIX)) return;
          const e = decodeEvent(payload);
          if (e) setEvents((list) => [e, ...list].slice(0, 30));
        },
      ).on(RoomEvent.Disconnected, () => setSharing(false));
      await r.connect(grant.server_url, grant.token);
      await r.localParticipant.setScreenShareEnabled(true, { audio: false });
      setSharing(true);
    });

  const setControl = (on: boolean) =>
    run(async () => {
      if (!resolved) return;
      setView(
        await api.consent(resolved.session_uuid, {
          client_secret: resolved.client_secret,
          control: on,
        }),
      );
    });

  const stop = () =>
    run(async () => {
      if (!resolved) return;
      await room.current?.disconnect();
      room.current = null;
      setSharing(false);
      setView(await api.end(resolved.session_uuid, resolved.client_secret));
    });

  return (
    <main className="mx-auto max-w-2xl space-y-6 p-6">
      <div
        className="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-sm"
        role="note"
      >
        <FlaskConical className="mt-0.5 size-4 shrink-0" />
        {t("remote_support.dev.banner")}
      </div>
      <h1 className="text-xl font-semibold">{t("remote_support.dev.title")}</h1>

      {!resolved ? (
        <form
          className="space-y-2"
          onSubmit={(e) => {
            e.preventDefault();
            void resolve();
          }}
        >
          <Label htmlFor="dev-code">{t("remote_support.dev.code")}</Label>
          <div className="flex gap-2">
            <Input
              id="dev-code"
              autoFocus
              inputMode="numeric"
              autoComplete="one-time-code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
            <Button
              type="submit"
              disabled={busy || code.replace(/\D/g, "").length !== 9}
            >
              {t("remote_support.dev.resolve")}
            </Button>
          </div>
        </form>
      ) : null}

      {resolved && !view ? (
        <section className="space-y-3 rounded-lg border p-4">
          <p className="text-sm">
            {t("remote_support.dev.who", {
              org: resolved.organization.name,
              agent: resolved.agent.name,
            })}
            {resolved.organization.verified
              ? ` · ${t("remote_support.dev.verified")}`
              : ""}
          </p>
          <p className="rounded-md bg-red-500/10 p-2 text-sm">
            {resolved.warning}
          </p>
          <ConsentBox
            id="dev-view"
            label={t("remote_support.dev.consent_view")}
            checked={consents.view}
            onChange={(v) => setConsents({ ...consents, view: v })}
          />
          {resolved.mode_requested === "control" ? (
            <ConsentBox
              id="dev-control"
              label={t("remote_support.dev.consent_control")}
              checked={consents.control}
              onChange={(v) => setConsents({ ...consents, control: v })}
            />
          ) : null}
          {resolved.recording_requested ? (
            <ConsentBox
              id="dev-recording"
              label={t("remote_support.dev.consent_recording")}
              checked={consents.recording}
              onChange={(v) => setConsents({ ...consents, recording: v })}
            />
          ) : null}
          <div className="flex gap-2">
            <Button
              disabled={busy || !consents.view}
              onClick={() => void start()}
            >
              <MonitorUp className="size-4" />
              {t("remote_support.dev.share")}
            </Button>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void stop()}
            >
              {t("remote_support.dev.decline")}
            </Button>
          </div>
        </section>
      ) : null}

      {view ? (
        <section className="space-y-3 rounded-lg border p-4" aria-live="polite">
          <p className="text-sm">
            {t("remote_support.dev.state", {
              status: t(`remote_support.status.${view.status}`),
            })}
            {sharing ? ` · ${t("remote_support.dev.sharing")}` : ""}
          </p>
          <div className="flex flex-wrap gap-2">
            {resolved?.mode_requested === "control" ? (
              <Button
                variant="outline"
                disabled={busy || view.ended_at != null}
                onClick={() => void setControl(!view.control_active)}
              >
                <Hand className="size-4" />
                {view.control_active
                  ? t("remote_support.dev.revoke_control")
                  : t("remote_support.dev.grant_control")}
              </Button>
            ) : null}
            <Button
              variant="destructive"
              disabled={busy || view.ended_at != null}
              onClick={() => void stop()}
            >
              <Square className="size-4" />
              {t("remote_support.dev.stop")}
            </Button>
          </div>
          <h2 className="text-sm font-medium">
            {t("remote_support.dev.events")}
          </h2>
          <ol className="bg-muted/40 max-h-64 overflow-auto rounded-md p-2 font-mono text-xs">
            {events.length === 0 ? (
              <li className="text-muted-foreground">
                {t("remote_support.dev.no_events")}
              </li>
            ) : (
              events.map((e) => <li key={e.seq}>{JSON.stringify(e)}</li>)
            )}
          </ol>
        </section>
      ) : null}

      {error ? (
        <p className="text-destructive text-sm" role="alert">
          {t(`remote_support.errors.${error}`)}
        </p>
      ) : null}
    </main>
  );
}

function ConsentBox({
  id,
  label,
  checked,
  onChange,
}: {
  id: string;
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-start gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onChange(v === true)}
      />
      <Label htmlFor={id} className="text-sm leading-snug font-normal">
        {label}
      </Label>
    </div>
  );
}
