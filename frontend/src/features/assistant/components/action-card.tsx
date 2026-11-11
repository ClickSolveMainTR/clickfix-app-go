"use client";

import {
  useAuiState,
  type ToolCallMessagePartProps,
} from "@assistant-ui/react";
import {
  AlertTriangle,
  Check,
  CheckCircle2,
  Copy,
  FlaskConical,
  Loader2,
  X,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useId, useMemo, useState } from "react";

import {
  Confirmation,
  ConfirmationAction,
  ConfirmationActions,
  ConfirmationRequest,
  ConfirmationTitle,
  type ConfirmationState,
} from "@/components/ai-elements/confirmation";
import {
  Tool,
  ToolContent,
  ToolHeader,
  ToolInput,
  ToolOutput,
  getStatusBadge,
  type ToolState,
} from "@/components/ai-elements/tool";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

import { focusHref, readValue, toText } from "../lib/card";
import { errorKey } from "../lib/errors";
import type { ActionResult } from "../lib/messages";
import { useAssistantSurface } from "../runtime/surface";
import {
  assistantService,
  type AssistantAction,
  type AssistantActionDetail,
} from "../services/assistant.service";

/** Fields edited in the card, sent with dry-run / confirm. */
export type EditedParams = Record<string, unknown>;

export function RouteLink({
  route,
  target,
}: {
  route: string;
  target?: string | null;
}) {
  const { t } = useLocale();
  const href = focusHref(route, target);
  if (!href) return null;
  return (
    <Link href={href} className="text-primary mt-1 inline-block underline">
      {target
        ? t("assistant.action.open_and_show")
        : t("assistant.action.open_page")}
    </Link>
  );
}

/** A translation, or the server's label when the key has none. */
function tOr(t: (key: string) => string, key: string, fallback: string) {
  const v = t(key);
  return v === key ? fallback : v;
}

function CopyButton({ value }: { value: string }) {
  const { t } = useLocale();
  const [done, setDone] = useState(false);
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      onClick={() => {
        void navigator.clipboard
          .writeText(value)
          .then(() => setDone(true))
          .catch(() => undefined);
      }}
    >
      <Copy className="size-3" />
      {done ? t("assistant.copied") : t("assistant.copy")}
    </Button>
  );
}

function Details({ items }: { items?: AssistantActionDetail[] | null }) {
  if (!items?.length) return null;
  return (
    <div className="space-y-2 text-xs">
      {items.map((d, i) => (
        <div key={`${d.label}-${i}`}>
          <p className="text-muted-foreground">{d.label}</p>
          {d.value ? <p className="break-all">{d.value}</p> : null}
          {d.code ? (
            <div className="bg-muted mt-1 rounded-md">
              <div className="flex justify-end border-b px-1">
                <CopyButton value={d.code} />
              </div>
              <pre className="overflow-x-auto p-2 font-mono text-[11px] whitespace-pre">
                {d.code}
              </pre>
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

const toolState = (action: AssistantAction): ToolState =>
  action.status === "done"
    ? "output-available"
    : action.status === "failed"
      ? "output-error"
      : action.status === "rejected"
        ? "output-denied"
        : action.status === "proposed"
          ? "approval-requested"
          : "input-available";

/**
 * Confirmation card (CF-184) as the assistant-ui tool UI of a proposed
 * action: editable params, dry run, verification after apply, a secret
 * shown once, "go to the page and highlight". The card decides through the
 * assistant-ui approval gate; the chat model then calls confirm / reject.
 * When the gate cannot take the decision (an older card in the thread, a
 * failed confirm), the card calls the endpoints itself. Every value is
 * checked again by the server; the form is only a draft.
 */
export function ActionCard(
  props: ToolCallMessagePartProps<unknown, ActionResult>,
) {
  const { t, locale } = useLocale();
  // The same card can render in two panels at once (page + sheet).
  const fieldId = useId();
  const surface = useAssistantSurface();
  const remoteId = useAuiState((s) => s.threadListItem.remoteId);
  const proposed = props.artifact as AssistantAction;
  const [local, setLocal] = useState<ActionResult | null>(null);
  const [preview, setPreview] = useState<AssistantAction["preview"]>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A denied gate leaves `{ error }` as the result; only a server answer
  // (confirm / history) carries the action.
  const answered =
    props.result && typeof props.result === "object" && "action" in props.result
      ? props.result
      : null;
  const settled = local ?? answered;
  const action: AssistantAction = settled
    ? settled.action
    : { ...proposed, preview: preview ?? proposed.preview };
  const secrets = settled?.secrets;
  const approval = props.approval;
  // Waiting on the server (the gate took the decision, no result yet).
  const deciding =
    !settled &&
    approval?.approved !== undefined &&
    props.status.type === "running";
  const rejectedByGate =
    !settled && approval?.approved === false && action.status === "proposed";
  const pending = action.status === "proposed" && !rejectedByGate && !deciding;

  const form = useMemo(() => action.form ?? [], [action.form]);
  const [values, setValues] = useState<Record<string, string | boolean>>(() => {
    const out: Record<string, string | boolean> = {};
    for (const f of form) {
      const v = (proposed.params ?? {})[f.key];
      out[f.key] = f.type === "boolean" ? Boolean(v) : toText(v);
    }
    return out;
  });
  const [invalid, setInvalid] = useState<string | null>(null);

  // Only the fields that differ from the proposal are sent.
  const edited = (): EditedParams | undefined | null => {
    const out: EditedParams = {};
    for (const f of form) {
      const next = readValue(f.type, values[f.key] ?? "");
      if (next === null) {
        setInvalid(f.label);
        return null;
      }
      const before = (proposed.params ?? {})[f.key];
      if (f.type === "secret") {
        if (next !== undefined) out[f.key] = next;
        continue;
      }
      if (JSON.stringify(next ?? null) !== JSON.stringify(before ?? null)) {
        out[f.key] = next === undefined ? null : next;
      }
    }
    setInvalid(null);
    return Object.keys(out).length ? out : undefined;
  };

  const thread = async () => remoteId ?? surface.ensureThread();

  const decideDirect = async (approve: boolean, params?: EditedParams) => {
    const id = await thread();
    if (!approve) {
      await assistantService.reject(surface.slug, id, action.id);
      setLocal({ action: { ...action, status: "rejected" } });
      return;
    }
    const out = await assistantService.confirm(
      surface.slug,
      id,
      action.id,
      locale,
      params,
    );
    const updated = out.actions?.find((a) => a.id === action.id) ?? {
      ...action,
      status: "done" as const,
    };
    setLocal({
      action: updated,
      ...(out.secret_once && Object.keys(out.secret_once).length
        ? { secrets: out.secret_once }
        : {}),
    });
  };

  const decide = async (approve: boolean) => {
    const params = approve ? edited() : undefined;
    if (params === null) return;
    setError(null);
    setBusy(true);
    try {
      const gateOpen =
        approval !== undefined &&
        approval.approved === undefined &&
        approval.resolution === undefined;
      if (gateOpen) {
        surface.decisions.push({
          actionId: action.id,
          approved: approve,
          params,
        });
        try {
          await props.respondToApproval({ approved: approve });
          return;
        } catch {
          surface.decisions.remove(action.id);
        }
      }
      await decideDirect(approve, params);
    } catch (err) {
      setError(errorKey(err));
    } finally {
      setBusy(false);
    }
  };

  const dryRun = async () => {
    const params = edited();
    if (params === null) return;
    setError(null);
    setBusy(true);
    try {
      const id = await thread();
      const out = await assistantService.dryRun(
        surface.slug,
        id,
        action.id,
        locale,
        params,
      );
      setPreview(out.preview);
    } catch (err) {
      setError(errorKey(err));
    } finally {
      setBusy(false);
    }
  };

  const readOnly = Object.entries(action.params ?? {}).filter(
    ([k, v]) =>
      !form.some((f) => f.key === k) &&
      v !== "" &&
      v !== null &&
      typeof v !== "object",
  );
  const destructive = action.risk === "destructive";
  const state: ConfirmationState = pending
    ? "requested"
    : action.status === "rejected" || rejectedByGate
      ? "rejected"
      : "accepted";
  const statusKey = rejectedByGate ? "rejected" : action.status;

  if (!pending && !deciding) {
    // A settled card: the AI Elements "Tool" record, open by default when it
    // has something to show (result, verification, secret, link).
    const hasOutput = Boolean(
      action.result ||
      action.verification ||
      action.details?.length ||
      secrets ||
      action.focus ||
      action.route ||
      action.undo,
    );
    return (
      <Tool defaultOpen={hasOutput} className="my-2">
        <ToolHeader
          title={action.title}
          state={toolState({ ...action, status: statusKey })}
          statusLabel={t(`assistant.action.status.${statusKey}`)}
        />
        <ToolContent>
          {action.summary ? (
            <p className="text-muted-foreground text-xs">{action.summary}</p>
          ) : null}
          <ToolInput
            input={Object.fromEntries(readOnly)}
            label={t("assistant.action.params")}
          />
          {action.status === "failed" ? (
            <ToolOutput
              errorText={action.result || t("assistant.errors.generic")}
            />
          ) : null}
          {action.status === "done" ? (
            <div className="space-y-2">
              {action.verification &&
              action.verification.status !== "skipped" ? (
                <p className="flex items-center gap-1 text-xs">
                  {action.verification.status === "ok" ? (
                    <CheckCircle2 className="size-3 text-emerald-600" />
                  ) : (
                    <XCircle className="text-destructive size-3" />
                  )}
                  {action.verification.status === "ok"
                    ? t("assistant.action.verified")
                    : t("assistant.action.not_verified")}
                  {action.verification.message
                    ? ` — ${action.verification.message}`
                    : ""}
                </p>
              ) : null}
              {action.result ? <ToolOutput output={action.result} /> : null}
              <Details items={action.details} />
              {secrets && Object.keys(secrets).length ? (
                <div
                  role="status"
                  className="rounded-md border border-amber-500/40 bg-amber-500/5 p-2 text-xs"
                >
                  <p className="font-medium">
                    {t("assistant.action.secret_once")}
                  </p>
                  {Object.entries(secrets).map(([k, v]) => (
                    <div key={k} className="mt-1">
                      <p className="text-muted-foreground">
                        {tOr(t, `assistant.fields.${k}`, k)}
                      </p>
                      <div className="flex items-center gap-1">
                        <code className="bg-muted min-w-0 flex-1 rounded px-1 py-0.5 break-all">
                          {v}
                        </code>
                        <CopyButton value={v} />
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}
              {action.focus ? (
                <RouteLink
                  route={action.focus.route}
                  target={action.focus.target}
                />
              ) : action.route ? (
                <RouteLink route={action.route} />
              ) : null}
              {action.undo ? (
                <p className="text-muted-foreground text-[11px]">
                  {t("assistant.action.undo", { how: action.undo })}
                </p>
              ) : null}
            </div>
          ) : null}
          {error ? (
            <p role="alert" className="text-destructive text-xs">
              {t(error)}
            </p>
          ) : null}
        </ToolContent>
      </Tool>
    );
  }

  return (
    <Confirmation state={state} destructive={destructive} className="my-2">
      <ConfirmationTitle>
        <p className="font-medium">{action.title}</p>
        {getStatusBadge(
          deciding ? "input-streaming" : "approval-requested",
          deciding
            ? t("assistant.action.applying")
            : t("assistant.action.status.proposed"),
        )}
      </ConfirmationTitle>
      {action.summary ? (
        <p className="text-muted-foreground -mt-1">{action.summary}</p>
      ) : null}
      <ConfirmationRequest className="space-y-3">
        {destructive ? (
          <p className="text-destructive flex items-center gap-1 text-xs">
            <AlertTriangle className="size-3" />
            {t("assistant.action.destructive")}
          </p>
        ) : null}
        {form.length ? (
          <div className="space-y-2">
            {form.map((f) => {
              const id = `${fieldId}-${f.key}`;
              const label = tOr(t, `assistant.fields.${f.key}`, f.label);
              return (
                <div key={f.key} className="space-y-1">
                  <label htmlFor={id} className="text-xs font-medium">
                    {label}
                    {f.required ? " *" : ""}
                  </label>
                  {f.type === "boolean" ? (
                    <div>
                      <Switch
                        id={id}
                        checked={Boolean(values[f.key])}
                        onCheckedChange={(v) =>
                          setValues((s) => ({ ...s, [f.key]: v }))
                        }
                      />
                    </div>
                  ) : f.type === "enum" ? (
                    <select
                      id={id}
                      className="border-input bg-background h-8 w-full rounded-md border px-2 text-sm"
                      value={String(values[f.key] ?? "")}
                      onChange={(e) =>
                        setValues((s) => ({ ...s, [f.key]: e.target.value }))
                      }
                    >
                      <option value="">—</option>
                      {(f.options ?? []).map((o) => (
                        <option key={o} value={o}>
                          {o}
                        </option>
                      ))}
                    </select>
                  ) : f.type === "json" ? (
                    <Textarea
                      id={id}
                      rows={4}
                      className="font-mono text-xs"
                      value={String(values[f.key] ?? "")}
                      onChange={(e) =>
                        setValues((s) => ({ ...s, [f.key]: e.target.value }))
                      }
                    />
                  ) : (
                    <Input
                      id={id}
                      type={
                        f.type === "secret"
                          ? "password"
                          : f.type === "number"
                            ? "number"
                            : f.type === "url"
                              ? "url"
                              : "text"
                      }
                      autoComplete={
                        f.type === "secret" ? "new-password" : "off"
                      }
                      placeholder={
                        f.type === "secret" && f.generated
                          ? t("assistant.action.secret_generated")
                          : undefined
                      }
                      value={String(values[f.key] ?? "")}
                      onChange={(e) =>
                        setValues((s) => ({ ...s, [f.key]: e.target.value }))
                      }
                    />
                  )}
                  {f.help && f.type !== "secret" ? (
                    <p className="text-muted-foreground text-[11px]">
                      {tOr(t, `assistant.fields_help.${f.key}`, f.help)}
                    </p>
                  ) : null}
                </div>
              );
            })}
            {invalid ? (
              <p role="alert" className="text-destructive text-xs">
                {t("assistant.action.invalid_json", { field: invalid })}
              </p>
            ) : null}
          </div>
        ) : readOnly.length ? (
          <ToolInput
            input={Object.fromEntries(readOnly)}
            label={t("assistant.action.params")}
          />
        ) : null}
        {action.preview ? (
          <div
            className={
              "rounded-md border p-2 text-xs " +
              (action.preview.ok
                ? "border-emerald-500/30 bg-emerald-500/5"
                : "border-destructive/30 bg-destructive/5")
            }
          >
            <p className="font-medium">
              {action.preview.ok
                ? t("assistant.action.preview_ok")
                : t("assistant.action.preview_failed")}
            </p>
            <p className="mt-0.5">{action.preview.summary}</p>
            <div className="mt-2">
              <Details items={action.preview.details} />
            </div>
          </div>
        ) : null}
      </ConfirmationRequest>
      <ConfirmationActions>
        <ConfirmationAction
          variant={destructive ? "destructive" : "default"}
          disabled={busy}
          onClick={() => void decide(true)}
        >
          {busy ? (
            <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />
          ) : (
            <Check className="size-4" />
          )}
          {t("assistant.action.confirm")}
        </ConfirmationAction>
        {action.dry_run ? (
          <ConfirmationAction
            variant="outline"
            disabled={busy}
            onClick={() => void dryRun()}
          >
            <FlaskConical className="size-4" />
            {t("assistant.action.dry_run")}
          </ConfirmationAction>
        ) : null}
        <ConfirmationAction
          variant="ghost"
          disabled={busy}
          onClick={() => void decide(false)}
        >
          <X className="size-4" />
          {t("assistant.action.reject")}
        </ConfirmationAction>
      </ConfirmationActions>
      {error ? (
        <p role="alert" className="text-destructive text-xs">
          {t(error)}
        </p>
      ) : null}
    </Confirmation>
  );
}
