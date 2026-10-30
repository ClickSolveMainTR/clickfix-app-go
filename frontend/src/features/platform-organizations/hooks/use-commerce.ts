"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { platformBillingKeys } from "@/features/billing/hooks/use-platform-billing";
import { platformBillingService } from "@/features/billing/services/platform-billing.service";
import type { BillingWalletEntryInput } from "@/features/billing/types";
import { organizationKeys } from "@/features/platform-organizations/hooks/use-organizations";
import {
  organizationCommerceService,
  type AdminPlanChange,
} from "@/features/platform-organizations/services/commerce.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Refreshes billing data of one organization and the organization rows. */
function useInvalidateOrganizationCommerce(organization: string) {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({
      queryKey: platformBillingKeys.org(organization),
    });
    void qc.invalidateQueries({ queryKey: platformBillingKeys.stats() });
    void qc.invalidateQueries({
      queryKey: [...platformBillingKeys.all, "subscriptions"],
    });
    void qc.invalidateQueries({ queryKey: organizationKeys.all });
  };
}

const PLAN_PARAMS = { limit: 100, offset: 0 };

/** Plan catalog for pickers; off when the operator cannot read plans. */
export function usePlanCatalog(enabled = true) {
  return useQuery({
    queryKey: platformBillingKeys.plans(PLAN_PARAMS),
    queryFn: () => platformBillingService.plans(PLAN_PARAMS),
    enabled,
    staleTime: 60_000,
  });
}

export function useChangeOrganizationPlan(organization: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizationCommerce(organization);
  return useMutation({
    mutationFn: (body: AdminPlanChange) =>
      organizationCommerceService.changePlan(organization, body),
    onSuccess: (_sub, body) => {
      appToast.success(
        body.timing === "now"
          ? t("commerce.org.change_plan.toast_now")
          : t("commerce.org.change_plan.toast_scheduled"),
      );
      invalidate();
    },
  });
}

/**
 * Manual credit / debit with a category (CF-143). The caller owns the
 * idempotency key so a retry after the step-up prompt does not double-post.
 */
export function useAdjustOrganizationWallet(organization: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizationCommerce(organization);
  return useMutation({
    mutationFn: (body: BillingWalletEntryInput) =>
      organizationCommerceService.walletEntry(organization, body, {
        silent: true,
      }),
    onSuccess: (_entry, body) => {
      appToast.success(
        body.delta > 0
          ? t("commerce.org.wallet.toast_credited")
          : t("commerce.org.wallet.toast_debited"),
      );
      invalidate();
    },
  });
}
