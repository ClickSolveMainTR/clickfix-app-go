"use client";

import { StatusChip } from "@/components/common/status-chip";
import { BillingStatus } from "@/features/billing/components/billing-status";
import { daysUntil } from "@/features/billing/components/platform/org/plan-options";
import {
  ACCESS_LABEL_KEYS,
  ACCESS_TONE,
  STATUS_LABEL_KEYS,
  STATUS_TONE,
} from "@/features/platform-organizations/constants";
import type {
  OrganizationAccessState,
  OrganizationStatus,
  OrganizationSummary,
} from "@/features/platform-organizations/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function OrganizationStatusChip({
  status,
}: {
  status: OrganizationStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(STATUS_LABEL_KEYS[status])}
      tone={STATUS_TONE[status]}
    />
  );
}

export function OrganizationAccessChip({
  state,
}: {
  state: OrganizationAccessState;
}) {
  const { t } = useLocale();
  return (
    <StatusChip label={t(ACCESS_LABEL_KEYS[state])} tone={ACCESS_TONE[state]} />
  );
}

export function ownerLabel(owner: {
  name: string;
  surname: string;
  email: string;
}) {
  return `${owner.name} ${owner.surname}`.trim() || owner.email;
}

/** Days left of the trial, or `null` when the organization is not trialing. */
export function trialDaysLeft(
  org: Pick<OrganizationSummary, "subscription_status" | "trial_ends_at">,
  now = Date.now(),
) {
  if (org.subscription_status !== "trialing") return null;
  return daysUntil(org.trial_ends_at, now);
}

/** "Deneme 12 gün sonra bitiyor" / renewal date line for a summary row. */
export function useSubscriptionTiming() {
  const { t, locale } = useLocale();
  return (org: OrganizationSummary) => {
    const left = trialDaysLeft(org);
    if (left !== null) {
      return {
        urgent: left <= 3,
        text:
          left === 0
            ? t("commerce.org.trial.ends_today")
            : left === 1
              ? t("commerce.org.trial.ends_in_one_day")
              : t("commerce.org.trial.ends_in", { count: left }),
      };
    }
    const ended =
      org.subscription_status === "canceled" ||
      org.subscription_status === "expired";
    if (!org.period_end || ended || !org.subscription_status) return null;
    return {
      urgent: false,
      text: t("commerce.org.list.period_end", {
        date: date(org.period_end, undefined, locale),
      }),
    };
  };
}

/**
 * Plan name with the subscription status chip and one line of timing:
 * "Deneme 12 gün sonra bitiyor" while trialing, the period end otherwise.
 */
export function OrganizationPlanCell({ org }: { org: OrganizationSummary }) {
  const { t } = useLocale();
  const timingOf = useSubscriptionTiming();
  const plan = org.plan_name || org.plan_code;
  if (!plan) {
    return (
      <span className="text-muted-foreground">
        {t("organizations.no_plan")}
      </span>
    );
  }
  const timing = timingOf(org);
  return (
    <span className="flex min-w-0 flex-col gap-1 leading-tight">
      <span className="flex min-w-0 flex-wrap items-center gap-1.5">
        <span className="truncate font-medium">{plan}</span>
        {org.subscription_status ? (
          <BillingStatus kind="subscription" status={org.subscription_status} />
        ) : null}
      </span>
      {timing ? (
        <span
          className={
            timing.urgent
              ? "text-xs text-amber-700 dark:text-amber-300"
              : "text-muted-foreground text-xs"
          }
        >
          {timing.text}
        </span>
      ) : null}
    </span>
  );
}
