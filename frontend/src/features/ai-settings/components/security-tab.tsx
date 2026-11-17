"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  EyeOff,
  Gauge,
  Save,
  ShieldAlert,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";
import { useState, type ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { aiErrorMessage } from "../lib/errors";
import { formatMicros } from "../lib/format";
import {
  aiSettingsService,
  type AIPolicy,
  type AIPolicyInput,
} from "../services/ai-settings.service";
import { QueryState, SettingRow } from "./parts";
import type { AISettingsTabProps } from "./tab-registry";

export const policyKey = (slug: string) =>
  ["tenant", slug, "ai", "policy"] as const;

/**
 * What the backend masks before a provider call, in rule order
 * (`backend/internal/platform/ai/guard.go`, `piiRules`). Shown verbatim so
 * admins see the real list; masking itself never runs in the browser.
 */
const PII_KINDS = ["IBAN", "EMAIL", "CARD", "TCKN", "PHONE"] as const;

const MAX_BUDGET_MICROS = 1_000_000_000_000;

type Limit = AIPolicy["effective_limits"]["daily_calls"];

/** "" follows the platform limit (null); anything else must be a number. */
function parseOptional(raw: string): number | null {
  const trimmed = raw.trim().replace(",", ".");
  return trimmed === "" ? null : Number(trimmed);
}

function fieldText(value: number | null | undefined, scale = 1): string {
  return value == null ? "" : String(value / scale);
}

function toInput(policy: AIPolicy): AIPolicyInput {
  return {
    enabled: policy.enabled,
    mask_pii: policy.mask_pii,
    allow_unmask: policy.allow_unmask,
    daily_calls: policy.daily_calls,
    concurrency: policy.concurrency,
    daily_budget_micros: policy.daily_budget_micros,
    batch_share_percent: policy.batch_share_percent,
  };
}

function ToggleRow({
  id,
  label,
  hint,
  checked,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  hint: ReactNode;
  checked: boolean;
  disabled?: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="space-y-1">
        <Label htmlFor={id}>{label}</Label>
        <div className="text-muted-foreground text-xs">{hint}</div>
      </div>
      <Switch
        id={id}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onChange}
      />
    </div>
  );
}

function Explainer({
  icon: Icon,
  title,
  children,
}: {
  icon: LucideIcon;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="flex gap-3">
      <Icon className="text-muted-foreground mt-0.5 size-4 shrink-0" />
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        <div className="text-muted-foreground space-y-1 text-xs leading-relaxed">
          {children}
        </div>
      </div>
    </div>
  );
}

/**
 * "Güvenlik" tab (CF-71): AI on/off, PII masking and unmasking, daily call /
 * concurrency / budget limits, plus a plain description of the prompt
 * injection guard. Budget is edited in USD and stored in micros.
 */
export function SecurityTab({ slug, canWrite }: AISettingsTabProps) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: policyKey(slug),
    queryFn: () => aiSettingsService.policy(slug),
  });
  const [form, setForm] = useState<AIPolicyInput | null>(null);
  const [calls, setCalls] = useState("");
  const [concurrency, setConcurrency] = useState("");
  const [budget, setBudget] = useState("");
  const [share, setShare] = useState("");
  const [saving, setSaving] = useState(false);
  const reset = (policy: AIPolicy) => {
    setForm(toInput(policy));
    setCalls(fieldText(policy.daily_calls));
    setConcurrency(fieldText(policy.concurrency));
    setBudget(fieldText(policy.daily_budget_micros, 1_000_000));
    setShare(String(policy.batch_share_percent));
  };
  if (query.data && form === null) {
    reset(query.data);
  }
  if (!form || !query.data) {
    return (
      <QueryState
        loading={query.isLoading}
        error={query.isError}
        onRetry={() => void query.refetch()}
      />
    );
  }

  const readOnly = !canWrite;
  const set = <K extends keyof AIPolicyInput>(
    key: K,
    value: AIPolicyInput[K],
  ) => setForm((prev) => (prev ? { ...prev, [key]: value } : prev));
  // CF-229: an empty limit follows the platform limit; a value can only
  // lower it, so the effective limit is the lower of the two.
  const callsValue = parseOptional(calls);
  const concurrencyValue = parseOptional(concurrency);
  const budgetValue = parseOptional(budget);
  const budgetMicros =
    budgetValue === null ? null : Math.round(budgetValue * 1_000_000);
  const errors = {
    calls:
      callsValue !== null &&
      (!Number.isInteger(callsValue) || callsValue < 1 || callsValue > 100_000),
    concurrency:
      concurrencyValue !== null &&
      (!Number.isInteger(concurrencyValue) ||
        concurrencyValue < 1 ||
        concurrencyValue > 20),
    budget:
      budgetMicros !== null &&
      (!Number.isFinite(budgetMicros) ||
        budgetMicros < 1 ||
        budgetMicros > MAX_BUDGET_MICROS),
  };
  // CF-228: batch jobs may use this share of the daily limits.
  const shareValue = Number(share.trim());
  const shareInvalid =
    !Number.isInteger(shareValue) || shareValue < 10 || shareValue > 90;
  const invalid =
    errors.calls || errors.concurrency || errors.budget || shareInvalid;
  const next: AIPolicyInput = {
    ...form,
    daily_calls: callsValue,
    concurrency: concurrencyValue,
    daily_budget_micros: budgetMicros,
    batch_share_percent: shareValue,
  };
  const dirty =
    JSON.stringify(next) !== JSON.stringify(toInput(query.data)) && !invalid;
  const platform = query.data.platform_limits;
  const effective = query.data.effective_limits;
  const batch = query.data.batch_limits;
  const limitText = (limit: Limit, format?: (n: number) => string) =>
    limit.unlimited || limit.limit == null
      ? t("ai_settings.security.unlimited")
      : format
        ? format(limit.limit)
        : limit.limit.toLocaleString(locale);
  const limitHint = (
    range: string,
    key: "daily_calls" | "concurrency" | "daily_budget_micros",
    format?: (n: number) => string,
  ) => (
    <>
      <span className="block">{range}</span>
      <span className="block">
        {t("ai_settings.security.platform_limit", {
          value: limitText(platform[key], format),
        })}
        {" · "}
        {t("ai_settings.security.effective_limit", {
          value: limitText(effective[key], format),
        })}
      </span>
    </>
  );
  const money = (micros: number) => formatMicros(micros, locale);

  const save = async () => {
    setSaving(true);
    try {
      const saved = await aiSettingsService.savePolicy(slug, next);
      qc.setQueryData(policyKey(slug), saved);
      reset(saved);
      appToast.success(t("ai_settings.security.saved"));
    } catch (error) {
      appToast.error(aiErrorMessage(error, t));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div className="min-w-0 space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex flex-wrap items-center gap-2 text-base">
              {t("ai_settings.security.status_title")}
              <Badge variant={form.enabled ? "success" : "secondary"}>
                {form.enabled
                  ? t("ai_settings.security.on")
                  : t("ai_settings.security.off")}
              </Badge>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <ToggleRow
              id="ai-enabled"
              label={t("ai_settings.security.enabled")}
              hint={t("ai_settings.security.enabled_hint")}
              checked={form.enabled}
              disabled={readOnly}
              onChange={(value) => set("enabled", value)}
            />
            <ToggleRow
              id="ai-mask"
              label={t("ai_settings.security.mask_pii")}
              hint={
                <>
                  <p>{t("ai_settings.security.mask_pii_hint")}</p>
                  {!form.mask_pii && (
                    <p className="text-destructive mt-1">
                      {t("ai_settings.security.mask_off_warning")}
                    </p>
                  )}
                </>
              }
              checked={form.mask_pii}
              disabled={readOnly}
              onChange={(value) => set("mask_pii", value)}
            />
            <ToggleRow
              id="ai-unmask"
              label={t("ai_settings.security.allow_unmask")}
              hint={t("ai_settings.security.allow_unmask_hint")}
              checked={form.allow_unmask}
              disabled={readOnly}
              onChange={(value) => set("allow_unmask", value)}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Gauge className="size-4" />
              {t("ai_settings.security.limits_title")}
            </CardTitle>
            <p className="text-muted-foreground text-sm">
              {t("ai_settings.security.limits_hint")}
            </p>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-3">
            <SettingRow
              label={t("ai_settings.security.daily_calls")}
              hint={limitHint(
                t("ai_settings.security.daily_calls_hint"),
                "daily_calls",
              )}
              htmlFor="ai-daily-calls"
            >
              <Input
                id="ai-daily-calls"
                type="number"
                min={1}
                max={100000}
                value={calls}
                placeholder={t("ai_settings.security.follow_platform")}
                disabled={readOnly}
                aria-invalid={errors.calls}
                onChange={(e) => setCalls(e.target.value)}
              />
            </SettingRow>
            <SettingRow
              label={t("ai_settings.security.concurrency")}
              hint={limitHint(
                t("ai_settings.security.concurrency_hint"),
                "concurrency",
              )}
              htmlFor="ai-concurrency"
            >
              <Input
                id="ai-concurrency"
                type="number"
                min={1}
                max={20}
                value={concurrency}
                placeholder={t("ai_settings.security.follow_platform")}
                disabled={readOnly}
                aria-invalid={errors.concurrency}
                onChange={(e) => setConcurrency(e.target.value)}
              />
            </SettingRow>
            <SettingRow
              label={t("ai_settings.security.daily_budget")}
              hint={limitHint(
                errors.budget
                  ? t("ai_settings.security.daily_budget_invalid")
                  : budgetMicros === null
                    ? t("ai_settings.security.daily_budget_follow")
                    : t("ai_settings.security.daily_budget_hint", {
                        value: money(budgetMicros),
                      }),
                "daily_budget_micros",
                money,
              )}
              htmlFor="ai-budget"
            >
              <div className="relative">
                <span className="text-muted-foreground pointer-events-none absolute inset-y-0 start-3 flex items-center text-sm">
                  $
                </span>
                <Input
                  id="ai-budget"
                  inputMode="decimal"
                  className="ps-7"
                  value={budget}
                  placeholder={t("ai_settings.security.follow_platform")}
                  disabled={readOnly}
                  aria-invalid={errors.budget}
                  onChange={(e) => setBudget(e.target.value)}
                />
              </div>
            </SettingRow>
            <SettingRow
              label={t("ai_settings.security.batch_share")}
              hint={
                <>
                  <span className="block">
                    {shareInvalid
                      ? t("ai_settings.security.batch_share_invalid")
                      : t("ai_settings.security.batch_share_hint")}
                  </span>
                  <span className="block">
                    {t("ai_settings.security.batch_limits", {
                      calls: limitText(batch.daily_calls),
                      budget: limitText(batch.daily_budget_micros, money),
                    })}
                  </span>
                </>
              }
              htmlFor="ai-batch-share"
            >
              <div className="relative">
                <Input
                  id="ai-batch-share"
                  type="number"
                  min={10}
                  max={90}
                  className="pe-7"
                  value={share}
                  disabled={readOnly}
                  aria-invalid={shareInvalid}
                  onChange={(e) => setShare(e.target.value)}
                />
                <span className="text-muted-foreground pointer-events-none absolute inset-y-0 end-3 flex items-center text-sm">
                  %
                </span>
              </div>
            </SettingRow>
          </CardContent>
        </Card>

        {canWrite && (
          <div className="bg-background/95 sticky bottom-0 z-10 -mx-1 flex flex-wrap items-center justify-end gap-2 border-t px-1 py-3 backdrop-blur">
            <span className="text-muted-foreground me-auto text-xs">
              {dirty
                ? t("ai_settings.unsaved")
                : query.data.updated_at
                  ? t("ai_settings.security.updated", {
                      date: datetime(query.data.updated_at, undefined, locale),
                    })
                  : t("ai_settings.security.defaults")}
            </span>
            <Button
              variant="outline"
              disabled={!dirty || saving}
              onClick={() => reset(query.data)}
            >
              {t("ai_settings.discard")}
            </Button>
            <Button
              disabled={!dirty || invalid || saving}
              onClick={() => void save()}
            >
              <Save className="size-4" />
              {saving ? t("ai_settings.saving") : t("ai_settings.save")}
            </Button>
          </div>
        )}
      </div>

      <Card className="h-fit">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="size-4" />
            {t("ai_settings.security.how_title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <Explainer
            icon={EyeOff}
            title={t("ai_settings.security.masked_title")}
          >
            <p>{t("ai_settings.security.masked_intro")}</p>
            <ul className="space-y-1.5 pt-1">
              {PII_KINDS.map((kind) => (
                <li key={kind} className="flex items-start gap-2">
                  <Badge variant="outline" className="shrink-0 font-mono">
                    {kind}
                  </Badge>
                  <span>{t(`ai_settings.security.pii.${kind}`)}</span>
                </li>
              ))}
            </ul>
            <p className="pt-1">{t("ai_settings.security.masked_token")}</p>
            <p>{t("ai_settings.security.masked_audit")}</p>
          </Explainer>
          <Explainer
            icon={ShieldAlert}
            title={t("ai_settings.security.injection_title")}
          >
            <p>{t("ai_settings.security.injection_data")}</p>
            <p>{t("ai_settings.security.injection_guard")}</p>
            <p>{t("ai_settings.security.injection_tools")}</p>
            <p>{t("ai_settings.security.injection_preferences")}</p>
          </Explainer>
        </CardContent>
      </Card>
    </div>
  );
}
