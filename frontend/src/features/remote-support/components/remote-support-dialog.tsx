"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Circle,
  Copy,
  Loader2,
  MonitorUp,
  ShieldCheck,
  Square,
} from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { useChannel } from "@/hooks/use-realtime";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  STEPS,
  errorKey,
  formatCode,
  formatDuration,
  isOpen,
  remoteSupportAction,
  sessionStep,
  type Mode,
} from "../lib/access";
import {
  remoteSupportKeys,
  useRemoteSupportScope,
} from "../lib/use-remote-support";
import {
  remoteSupportService,
  type RemoteSupportCreated,
  type RemoteSupportSession,
} from "../services/remote-support.service";
import { RemoteViewer } from "./remote-viewer";

/** Fallback when the realtime channel is not connected. */
const POLL_MS = 4000;

/**
 * Remote support from a conversation (CF-237, docs/design/masaustu.md §5):
 * pick a mode, hand the customer the code, follow the consent steps live,
 * watch (and with consent control) the screen, end and see the summary.
 * Closing the dialog keeps an open session running; the header action
 * brings it back.
 */
export function RemoteSupportDialog({
  conversation,
  session: initial,
  onClose,
}: {
  conversation: string;
  session?: RemoteSupportSession | null;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug, use, control } = useRemoteSupportScope();
  const { active: org } = useActiveOrganization();
  const canAudit = (org?.permissions ?? []).includes(
    Permission.TenantAuditRead,
  );
  const qc = useQueryClient();
  const modes = remoteSupportAction({ use, control }).modes;
  const [mode, setMode] = useState<Mode>("view");
  const [created, setCreated] = useState<RemoteSupportCreated | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(
    initial?.uuid ?? null,
  );

  const key = remoteSupportKeys.session(slug, sessionId ?? "");
  const query = useQuery({
    queryKey: key,
    queryFn: () => remoteSupportService.session(slug, sessionId!),
    enabled: Boolean(slug && sessionId),
    initialData: initial ?? undefined,
    refetchInterval: (q) =>
      q.state.data && isOpen(q.state.data.status) ? POLL_MS : false,
  });
  const session = query.data ?? created?.session ?? null;

  const onLive = useCallback(
    (message: { data: Record<string, unknown> }) => {
      const next = message.data.session as RemoteSupportSession | undefined;
      if (next?.uuid) qc.setQueryData(key, next);
    },
    // key is derived from slug + sessionId
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [qc, slug, sessionId],
  );
  useChannel(session?.channel, onLive, {
    enabled: Boolean(session && isOpen(session.status)),
  });

  const create = useMutation({
    mutationFn: () => remoteSupportService.create(slug, conversation, mode),
    onSuccess: (out) => {
      setCreated(out);
      setSessionId(out.session.uuid);
      qc.setQueryData(
        remoteSupportKeys.session(slug, out.session.uuid),
        out.session,
      );
      void qc.invalidateQueries({
        queryKey: remoteSupportKeys.conversation(slug, conversation),
      });
    },
  });
  const end = useMutation({
    mutationFn: () => remoteSupportService.end(slug, session!.uuid),
    onSuccess: (next) => {
      qc.setQueryData(key, next);
      void qc.invalidateQueries({
        queryKey: remoteSupportKeys.conversation(slug, conversation),
      });
    },
  });

  const open = session ? isOpen(session.status) : false;
  const step = session ? sessionStep(session) : null;
  const watching =
    session &&
    Boolean(session.view_consent_at) &&
    (session.status === "joined" || session.status === "active");
  const error = create.error ?? end.error;

  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("remote_support.dialog.title")}
      description={t("remote_support.dialog.description")}
      className="max-h-[94vh] overflow-y-auto sm:max-w-5xl"
    >
      <div className="space-y-4">
        {!session ? (
          <StartForm modes={modes} mode={mode} onMode={setMode} />
        ) : null}

        {created && session?.status === "pending" ? (
          <CodePanel created={created} />
        ) : null}
        {!created && session?.status === "pending" ? (
          <p className="text-muted-foreground rounded-lg border p-3 text-sm">
            {t("remote_support.code.hidden")}
          </p>
        ) : null}

        {session && step ? <Steps session={session} /> : null}

        {watching ? (
          <RemoteViewer
            slug={slug}
            session={session}
            canControlPermission={control}
          />
        ) : null}

        {session && !open ? <Summary session={session} /> : null}

        {error ? (
          <p className="text-destructive text-sm" role="alert">
            {t(`remote_support.errors.${errorKey(error)}`)}
          </p>
        ) : null}

        <div className="text-muted-foreground flex items-start gap-2 rounded-md border border-dashed p-2.5 text-xs">
          <ShieldCheck className="mt-0.5 size-3.5 shrink-0" />
          <div className="space-y-1">
            <p>{t("remote_support.dialog.audit_note")}</p>
            {session ? (
              <p>
                {t("remote_support.dialog.created_at", {
                  at: datetime(session.created_at, "dd.MM.yyyy HH:mm", locale),
                })}
              </p>
            ) : null}
            {canAudit ? (
              <Link
                href={routes.cms.settings.audit}
                className="underline underline-offset-2"
              >
                {t("remote_support.dialog.audit_link")}
              </Link>
            ) : null}
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
              disabled={create.isPending || !use}
              onClick={() => create.mutate()}
            >
              {create.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <MonitorUp className="size-4" />
              )}
              {t("remote_support.dialog.start")}
            </Button>
          </>
        ) : open ? (
          <>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("remote_support.dialog.minimize")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={end.isPending}
              onClick={() => end.mutate()}
            >
              <Square className="size-4" />
              {session.status === "pending"
                ? t("remote_support.dialog.cancel")
                : t("remote_support.dialog.end")}
            </Button>
          </>
        ) : (
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.close")}
          </Button>
        )}
      </FormActions>
    </AppDialog>
  );
}

function StartForm({
  modes,
  mode,
  onMode,
}: {
  modes: Mode[];
  mode: Mode;
  onMode: (m: Mode) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("remote_support.dialog.intro")}
      </p>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">
          {t("remote_support.mode.label")}
        </legend>
        <RadioGroup
          value={mode}
          onValueChange={(v) => onMode(v as Mode)}
          className="grid gap-2 sm:grid-cols-2"
        >
          {(["view", "control"] as const).map((m) => {
            const disabled = !modes.includes(m);
            return (
              <Label
                key={m}
                htmlFor={`rs-mode-${m}`}
                className={cn(
                  "flex cursor-pointer items-start gap-3 rounded-lg border p-3 font-normal",
                  mode === m && "border-primary",
                  disabled && "cursor-not-allowed opacity-60",
                )}
              >
                <RadioGroupItem
                  id={`rs-mode-${m}`}
                  value={m}
                  disabled={disabled}
                  className="mt-0.5"
                />
                <span className="space-y-0.5">
                  <span className="block text-sm font-medium">
                    {t(`remote_support.mode.${m}`)}
                  </span>
                  <span className="text-muted-foreground block text-xs">
                    {disabled
                      ? t("remote_support.mode.control_forbidden")
                      : t(`remote_support.mode.${m}_hint`)}
                  </span>
                </span>
              </Label>
            );
          })}
        </RadioGroup>
      </fieldset>
      <div className="space-y-1.5">
        <p className="text-sm font-medium">
          {t("remote_support.preview.title")}
        </p>
        <blockquote className="bg-muted/40 rounded-lg border p-3 text-sm whitespace-pre-line">
          {t("remote_support.preview.body", { code: "••• ••• •••" })}
        </blockquote>
        <p className="text-muted-foreground text-xs">
          {t("remote_support.preview.hint")}
        </p>
      </div>
    </div>
  );
}

function CopyRow({ label, value }: { label: string; value: string }) {
  const { t } = useLocale();
  const { isCopied, copyToClipboard } = useCopyToClipboard();
  return (
    <div className="flex items-center gap-2">
      <span className="text-muted-foreground w-28 shrink-0 text-xs">
        {label}
      </span>
      <code className="bg-muted min-w-0 flex-1 truncate rounded px-2 py-1 text-xs">
        {value}
      </code>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => copyToClipboard(value)}
        aria-label={t("remote_support.code.copy_label", { label })}
      >
        {isCopied ? <Check className="size-4" /> : <Copy className="size-4" />}
        {isCopied
          ? t("remote_support.code.copied")
          : t("remote_support.code.copy")}
      </Button>
    </div>
  );
}

function CodePanel({ created }: { created: RemoteSupportCreated }) {
  const { t } = useLocale();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);
  const left = Math.max(
    0,
    (new Date(created.session.code_expires_at).getTime() - now) / 1000,
  );
  const code = formatCode(created.code);
  return (
    <section
      className="space-y-3 rounded-lg border p-4"
      aria-labelledby="rs-code-title"
    >
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h3 id="rs-code-title" className="text-sm font-medium">
            {t("remote_support.code.title")}
          </h3>
          <p
            className="font-mono text-3xl font-semibold tracking-widest"
            aria-label={created.code.split("").join(" ")}
          >
            {code}
          </p>
        </div>
        <p className="text-muted-foreground text-xs" role="timer">
          {t("remote_support.code.expires_in", {
            time: formatDuration(left),
          })}
        </p>
      </div>
      <CopyRow label={t("remote_support.code.code")} value={code} />
      <CopyRow
        label={t("remote_support.code.join_link")}
        value={created.join_url}
      />
      <CopyRow
        label={t("remote_support.code.download")}
        value={created.download_url}
      />
      <p
        className={cn(
          "rounded-md p-2 text-xs",
          created.message_posted
            ? "bg-emerald-500/10 text-emerald-800 dark:text-emerald-200"
            : "bg-amber-500/10 text-amber-900 dark:text-amber-100",
        )}
        role="status"
      >
        {created.message_posted
          ? t("remote_support.code.message_posted")
          : t("remote_support.code.message_not_posted")}
      </p>
      <p className="text-muted-foreground text-xs">
        {t("remote_support.code.once_hint")}
      </p>
    </section>
  );
}

function Steps({ session }: { session: RemoteSupportSession }) {
  const { t } = useLocale();
  const current = sessionStep(session);
  const index = STEPS.indexOf(current);
  return (
    <section aria-labelledby="rs-steps-title" className="space-y-2">
      <h3 id="rs-steps-title" className="sr-only">
        {t("remote_support.steps.title")}
      </h3>
      <ol className="grid gap-2 sm:grid-cols-5" aria-live="polite">
        {STEPS.map((s, i) => {
          const done = i < index || (s === "ended" && current === "ended");
          const here = i === index;
          return (
            <li
              key={s}
              aria-current={here ? "step" : undefined}
              className={cn(
                "flex items-center gap-2 rounded-md border px-2.5 py-2 text-xs",
                here && "border-primary bg-primary/5 font-medium",
                done && !here && "text-muted-foreground",
              )}
            >
              {done ? (
                <Check className="size-3.5 text-emerald-600" />
              ) : here && s !== "ended" ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Circle className="text-muted-foreground size-3.5" />
              )}
              {t(`remote_support.steps.${s}`)}
            </li>
          );
        })}
      </ol>
      <ul className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
        <li>
          {t("remote_support.consent.view")}:{" "}
          {consentLabel(t, session.view_consent_at ? true : null)}
        </li>
        {session.mode_requested === "control" ? (
          <li>
            {t("remote_support.consent.control")}:{" "}
            {session.control_revoked_at
              ? t("remote_support.consent.revoked")
              : consentLabel(t, session.control_consent_at ? true : null)}
          </li>
        ) : null}
        {session.recording_consent !== null &&
        session.recording_consent !== undefined ? (
          <li>
            {t("remote_support.consent.recording")}:{" "}
            {consentLabel(t, session.recording_consent)}
          </li>
        ) : null}
        {session.client_os ? (
          <li>
            {t("remote_support.consent.client", {
              os: session.client_os,
              version: session.client_version || "—",
            })}
          </li>
        ) : null}
      </ul>
    </section>
  );
}

function consentLabel(
  t: (k: string, p?: Record<string, string | number>) => string,
  v: boolean | null | undefined,
) {
  if (v === true) return t("remote_support.consent.given");
  if (v === false) return t("remote_support.consent.declined");
  return t("remote_support.consent.waiting");
}

function Summary({ session }: { session: RemoteSupportSession }) {
  const { t, locale } = useLocale();
  const seconds =
    session.started_at && session.ended_at
      ? (new Date(session.ended_at).getTime() -
          new Date(session.started_at).getTime()) /
        1000
      : 0;
  const rows: [string, string][] = [
    [
      t("remote_support.summary.status"),
      t(`remote_support.status.${session.status}`),
    ],
    [
      t("remote_support.summary.reason"),
      session.ended_reason
        ? t(`remote_support.reason.${session.ended_reason}`)
        : "—",
    ],
    [t("remote_support.summary.duration"), formatDuration(seconds)],
    [
      t("remote_support.summary.billed"),
      t("remote_support.summary.minutes", { minutes: session.billed_minutes }),
    ],
    [
      t("remote_support.summary.mode"),
      t(`remote_support.mode.${session.mode_requested}`),
    ],
    [
      t("remote_support.consent.view"),
      consentLabel(t, session.view_consent_at ? true : null),
    ],
  ];
  if (session.mode_requested === "control")
    rows.push([
      t("remote_support.consent.control"),
      session.control_revoked_at
        ? t("remote_support.consent.revoked")
        : consentLabel(t, session.control_consent_at ? true : null),
    ]);
  if (session.recording_status !== "none")
    rows.push([
      t("remote_support.summary.recording"),
      t(`remote_support.recording.${session.recording_status}`),
    ]);
  if (session.ended_at)
    rows.push([
      t("remote_support.summary.ended_at"),
      datetime(session.ended_at, "dd.MM.yyyy HH:mm", locale),
    ]);
  return (
    <section
      className="space-y-2 rounded-lg border p-4"
      aria-labelledby="rs-summary-title"
      role="status"
    >
      <h3 id="rs-summary-title" className="text-sm font-medium">
        {t("remote_support.summary.title")}
      </h3>
      <dl className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-2">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between gap-3">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="font-medium">{v}</dd>
          </div>
        ))}
      </dl>
      <p className="text-muted-foreground text-xs">
        {t("remote_support.summary.timeline_note")}
      </p>
    </section>
  );
}
