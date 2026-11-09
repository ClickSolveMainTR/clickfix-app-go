"use client";

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
import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

import { focusHref, readValue, toText } from "../lib/card";
import type {
  AssistantAction,
  AssistantActionDetail,
} from "../services/assistant.service";

/** Fields edited in the card, sent with dry-run / confirm. */
export type EditedParams = Record<string, unknown>;

export type ActionDecision = {
  action: AssistantAction;
  confirm: boolean;
  params?: EditedParams;
};

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
    <div className="mt-2 space-y-2 text-xs">
      {items.map((d, i) => (
        <div key={`${d.label}-${i}`}>
          <p className="text-muted-foreground">{d.label}</p>
          {d.value ? <p className="break-all">{d.value}</p> : null}
          {d.code ? (
            <div className="bg-muted relative mt-1 rounded-md">
              <pre className="overflow-x-auto p-2 pr-16 font-mono text-[11px] whitespace-pre">
                {d.code}
              </pre>
              <div className="absolute top-0 right-0">
                <CopyButton value={d.code} />
              </div>
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

/**
 * Confirmation card v2 (CF-184): editable params, dry run, verification
 * after apply, a secret shown once, "go to the page and highlight".
 * Every value is checked again by the server; the form is only a draft.
 */
export function ActionCard({
  action,
  busy,
  secrets,
  onDecide,
  onDryRun,
}: {
  action: AssistantAction;
  busy: boolean;
  /** Secrets returned by confirm; kept in memory only. */
  secrets?: Record<string, string>;
  onDecide: (d: ActionDecision) => void;
  onDryRun: (action: AssistantAction, params?: EditedParams) => void;
}) {
  const { t } = useLocale();
  const form = useMemo(() => action.form ?? [], [action.form]);
  const pending = action.status === "proposed";
  const [values, setValues] = useState<Record<string, string | boolean>>(() => {
    const out: Record<string, string | boolean> = {};
    for (const f of form) {
      const v = (action.params ?? {})[f.key];
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
      const before = (action.params ?? {})[f.key];
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

  const readOnly = Object.entries(action.params ?? {}).filter(
    ([k, v]) =>
      !form.some((f) => f.key === k) &&
      v !== "" &&
      v !== null &&
      typeof v !== "object",
  );
  const destructive = action.risk === "destructive";

  return (
    <div
      className={
        "bg-card mt-2 w-[90%] rounded-xl border p-3 text-sm shadow-xs" +
        (destructive ? " border-destructive/40" : "")
      }
    >
      <div className="flex items-start justify-between gap-2">
        <p className="font-medium">{action.title}</p>
        <Badge variant={action.status === "failed" ? "danger" : "secondary"}>
          {t(`assistant.action.status.${action.status}`)}
        </Badge>
      </div>
      {action.summary ? (
        <p className="text-muted-foreground mt-1">{action.summary}</p>
      ) : null}
      {destructive && pending ? (
        <p className="text-destructive mt-2 flex items-center gap-1 text-xs">
          <AlertTriangle className="size-3" />
          {t("assistant.action.destructive")}
        </p>
      ) : null}

      {pending && form.length ? (
        <div className="mt-3 space-y-2">
          {form.map((f) => {
            const id = `${action.id}-${f.key}`;
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
                    autoComplete={f.type === "secret" ? "new-password" : "off"}
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
                  <p className="text-muted-foreground text-[11px]">{f.help}</p>
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
        <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-xs">
          {readOnly.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-muted-foreground">{k}</dt>
              <dd className="truncate">{String(v)}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      {action.preview ? (
        <div
          className={
            "mt-3 rounded-md border p-2 text-xs " +
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
          <Details items={action.preview.details} />
        </div>
      ) : null}

      {pending ? (
        <div className="mt-3 flex flex-wrap gap-2">
          <Button
            size="sm"
            variant={destructive ? "destructive" : "default"}
            disabled={busy}
            onClick={() => {
              const params = edited();
              if (params === null) return;
              onDecide({ action, confirm: true, params });
            }}
          >
            {busy ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Check className="size-4" />
            )}
            {t("assistant.action.confirm")}
          </Button>
          {action.dry_run ? (
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => {
                const params = edited();
                if (params === null) return;
                onDryRun(action, params);
              }}
            >
              <FlaskConical className="size-4" />
              {t("assistant.action.dry_run")}
            </Button>
          ) : null}
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            onClick={() => onDecide({ action, confirm: false })}
          >
            <X className="size-4" />
            {t("assistant.action.reject")}
          </Button>
        </div>
      ) : null}

      {action.status === "failed" && action.result ? (
        <p className="text-destructive mt-2 text-xs">{action.result}</p>
      ) : null}
      {action.status === "done" ? (
        <>
          {action.verification && action.verification.status !== "skipped" ? (
            <p className="mt-2 flex items-center gap-1 text-xs">
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
          <Details items={action.details} />
          {secrets && Object.keys(secrets).length ? (
            <div
              role="status"
              className="mt-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-2 text-xs"
            >
              <p className="font-medium">{t("assistant.action.secret_once")}</p>
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
        </>
      ) : null}
      {action.undo && action.status === "done" ? (
        <p className="text-muted-foreground mt-1 text-[11px]">
          {t("assistant.action.undo", { how: action.undo })}
        </p>
      ) : null}
    </div>
  );
}
