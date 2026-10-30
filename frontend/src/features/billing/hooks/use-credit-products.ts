"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { platformBillingKeys } from "@/features/billing/hooks/use-platform-billing";
import {
  creditProductsService as api,
  type CreditProductInput,
  type PlanMigrationInput,
} from "@/features/billing/services/credit-products.service";
import type { BillingAdminSubscription } from "@/features/billing/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const creditProductKeys = {
  all: [...platformBillingKeys.all, "credit-products"] as const,
  planSubscribers: (plan: string) =>
    [...platformBillingKeys.all, "subscriptions", "plan", plan] as const,
};

/** Every credit product (the catalog is small; the API does not page it). */
export function useCreditProducts() {
  return useQuery({
    queryKey: creditProductKeys.all,
    queryFn: () => api.list(),
    select: (data) => data.items,
  });
}

export function useSaveCreditProduct() {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: {
      uuid?: string;
      body: CreditProductInput;
      toast?: "created" | "saved" | "archived" | "restored";
    }) => (v.uuid ? api.update(v.uuid, v.body) : api.create(v.body)),
    onSuccess: (_d, v) => {
      appToast.success(
        t(`credit_products.toast.${v.toast ?? (v.uuid ? "saved" : "created")}`),
      );
      void qc.invalidateQueries({ queryKey: creditProductKeys.all });
    },
  });
}

/** Live and cancelable subscriptions read in pages of 100, at most 1000. */
const PAGE = 100;
const MAX_ROWS = 1000;

/**
 * Subscriptions of one plan across organizations. The admin list has no plan
 * filter, so live rows are paged in and filtered here (bounded by MAX_ROWS).
 */
export function usePlanSubscribers(plan: string, enabled: boolean) {
  return useQuery({
    queryKey: creditProductKeys.planSubscribers(plan),
    enabled,
    queryFn: async () => {
      const rows: BillingAdminSubscription[] = [];
      let offset = 0;
      let total = 0;
      do {
        const page = await api.subscriptions({
          limit: PAGE,
          offset,
          live: "true",
        });
        rows.push(...page.items);
        total = page.total;
        offset += PAGE;
      } while (offset < total && offset < MAX_ROWS);
      return {
        items: rows.filter((row) => row.plan_uuid === plan),
        truncated: total > MAX_ROWS,
      };
    },
  });
}

export function useSchedulePlanMigration(plan: string) {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: PlanMigrationInput) => api.scheduleMigration(body),
    onSuccess: (data, body) => {
      appToast.success(
        t("credit_products.migrate.toast", {
          count: data?.scheduled ?? body.subscription_uuids.length,
        }),
      );
      void qc.invalidateQueries({
        queryKey: creditProductKeys.planSubscribers(plan),
      });
      void qc.invalidateQueries({ queryKey: platformBillingKeys.plan(plan) });
    },
  });
}
