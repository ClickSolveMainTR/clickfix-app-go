"use client";

import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  FEATURE_GROUPS,
  FEATURE_UNITS,
  GIB,
  featureGroup,
  isBytes,
} from "@/features/billing/components/platform/catalog/catalog-lib";
import { formatBytes, formatCount } from "@/features/billing/lib/money";
import { featureLabel } from "@/features/billing/lib/plans";
import type { BillingFeature, BillingGrant } from "@/features/billing/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

/** Unit word after a limit ("kredi", "dakika"); empty for unknown units. */
export function unitLabel(t: T, feature: BillingFeature) {
  if (isBytes(feature)) return "GB";
  const unit = feature.unit ?? "";
  return (FEATURE_UNITS as readonly string[]).includes(unit)
    ? t(`credit_products.feature_unit.${unit}`)
    : "";
}

/** "Sınırsız", "5 GB", "1.000 kredi", "Açık" / "Kapalı". */
export function grantValueText(
  t: T,
  locale: "tr" | "en",
  feature: BillingFeature,
  grant: BillingGrant,
) {
  if (feature.kind === "boolean")
    return grant.enabled
      ? t("credit_products.entitlements.on")
      : t("credit_products.entitlements.off");
  if (grant.unlimited) return t("billing.common.unlimited");
  const value = grant.limit ?? 0;
  if (isBytes(feature)) return formatBytes(value, locale);
  const unit = unitLabel(t, feature);
  return unit
    ? `${formatCount(value, locale)} ${unit}`
    : formatCount(value, locale);
}

/** Integer input; GB on screen for byte limits. */
function LimitInput({
  feature,
  grant,
  onChange,
  label,
}: {
  feature: BillingFeature;
  grant: BillingGrant;
  onChange: (limit: number) => void;
  label: string;
}) {
  const { t } = useLocale();
  const bytes = isBytes(feature);
  const shown = bytes
    ? Math.round(((grant.limit ?? 0) / GIB) * 100) / 100
    : (grant.limit ?? 0);
  const [text, setText] = useState(String(shown));
  const [prev, setPrev] = useState(shown);
  if (prev !== shown) {
    setPrev(shown);
    setText(String(shown));
  }
  const unit = unitLabel(t, feature);
  return (
    <div className="relative w-full sm:w-40">
      <Input
        type="number"
        min={0}
        step={bytes ? "any" : 1}
        inputMode={bytes ? "decimal" : "numeric"}
        aria-label={label}
        className={cn("h-8 text-end tabular-nums", unit && "pe-16")}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          const n = Number(e.target.value);
          if (!Number.isFinite(n) || n < 0) return;
          const limit = bytes ? Math.round(n * GIB) : n;
          if (Number.isSafeInteger(limit)) {
            setPrev(bytes ? Math.round((limit / GIB) * 100) / 100 : limit);
            onChange(limit);
          }
        }}
      />
      {unit ? (
        <span className="text-muted-foreground pointer-events-none absolute end-2.5 top-1/2 max-w-14 -translate-y-1/2 truncate text-xs">
          {unit}
        </span>
      ) : null}
    </div>
  );
}

function GrantRow({
  feature,
  grant,
  readOnly,
  filled,
  update,
}: {
  feature: BillingFeature;
  grant: BillingGrant;
  readOnly: boolean;
  filled: boolean;
  update: (patch: Partial<BillingGrant>) => void;
}) {
  const { t, locale } = useLocale();
  const label = featureLabel(feature, locale);
  const isLimit = feature.kind === "limit";
  return (
    <li className="grid gap-3 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:px-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,300px)_200px]">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
          {label}
          {filled ? (
            <Badge variant="warning" className="font-normal">
              {t("credit_products.entitlements.filled_badge")}
            </Badge>
          ) : null}
        </div>
        <p className="text-muted-foreground/80 truncate font-mono text-[11px]">
          {feature.key}
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-3 sm:justify-end lg:justify-start">
        {readOnly ? (
          <span
            className={cn(
              "text-sm tabular-nums",
              feature.kind === "boolean" &&
                !grant.enabled &&
                "text-muted-foreground",
            )}
          >
            {grantValueText(t, locale, feature, grant)}
          </span>
        ) : !isLimit ? (
          <label className="flex items-center gap-2 text-sm">
            <Switch
              checked={Boolean(grant.enabled)}
              aria-label={label}
              onCheckedChange={(v) => update({ enabled: v })}
            />
            <span className={cn(!grant.enabled && "text-muted-foreground")}>
              {grant.enabled
                ? t("credit_products.entitlements.on")
                : t("credit_products.entitlements.off")}
            </span>
          </label>
        ) : (
          <>
            <label className="flex items-center gap-2 text-sm">
              <Switch
                checked={Boolean(grant.unlimited)}
                aria-label={t("credit_products.entitlements.unlimited_for", {
                  feature: label,
                })}
                onCheckedChange={(v) =>
                  update({
                    unlimited: v,
                    limit: v ? null : (grant.limit ?? 0),
                  })
                }
              />
              {t("billing.common.unlimited")}
            </label>
            {grant.unlimited ? null : (
              <LimitInput
                feature={feature}
                grant={grant}
                label={label}
                onChange={(limit) => update({ limit })}
              />
            )}
          </>
        )}
      </div>

      <div className="flex items-center gap-2 sm:col-span-2 lg:col-span-1 lg:justify-end">
        {isLimit && !grant.unlimited ? (
          readOnly ? (
            <span className="text-muted-foreground text-xs">
              {t(
                `credit_products.entitlements.mode_${grant.enforcement ?? "hard"}`,
              )}
              {" · "}
              {t("credit_products.entitlements.warn_at", {
                percent: grant.warn_percent ?? 80,
              })}
            </span>
          ) : (
            <>
              <Select
                value={grant.enforcement ?? "hard"}
                onValueChange={(v) =>
                  update({ enforcement: v as "hard" | "soft" })
                }
              >
                <SelectTrigger
                  className="h-8 w-28"
                  aria-label={t("billing.versions.grant.enforcement")}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(["hard", "soft"] as const).map((v) => (
                    <SelectItem key={v} value={v}>
                      {t(`credit_products.entitlements.mode_${v}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="relative">
                <span className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 -translate-y-1/2 text-xs">
                  %
                </span>
                <Input
                  type="number"
                  min={1}
                  max={100}
                  aria-label={t("credit_products.entitlements.warn_label")}
                  title={t("credit_products.entitlements.warn_label")}
                  className="h-8 w-20 ps-6 text-end tabular-nums"
                  value={grant.warn_percent ?? 80}
                  onChange={(e) => {
                    const n = Number(e.target.value);
                    if (Number.isInteger(n) && n >= 1 && n <= 100)
                      update({ warn_percent: n });
                  }}
                />
              </div>
            </>
          )
        ) : null}
      </div>
    </li>
  );
}

/**
 * Every catalog feature of a version, grouped into readable sections (AI,
 * channels, team, video, automation, other). Limits get an "unlimited"
 * switch plus a number with its unit; on/off features a switch.
 */
export function EntitlementsEditor({
  features,
  grants,
  onChange,
  readOnly,
  filled = [],
}: {
  features: BillingFeature[];
  grants: BillingGrant[];
  onChange: (grants: BillingGrant[]) => void;
  readOnly: boolean;
  /** Keys prefilled from the catalog default (absent in the base version). */
  filled?: string[];
}) {
  const { t } = useLocale();
  const byKey = new Map(grants.map((g) => [g.feature, g]));
  const filledSet = new Set(filled);
  const update = (key: string, patch: Partial<BillingGrant>) =>
    onChange(grants.map((g) => (g.feature === key ? { ...g, ...patch } : g)));

  const sections = FEATURE_GROUPS.map((group) => ({
    group,
    items: features.filter(
      (f) => featureGroup(f.key) === group && byKey.has(f.key),
    ),
  })).filter((s) => s.items.length > 0);

  return (
    <div className="space-y-4">
      <div className="bg-muted/40 text-muted-foreground grid gap-1 rounded-lg px-3 py-2.5 text-xs sm:grid-cols-2">
        <p>
          <span className="text-foreground font-medium">
            {t("credit_products.entitlements.mode_hard")}:
          </span>{" "}
          {t("credit_products.entitlements.hard_explain")}
        </p>
        <p>
          <span className="text-foreground font-medium">
            {t("credit_products.entitlements.mode_soft")}:
          </span>{" "}
          {t("credit_products.entitlements.soft_explain")}
        </p>
        <p className="sm:col-span-2">
          {t("credit_products.entitlements.warn_explain")}
        </p>
      </div>
      {sections.map(({ group, items }) => (
        <section
          key={group}
          className="overflow-hidden rounded-lg border"
          aria-labelledby={`ent-${group}`}
        >
          <h4
            id={`ent-${group}`}
            className="bg-muted/40 flex items-center justify-between px-3 py-2 text-xs font-semibold tracking-wide uppercase sm:px-4"
          >
            {t(`credit_products.entitlements.group.${group}`)}
            <span className="text-muted-foreground font-normal normal-case">
              {items.length}
            </span>
          </h4>
          <ul className="divide-y">
            {items.map((feature) => (
              <GrantRow
                key={feature.key}
                feature={feature}
                grant={byKey.get(feature.key) as BillingGrant}
                readOnly={readOnly}
                filled={filledSet.has(feature.key)}
                update={(patch) => update(feature.key, patch)}
              />
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
