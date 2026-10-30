"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { platformBillingService as api } from "@/features/billing/services/platform-billing.service";
import type {
  AdminListParams,
  BillingAdminTransfer,
  BillingBankAccountInput,
  BillingOverride,
  BillingPlanInput,
  BillingTransition,
  BillingVersionInput,
  BillingWalletEntryInput,
  Page,
} from "@/features/billing/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const platformBillingKeys = {
  all: ["platform", "billing"] as const,
  stats: () => [...platformBillingKeys.all, "stats"] as const,
  subscriptions: (p: AdminListParams) =>
    [...platformBillingKeys.all, "subscriptions", p] as const,
  payments: (p: AdminListParams) =>
    [...platformBillingKeys.all, "payments", p] as const,
  transfers: (p?: AdminListParams) =>
    p
      ? ([...platformBillingKeys.all, "transfers", p] as const)
      : ([...platformBillingKeys.all, "transfers"] as const),
  bankAccounts: () => [...platformBillingKeys.all, "bank-accounts"] as const,
  plans: (p?: { limit: number; offset: number; q?: string }) =>
    p
      ? ([...platformBillingKeys.all, "plans", p] as const)
      : ([...platformBillingKeys.all, "plans"] as const),
  plan: (uuid: string) => [...platformBillingKeys.all, "plan", uuid] as const,
  features: () => [...platformBillingKeys.all, "features"] as const,
  org: (uuid: string) => [...platformBillingKeys.all, "org", uuid] as const,
  subscription: (uuid: string) =>
    [...platformBillingKeys.org(uuid), "subscription"] as const,
  history: (uuid: string, sub: string) =>
    [...platformBillingKeys.org(uuid), "history", sub] as const,
  entitlements: (uuid: string) =>
    [...platformBillingKeys.org(uuid), "entitlements"] as const,
  usage: (uuid: string) => [...platformBillingKeys.org(uuid), "usage"] as const,
  wallet: (uuid: string, p?: { limit: number; offset: number }) =>
    p
      ? ([...platformBillingKeys.org(uuid), "wallet", p] as const)
      : ([...platformBillingKeys.org(uuid), "wallet"] as const),
};

export function useBillingStats() {
  return useQuery({
    queryKey: platformBillingKeys.stats(),
    queryFn: () => api.stats(),
    staleTime: 30_000,
  });
}

export function useAdminSubscriptions(params: AdminListParams) {
  return useQuery({
    queryKey: platformBillingKeys.subscriptions(params),
    queryFn: () => api.subscriptions(params),
    placeholderData: keepPreviousData,
  });
}

export function useAdminPayments(params: AdminListParams) {
  return useQuery({
    queryKey: platformBillingKeys.payments(params),
    queryFn: () => api.payments(params),
    placeholderData: keepPreviousData,
  });
}

export function useTransferQueue(params: AdminListParams) {
  return useQuery({
    queryKey: platformBillingKeys.transfers(params),
    queryFn: () => api.transferQueue(params),
    placeholderData: keepPreviousData,
  });
}

/**
 * Approve / reject a bank transfer. The row flips optimistically in every
 * cached queue page; on error the snapshot comes back.
 */
export function useReviewTransfer() {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: {
      transfer: BillingAdminTransfer;
      approve: boolean;
      reason: string;
    }) =>
      api.reviewTransfer(v.transfer.organization_uuid, v.transfer.uuid, {
        approve: v.approve,
        reason: v.reason,
      }),
    onMutate: async (v) => {
      const key = platformBillingKeys.transfers();
      await qc.cancelQueries({ queryKey: key });
      const snapshot = qc.getQueriesData<Page<BillingAdminTransfer>>({
        queryKey: key,
      });
      qc.setQueriesData<Page<BillingAdminTransfer>>({ queryKey: key }, (old) =>
        old
          ? {
              ...old,
              items: old.items.map((row) =>
                row.uuid === v.transfer.uuid
                  ? {
                      ...row,
                      status: v.approve ? "approved" : "rejected",
                      rejection_reason: v.approve ? null : v.reason,
                      reviewed_at: new Date().toISOString(),
                    }
                  : row,
              ),
            }
          : old,
      );
      return { snapshot };
    },
    onError: (_e, _v, ctx) => {
      ctx?.snapshot.forEach(([key, data]) => qc.setQueryData(key, data));
    },
    onSuccess: (_d, v) =>
      appToast.success(
        v.approve
          ? t("billing.transfers.approved_toast")
          : t("billing.transfers.rejected_toast"),
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: platformBillingKeys.all });
    },
  });
}

export function useRefundPayment() {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: {
      organization: string;
      payment: string;
      amountMinor: number;
      cancel: boolean;
    }) =>
      api.refund(v.organization, v.payment, {
        cancel: v.cancel,
        amount_minor: v.amountMinor,
        idempotency_key: crypto.randomUUID(),
      }),
    onSuccess: () => {
      appToast.success(t("billing.payments.refund_toast"));
      void qc.invalidateQueries({ queryKey: platformBillingKeys.all });
    },
  });
}

export function useBankAccounts() {
  return useQuery({
    queryKey: platformBillingKeys.bankAccounts(),
    queryFn: () => api.bankAccounts(),
  });
}

export function useSaveBankAccount() {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BillingBankAccountInput) => api.saveBankAccount(body),
    onSuccess: () => {
      appToast.success(t("billing.bank.saved_toast"));
      void qc.invalidateQueries({
        queryKey: platformBillingKeys.bankAccounts(),
      });
    },
  });
}

export function usePlans(params: {
  limit: number;
  offset: number;
  q?: string;
}) {
  return useQuery({
    queryKey: platformBillingKeys.plans(params),
    queryFn: () => api.plans(params),
    placeholderData: keepPreviousData,
  });
}

export function usePlan(uuid: string) {
  return useQuery({
    queryKey: platformBillingKeys.plan(uuid),
    queryFn: () => api.plan(uuid),
  });
}

export function useBillingFeatures() {
  return useQuery({
    queryKey: platformBillingKeys.features(),
    queryFn: () => api.features(),
    staleTime: 10 * 60_000,
  });
}

export function useCreatePlan() {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BillingPlanInput) => api.createPlan(body),
    onSuccess: () => {
      appToast.success(t("billing.plans.created_toast"));
      void qc.invalidateQueries({ queryKey: platformBillingKeys.plans() });
    },
  });
}

export function useUpdatePlan(uuid: string) {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BillingPlanInput) => api.updatePlan(uuid, body),
    onSuccess: (plan) => {
      appToast.success(t("billing.plans.saved_toast"));
      qc.setQueryData(platformBillingKeys.plan(uuid), plan);
      void qc.invalidateQueries({ queryKey: platformBillingKeys.plans() });
    },
  });
}

export function useArchivePlan(uuid: string) {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.archivePlan(uuid),
    onSuccess: () => {
      appToast.success(t("billing.plans.archived_toast"));
      void qc.invalidateQueries({ queryKey: platformBillingKeys.all });
    },
  });
}

export function useSaveVersion(plan: string) {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { version: string | null; body: BillingVersionInput }) =>
      api.saveVersion(plan, v.version, v.body),
    onSuccess: () => {
      appToast.success(t("billing.versions.saved_toast"));
      void qc.invalidateQueries({ queryKey: platformBillingKeys.plan(plan) });
    },
  });
}

export function usePublishVersion(plan: string) {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (version: string) => api.publishVersion(plan, version),
    onSuccess: () => {
      appToast.success(t("billing.versions.published_toast"));
      void qc.invalidateQueries({ queryKey: platformBillingKeys.plan(plan) });
      void qc.invalidateQueries({ queryKey: platformBillingKeys.plans() });
    },
  });
}

export function useOrgSubscription(organization: string) {
  return useQuery({
    queryKey: platformBillingKeys.subscription(organization),
    queryFn: () => api.subscription(organization),
  });
}

export function useSubscriptionHistory(
  organization: string,
  subscription: string | undefined,
) {
  return useQuery({
    queryKey: platformBillingKeys.history(organization, subscription ?? ""),
    queryFn: () => api.history(organization, subscription ?? ""),
    enabled: Boolean(subscription),
  });
}

export function useOrgEntitlements(organization: string) {
  return useQuery({
    queryKey: platformBillingKeys.entitlements(organization),
    queryFn: () => api.entitlements(organization),
  });
}

export function useOrgUsage(organization: string) {
  return useQuery({
    queryKey: platformBillingKeys.usage(organization),
    queryFn: () => api.usage(organization),
  });
}

export function useOrgWallet(
  organization: string,
  params: { limit: number; offset: number },
) {
  return useQuery({
    queryKey: platformBillingKeys.wallet(organization, params),
    queryFn: () => api.wallet(organization, params),
    placeholderData: keepPreviousData,
  });
}

/**
 * Manual wallet credit/debit (CF-136). The caller owns the idempotency key so
 * a retry after a step-up prompt or a dropped response does not double-post.
 */
export function useCreateWalletEntry(organization: string) {
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BillingWalletEntryInput) =>
      api.createWalletEntry(organization, body),
    onSuccess: () => {
      appToast.success(t("billing.wallet.created_toast"));
      void qc.invalidateQueries({
        queryKey: platformBillingKeys.wallet(organization),
      });
    },
  });
}

function useOrgInvalidate(organization: string) {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({
      queryKey: platformBillingKeys.org(organization),
    });
    void qc.invalidateQueries({ queryKey: platformBillingKeys.stats() });
    void qc.invalidateQueries({
      queryKey: [...platformBillingKeys.all, "subscriptions"],
    });
  };
}

export function useTransitionSubscription(organization: string) {
  const { t } = useLocale();
  const invalidate = useOrgInvalidate(organization);
  return useMutation({
    mutationFn: (v: { subscription: string; body: BillingTransition }) =>
      api.transition(organization, v.subscription, v.body),
    onSuccess: () => {
      appToast.success(t("billing.subscription.updated_toast"));
      invalidate();
    },
  });
}

export function useExtendTrial(organization: string) {
  const { t } = useLocale();
  const invalidate = useOrgInvalidate(organization);
  return useMutation({
    mutationFn: (v: { subscription: string; endsAt: string; reason: string }) =>
      api.extendTrial(organization, v.subscription, {
        ends_at: v.endsAt,
        reason: v.reason,
      }),
    onSuccess: () => {
      appToast.success(t("billing.subscription.trial_extended_toast"));
      invalidate();
    },
  });
}

export function useAssignPlan(organization: string) {
  const { t } = useLocale();
  const invalidate = useOrgInvalidate(organization);
  return useMutation({
    mutationFn: (body: Parameters<typeof api.createSubscription>[0]) =>
      api.createSubscription(body),
    onSuccess: () => {
      appToast.success(t("billing.subscription.assigned_toast"));
      invalidate();
    },
  });
}

export function useSetOverride(organization: string) {
  const { t } = useLocale();
  const invalidate = useOrgInvalidate(organization);
  return useMutation({
    mutationFn: (v: { feature: string; body: BillingOverride | null }) =>
      v.body
        ? api.setOverride(organization, v.feature, v.body)
        : api.clearOverride(organization, v.feature),
    onSuccess: (_d, v) => {
      appToast.success(
        v.body
          ? t("billing.entitlements.override_saved_toast")
          : t("billing.entitlements.override_cleared_toast"),
      );
      invalidate();
    },
  });
}
