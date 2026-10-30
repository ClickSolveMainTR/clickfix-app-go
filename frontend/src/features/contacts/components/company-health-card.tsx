"use client";

import { HeartPulse, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useCompanyHealth,
  useRecomputeHealth,
} from "../hooks/use-company-health";
import { useContactsScope } from "../hooks/use-contacts";
import { healthTone } from "./company-health-badge";

const SIGNALS = [
  "csat",
  "open_conversations",
  "sla_breaches",
  "frequency_trend",
  "billing",
  "usage",
] as const;

export type CompanyHealthCardProps = {
  companyUuid: string;
  className?: string;
};

/**
 * Customer health of a company (CF-87): the score, each signal's sub-score
 * and weight, the recent trend and an on-demand recompute. Used on the
 * companies page and the account screen (CF-85).
 */
export function CompanyHealthCard({
  companyUuid,
  className,
}: CompanyHealthCardProps) {
  const { t, locale } = useLocale();
  const scope = useContactsScope();
  const q = useCompanyHealth(companyUuid);
  const recompute = useRecomputeHealth(companyUuid);
  const h = q.data;
  const history = [...(h?.history ?? [])].reverse();

  return (
    <section className={cn("bg-card rounded-xl border p-4", className)}>
      <header className="mb-3 flex items-center gap-2">
        <HeartPulse className="text-muted-foreground size-4" />
        <h3 className="text-sm font-medium">{t("contacts.health.title")}</h3>
        {scope.canWrite && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="ml-auto h-7"
            disabled={recompute.isPending}
            onClick={() => recompute.mutate()}
          >
            <RefreshCw
              className={cn("size-3.5", recompute.isPending && "animate-spin")}
            />
            {t("contacts.health.recompute")}
          </Button>
        )}
      </header>
      {q.isLoading ? (
        <Skeleton className="h-24 rounded-lg" />
      ) : !h || h.score == null ? (
        <p className="text-muted-foreground text-sm">
          {t("contacts.health.empty")}
        </p>
      ) : (
        <div className="grid gap-3">
          <div className="flex items-center gap-3">
            <span
              className={cn(
                "flex size-12 items-center justify-center rounded-xl text-lg font-semibold tabular-nums",
                healthTone(h.score, h.below_threshold),
              )}
            >
              {h.score}
            </span>
            <div className="min-w-0 text-xs">
              <p className="font-medium">
                {h.below_threshold
                  ? t("contacts.health.at_risk")
                  : t("contacts.health.ok")}
              </p>
              <p className="text-muted-foreground">
                {t("contacts.health.threshold", { threshold: h.threshold })}
              </p>
              {h.computed_at && (
                <p className="text-muted-foreground">
                  {t("contacts.health.computed_at", {
                    date: new Date(h.computed_at).toLocaleString(
                      locale === "tr" ? "tr-TR" : "en-US",
                      { dateStyle: "medium", timeStyle: "short" },
                    ),
                  })}
                </p>
              )}
            </div>
            {history.length > 1 && (
              <Sparkline
                points={history.map((p) => p.score)}
                className="ml-auto"
              />
            )}
          </div>
          <ul className="grid gap-1.5">
            {SIGNALS.map((key) => {
              const s = h.signals[key];
              if (!s) return null;
              return (
                <li
                  key={key}
                  className={cn(
                    "grid grid-cols-[1fr_6rem_2rem] items-center gap-2 text-xs",
                    !s.used && "text-muted-foreground",
                  )}
                >
                  <span className="truncate">
                    {t(`contacts.health.signal.${key}`)}
                    <span className="text-muted-foreground">
                      {" "}
                      · {t("contacts.health.weight", { weight: s.weight })}
                    </span>
                  </span>
                  <span className="bg-muted h-1.5 overflow-hidden rounded-full">
                    {s.score != null && (
                      <span
                        className={cn(
                          "block h-full rounded-full",
                          s.score < 40
                            ? "bg-destructive"
                            : s.score < 70
                              ? "bg-amber-500"
                              : "bg-emerald-500",
                        )}
                        style={{ width: `${s.score}%` }}
                      />
                    )}
                  </span>
                  <span className="text-right tabular-nums">
                    {s.score ?? "—"}
                  </span>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </section>
  );
}

function Sparkline({
  points,
  className,
}: {
  points: number[];
  className?: string;
}) {
  const w = 96;
  const hgt = 28;
  const step = w / Math.max(points.length - 1, 1);
  const d = points
    .map(
      (p, i) =>
        `${i === 0 ? "M" : "L"}${(i * step).toFixed(1)},${(hgt - (p / 100) * hgt).toFixed(1)}`,
    )
    .join(" ");
  return (
    <svg
      viewBox={`0 0 ${w} ${hgt}`}
      width={w}
      height={hgt}
      className={cn("text-muted-foreground shrink-0", className)}
      aria-hidden
    >
      <path d={d} fill="none" stroke="currentColor" strokeWidth={1.5} />
    </svg>
  );
}
