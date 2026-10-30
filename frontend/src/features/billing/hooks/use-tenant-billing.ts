"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

import {
  tenantBillingService as api,
  type BillingProfile,
  type CreditPurchaseInvoice,
  type TenantWalletSettingsInput,
} from "@/features/billing/services/tenant-billing.service";
import type {
  BillingInvoiceSummary,
  BillingPaymentCard,
  Page,
} from "@/features/billing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const tenantBillingKeys = {
  all: (slug: string) => ["tenant", slug, "billing"] as const,
  subscription: (slug: string) =>
    [...tenantBillingKeys.all(slug), "subscription"] as const,
  entitlements: (slug: string) =>
    [...tenantBillingKeys.all(slug), "entitlements"] as const,
  usage: (slug: string) => [...tenantBillingKeys.all(slug), "usage"] as const,
  invoices: (slug: string, offset: number) =>
    [...tenantBillingKeys.all(slug), "invoices", offset] as const,
  cards: (slug: string) => [...tenantBillingKeys.all(slug), "cards"] as const,
  bankAccounts: (slug: string) =>
    [...tenantBillingKeys.all(slug), "bank-accounts"] as const,
  transfers: (slug: string) =>
    [...tenantBillingKeys.all(slug), "transfers"] as const,
  profile: (slug: string) =>
    [...tenantBillingKeys.all(slug), "profile"] as const,
  catalog: () => ["public", "pricing"] as const,
  wallet: (slug: string, offset: number) =>
    [...tenantBillingKeys.all(slug), "wallet", offset] as const,
  creditPackages: (slug: string) =>
    [...tenantBillingKeys.all(slug), "credit-packages"] as const,
  availability: (slug: string) =>
    [...tenantBillingKeys.all(slug), "payment-availability"] as const,
  checkoutStatus: (slug: string, reference: string) =>
    [...tenantBillingKeys.all(slug), "checkout", reference] as const,
};

export const WALLET_PAGE_SIZE = 10;

function useInvalidate() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return () =>
    void qc.invalidateQueries({ queryKey: tenantBillingKeys.all(slug) });
}

export function useTenantSubscription() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.subscription(slug),
    queryFn: () => api.subscription(slug),
  });
}

export function useTenantEntitlements() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.entitlements(slug),
    queryFn: () => api.entitlements(slug),
  });
}

export function useTenantUsage() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.usage(slug),
    queryFn: () => api.usage(slug),
  });
}

export function useTenantInvoices(offset: number) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.invoices(slug, offset),
    queryFn: () => api.invoices(slug, { limit: 10, offset }),
    placeholderData: (prev) => prev,
  });
}

export function useTenantCards() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.cards(slug),
    queryFn: () => api.cards(slug),
  });
}

/** Removes a saved card; the row disappears immediately (rolled back on error). */
export function useRemoveCard() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const qc = useQueryClient();
  const key = tenantBillingKeys.cards(slug);
  return useMutation({
    mutationFn: (uuid: string) => api.removeCard(slug, uuid),
    onMutate: async (uuid) => {
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<Page<BillingPaymentCard>>(key);
      if (prev)
        qc.setQueryData<Page<BillingPaymentCard>>(key, {
          ...prev,
          items: prev.items.filter((c) => c.uuid !== uuid),
          total: Math.max(0, prev.total - 1),
        });
      return { prev };
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev);
    },
    onSuccess: () => appToast.success(t("billing.portal.cards.removed_toast")),
    onSettled: () => void qc.invalidateQueries({ queryKey: key }),
  });
}

export function useTenantBankAccounts(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.bankAccounts(slug),
    queryFn: () => api.bankAccounts(slug),
    enabled,
    staleTime: 5 * 60_000,
  });
}

export function useTenantTransfers() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.transfers(slug),
    queryFn: () => api.transfers(slug),
  });
}

export function useReportTransfer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: async (v: {
      invoice: string;
      bankAccount: string;
      amountMinor: number;
      file: File;
      note: string;
    }) => {
      const attachment = await api.uploadReceipt(slug, v.file);
      return api.reportTransfer(slug, {
        invoice_uuid: v.invoice,
        bank_account_uuid: v.bankAccount,
        attachment_uuid: attachment.uuid,
        amount_minor: v.amountMinor,
        note: v.note || undefined,
      });
    },
    onSuccess: () => {
      appToast.success(t("billing.portal.pay.transfer_reported_toast"));
      invalidate();
    },
  });
}

/**
 * Starts a hosted card checkout and sends the browser to the gateway page.
 * `idempotencyKey` is created once per pay dialog: a double click or a retry
 * reuses the same payment and page instead of opening a second one.
 */
export function useCheckout() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (v: {
      invoice: string;
      storeCard: boolean;
      autoPay: boolean;
      idempotencyKey: string;
    }) =>
      api.checkout(slug, {
        invoice_uuid: v.invoice,
        idempotency_key: v.idempotencyKey,
        store_card: v.storeCard,
        auto_pay_consent: v.storeCard && v.autoPay,
      }),
    onSuccess: (out) => {
      if (out.page_url) window.location.assign(out.page_url);
    },
  });
}

/** Payment readiness: purchases are hidden or disabled when not ready. */
export function usePaymentAvailability() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.availability(slug),
    queryFn: () => api.paymentAvailability(slug),
    staleTime: 60_000,
  });
}

const FINAL_PAYMENT = new Set(["succeeded", "failed", "expired"]);

/**
 * Polls a checkout after the provider return until the backend has a final,
 * provider-verified result (webhook, return confirmation or reconciliation).
 */
export function useCheckoutStatus(reference: string | null) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: tenantBillingKeys.checkoutStatus(slug, reference ?? ""),
    enabled: Boolean(reference),
    queryFn: () => api.checkoutStatus(slug, reference ?? ""),
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data && FINAL_PAYMENT.has(data.status)) return false;
      // Stop after ~5 minutes; reconciliation settles slower cases.
      return query.state.dataUpdateCount > 100 ? false : 3000;
    },
    retry: (count, error) =>
      !(error instanceof ApiError && error.status === 404) && count < 3,
  });
  const final = query.data && FINAL_PAYMENT.has(query.data.status);
  useEffect(() => {
    // Invoices, wallet and subscription change with the result; refresh
    // them once (not this status query, which would poll again).
    if (!final) return;
    void qc.invalidateQueries({
      queryKey: tenantBillingKeys.all(slug),
      predicate: (q) => q.queryKey[3] !== "checkout",
    });
  }, [final, qc, slug]);
  return query;
}

/** Billing profile; `null` while none is saved yet (API answers 404). */
export function useBillingProfile() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.profile(slug),
    queryFn: async () => {
      try {
        return await api.profile(slug);
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return null;
        throw error;
      }
    },
  });
}

export function useSaveBillingProfile() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BillingProfile) => api.saveProfile(slug, body),
    onSuccess: (profile) => {
      appToast.success(t("billing.portal.profile.saved_toast"));
      qc.setQueryData(tenantBillingKeys.profile(slug), profile);
    },
  });
}

export function useCatalog() {
  return useQuery({
    queryKey: tenantBillingKeys.catalog(),
    queryFn: () => api.catalog(),
    staleTime: 5 * 60_000,
  });
}

export function usePlanChangePreview() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (body: { price_uuid: string; seats: number }) =>
      api.previewPlanChange(slug, body),
  });
}

export function useApplyPlanChange() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: { price_uuid: string; seats: number }) =>
      api.applyPlanChange(slug, {
        ...body,
        idempotency_key: crypto.randomUUID(),
      }),
    onSuccess: () => {
      appToast.success(t("billing.portal.change.applied_toast"));
      invalidate();
    },
  });
}

export function useAttachCoupon() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (code: string) => api.attachCoupon(slug, code),
    onSuccess: () => {
      appToast.success(t("billing.portal.coupon.applied_toast"));
      invalidate();
    },
  });
}

/** Fetches (or creates) the post-trial invoice so it can be paid now. */
export function useTrialInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (subscription: string) => api.trialInvoice(slug, subscription),
    onSuccess: () => invalidate(),
  });
}

/** Wallet balances, settings and one ledger page (offset 0 for the cards). */
export function useTenantWallet(offset = 0) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.wallet(slug, offset),
    queryFn: () => api.wallet(slug, { limit: WALLET_PAGE_SIZE, offset }),
    placeholderData: (prev) => prev,
  });
}

export function useCreditPackages() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantBillingKeys.creditPackages(slug),
    queryFn: () => api.creditPackages(slug),
    staleTime: 5 * 60_000,
  });
}

export function useSaveWalletSettings() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: TenantWalletSettingsInput) =>
      api.saveWalletSettings(slug, body),
    onSuccess: () => {
      appToast.success(t("wallet.settings.saved_toast"));
      void qc.invalidateQueries({
        queryKey: [...tenantBillingKeys.all(slug), "wallet"],
      });
    },
    // Silent request: explain a 409 (no auto-pay card / pack gone) in plain words.
    onError: (error) =>
      appToast.error(
        t(
          error instanceof ApiError && error.status === 409
            ? "wallet.settings.conflict"
            : "wallet.settings.failed",
        ),
      ),
  });
}

/**
 * Opens the invoice of a credit pack and resolves it as an invoice summary
 * (with the payment reference the transfer flow needs). Credits are added by
 * the backend only after the invoice is paid; nothing is granted here.
 */
export function usePurchaseCredits() {
  const { slug } = useTenant();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: async (v: {
      packageUuid: string;
      idempotencyKey: string;
    }): Promise<BillingInvoiceSummary> => {
      const record = await api.purchaseCredits(
        slug,
        v.packageUuid,
        v.idempotencyKey,
      );
      const recent = await api.invoices(slug, { limit: 10, offset: 0 });
      const summary = recent.items.find((i) => i.uuid === record.uuid);
      if (summary) return summary;
      const loose = record as CreditPurchaseInvoice & {
        payment_reference?: string;
      };
      const now = new Date().toISOString();
      return {
        uuid: record.uuid,
        number: record.number ?? null,
        payment_reference: loose.payment_reference ?? "",
        status: record.status,
        currency: record.currency,
        total_minor: record.total_minor,
        due_at: record.due_at ?? now,
        period_start: record.period_start ?? now,
        period_end: record.period_end ?? now,
      };
    },
    onSuccess: () => invalidate(),
  });
}

export function useCancelSubscription() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (subscription: string) => api.cancel(slug, subscription),
    onSuccess: () => {
      appToast.success(t("billing.portal.cancel.scheduled_toast"));
      invalidate();
    },
  });
}
