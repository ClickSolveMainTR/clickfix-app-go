"use client";

import { formatBytes, formatCount } from "@/features/billing/lib/money";
import type {
  BillingUsageSummary,
  EntitlementValue,
} from "@/features/billing/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

function display(feature: string, value: number, locale: "tr" | "en") {
  return feature === "storage.bytes"
    ? formatBytes(value, locale)
    : formatCount(value, locale);
}

/**
 * Usage vs limit for every limit entitlement: monthly sums reset each UTC
 * month, gauges (seats, channels, storage) are current counts. Colours
 * follow the grant's warn threshold; over the limit is destructive.
 */
export function UsageMeters({
  values,
  usage,
  labels,
  className,
}: {
  values: Record<string, EntitlementValue>;
  usage: BillingUsageSummary[];
  labels: Map<string, string>;
  className?: string;
}) {
  const { t, locale } = useLocale();
  // Rows come per period, newest (current) first: keep the first per key.
  const used = new Map<string, number>();
  for (const u of usage) if (!used.has(u.key)) used.set(u.key, u.quantity);
  const limits = Object.values(values)
    .filter((v) => v.kind === "limit")
    .sort((a, b) => a.feature.localeCompare(b.feature));

  if (limits.length === 0) return null;

  return (
    <ul className={cn("grid gap-3 sm:grid-cols-2", className)}>
      {limits.map((v) => {
        const quantity = used.get(v.feature) ?? 0;
        const pct = v.unlimited
          ? 0
          : v.limit > 0
            ? Math.min(100, Math.round((quantity / v.limit) * 100))
            : quantity > 0
              ? 100
              : 0;
        const over = !v.unlimited && quantity > v.limit;
        const warn = !v.unlimited && pct >= (v.warn_percent ?? 80);
        return (
          <li key={v.feature} className="rounded-lg border p-3">
            <div className="flex items-baseline justify-between gap-2 text-sm">
              <span className="truncate font-medium">
                {labels.get(v.feature) ?? v.feature}
              </span>
              <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
                {display(v.feature, quantity, locale)} /{" "}
                {v.unlimited
                  ? t("billing.common.unlimited")
                  : display(v.feature, v.limit, locale)}
              </span>
            </div>
            <div
              role="meter"
              aria-label={labels.get(v.feature) ?? v.feature}
              aria-valuemin={0}
              aria-valuemax={v.unlimited ? undefined : v.limit}
              aria-valuenow={quantity}
              className="bg-muted mt-2 h-1.5 overflow-hidden rounded-full"
            >
              <div
                className={cn(
                  "h-full rounded-full transition-[width]",
                  over
                    ? "bg-destructive"
                    : warn
                      ? "bg-amber-500"
                      : "bg-primary",
                  v.unlimited && "bg-primary/30",
                )}
                style={{ width: `${v.unlimited ? 100 : pct}%` }}
              />
            </div>
            <p className="text-muted-foreground mt-1.5 text-xs">
              {v.period === "month"
                ? t("billing.usage.monthly")
                : t("billing.usage.current")}
              {over
                ? ` · ${t(v.enforcement === "soft" ? "billing.usage.over_soft" : "billing.usage.over_hard")}`
                : ""}
            </p>
          </li>
        );
      })}
    </ul>
  );
}
