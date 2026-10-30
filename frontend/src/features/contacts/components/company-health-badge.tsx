"use client";

import { HeartPulse } from "lucide-react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { CompanyHealthSummary } from "../services/health.service";

export type CompanyHealthBadgeProps = {
  /** `Company.health` from the companies API; null renders nothing. */
  health:
    Pick<CompanyHealthSummary, "score" | "below_threshold"> | null | undefined;
  className?: string;
};

/** Colour band of a 0-100 health score. */
export function healthTone(score: number | null | undefined, below?: boolean) {
  if (score == null) return "bg-muted text-muted-foreground";
  if (below || score < 40) return "bg-destructive/10 text-destructive";
  if (score < 70) return "bg-amber-500/10 text-amber-700 dark:text-amber-400";
  return "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400";
}

/** Small pill with the customer health score (CF-87). */
export function CompanyHealthBadge({
  health,
  className,
}: CompanyHealthBadgeProps) {
  const { t } = useLocale();
  if (!health || health.score == null) return null;
  return (
    <span
      title={
        health.below_threshold
          ? t("contacts.health.at_risk")
          : t("contacts.health.title")
      }
      className={cn(
        "inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium tabular-nums",
        healthTone(health.score, health.below_threshold),
        className,
      )}
    >
      <HeartPulse className="size-3" />
      {health.score}
    </span>
  );
}
