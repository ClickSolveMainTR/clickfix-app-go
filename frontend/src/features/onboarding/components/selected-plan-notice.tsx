"use client";

import { Sparkles } from "lucide-react";
import Link from "next/link";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

import type { SelectedPlan } from "../hooks/use-open-organization";

/**
 * The plan picked on the pricing page (CF-42): its trial, "pay now", and
 * links to change or drop it. Shown on `/register` and on the organization form.
 */
export function SelectedPlanNotice({
  selected,
  noTrial,
  payNow,
  onPayNowChange,
  onClear,
}: {
  selected: SelectedPlan;
  noTrial: boolean;
  payNow: boolean;
  onPayNowChange: (value: boolean) => void;
  onClear: () => void;
}) {
  const { t } = useLocale();
  return (
    <div className="bg-muted/40 space-y-2 rounded-lg border p-3 text-sm">
      <p className="flex items-center gap-2 font-medium">
        <Sparkles className="text-primary size-4" aria-hidden />
        {t("onboarding.create.plan_selected", { plan: selected.name })}
      </p>
      <p className="text-muted-foreground text-xs">
        {noTrial
          ? t("onboarding.create.plan_no_trial")
          : t("onboarding.create.plan_trial", { days: selected.trialDays })}
      </p>
      {!noTrial ? (
        <Label className="flex items-start gap-2 text-xs font-normal">
          <Checkbox
            checked={payNow}
            onCheckedChange={(v) => onPayNowChange(v === true)}
            className="mt-0.5"
          />
          {t("onboarding.create.pay_now")}
        </Label>
      ) : null}
      <div className="flex gap-3 text-xs">
        <Link
          href={`${routes.public.landing}#pricing`}
          className="text-primary hover:underline"
        >
          {t("onboarding.create.plan_change")}
        </Link>
        <button
          type="button"
          className="text-muted-foreground hover:underline"
          onClick={onClear}
        >
          {t("onboarding.create.plan_clear")}
        </button>
      </div>
    </div>
  );
}
