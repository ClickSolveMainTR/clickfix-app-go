"use client";

import { CalendarClock, Info, ShoppingCart } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { PayDialog } from "@/features/billing/components/portal/pay-dialog";
import {
  formatUnitPrice,
  withVat,
  type CreditUnit,
} from "@/features/billing/components/portal/wallet/wallet-format";
import {
  useCreditPackages,
  usePaymentAvailability,
  usePurchaseCredits,
} from "@/features/billing/hooks/use-tenant-billing";
import {
  billingErrorText,
  billingReasonText,
} from "@/features/billing/lib/errors";
import { formatCount, formatMinor } from "@/features/billing/lib/money";
import type { CreditPackage } from "@/features/billing/services/tenant-billing.service";
import type { BillingInvoiceSummary } from "@/features/billing/types";
import { useLocale } from "@/providers/locale-provider";

const UNITS: CreditUnit[] = ["ai_credits", "video_minutes"];

function usePackText(vatBasisPoints: number | null) {
  const { t, locale } = useLocale();
  return (p: CreditPackage) => ({
    amount: t(`wallet.buy.amount.${p.unit}`, {
      count: formatCount(p.credits, locale),
    }),
    price: t("wallet.buy.price_excl", {
      amount: formatMinor(p.amount_minor, "TRY", locale),
    }),
    priceIncl:
      vatBasisPoints !== null
        ? t("wallet.buy.price_incl", {
            amount: formatMinor(
              withVat(p.amount_minor, vatBasisPoints),
              "TRY",
              locale,
            ),
          })
        : null,
    unitPrice: formatUnitPrice(p.amount_minor, p.credits, locale),
    validity: p.expires_days
      ? t("wallet.buy.validity_days", { days: p.expires_days })
      : t("wallet.buy.validity_none"),
  });
}

function PackTile({
  pack,
  canWrite,
  disabled,
  vatBasisPoints,
  onBuy,
}: {
  pack: CreditPackage;
  canWrite: boolean;
  /** Purchases cannot complete right now (see CreditPacksCard). */
  disabled: boolean;
  vatBasisPoints: number | null;
  onBuy: () => void;
}) {
  const { t } = useLocale();
  const text = usePackText(vatBasisPoints)(pack);
  return (
    <li className="flex flex-col rounded-xl border p-4">
      <p className="font-medium">{pack.name}</p>
      {pack.description ? (
        <p className="text-muted-foreground mt-0.5 text-xs">
          {pack.description}
        </p>
      ) : null}
      <p className="mt-3 text-2xl font-semibold tracking-tight tabular-nums">
        {text.amount}
      </p>
      <div className="mt-1 space-y-0.5 text-sm">
        <p className="tabular-nums">{text.price}</p>
        {text.priceIncl ? (
          <p className="text-muted-foreground text-xs tabular-nums">
            {text.priceIncl}
          </p>
        ) : null}
        {text.unitPrice ? (
          <p className="text-muted-foreground text-xs tabular-nums">
            {t(`wallet.buy.unit_price.${pack.unit}`, {
              amount: text.unitPrice,
            })}
          </p>
        ) : null}
      </div>
      <p className="text-muted-foreground mt-3 flex items-center gap-1.5 text-xs">
        <CalendarClock className="size-3.5" />
        {text.validity}
      </p>
      {canWrite ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="mt-4 self-start"
          disabled={disabled}
          onClick={onBuy}
        >
          <ShoppingCart className="size-4" />
          {t("wallet.buy.cta")}
        </Button>
      ) : null}
    </li>
  );
}

function ConfirmPurchase({
  pack,
  vatBasisPoints,
  onClose,
  onInvoice,
}: {
  pack: CreditPackage;
  vatBasisPoints: number | null;
  onClose: () => void;
  onInvoice: (invoice: BillingInvoiceSummary) => void;
}) {
  const { t } = useLocale();
  const purchase = usePurchaseCredits();
  // One key per dialog: a double click or retry reuses the same invoice.
  const [key] = useState(() => crypto.randomUUID());
  const text = usePackText(vatBasisPoints)(pack);
  return (
    <Dialog open onOpenChange={(v) => !v && !purchase.isPending && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t("wallet.buy.confirm_title", { name: pack.name })}
          </DialogTitle>
          <DialogDescription>
            {t("wallet.buy.confirm_description")}
          </DialogDescription>
        </DialogHeader>
        <dl className="divide-y rounded-lg border text-sm">
          <div className="flex justify-between gap-3 px-3 py-2">
            <dt className="text-muted-foreground">
              {t("wallet.buy.confirm_amount")}
            </dt>
            <dd className="font-medium tabular-nums">{text.amount}</dd>
          </div>
          <div className="flex justify-between gap-3 px-3 py-2">
            <dt className="text-muted-foreground">
              {t("wallet.buy.confirm_price")}
            </dt>
            <dd className="text-end tabular-nums">
              <span className="font-medium">{text.price}</span>
              {text.priceIncl ? (
                <span className="text-muted-foreground block text-xs">
                  {text.priceIncl}
                </span>
              ) : null}
            </dd>
          </div>
          <div className="flex justify-between gap-3 px-3 py-2">
            <dt className="text-muted-foreground">
              {t("wallet.buy.confirm_validity")}
            </dt>
            <dd>{text.validity}</dd>
          </div>
        </dl>
        {purchase.isError ? (
          <p role="alert" className="text-destructive text-sm">
            {billingErrorText(t, purchase.error, "wallet.buy.failed")}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={purchase.isPending}
            onClick={onClose}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={purchase.isPending || purchase.isSuccess}
            onClick={() =>
              purchase.mutate(
                { packageUuid: pack.uuid, idempotencyKey: key },
                { onSuccess: onInvoice },
              )
            }
          >
            {t("wallet.buy.confirm_cta")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Packs the organization's plan may buy, grouped by unit. Buying opens an
 * invoice and the regular pay dialog; credits are never shown as added
 * here: the backend grants them once the payment is confirmed.
 */
export function CreditPacksCard({
  canWrite,
  vatBasisPoints,
}: {
  canWrite: boolean;
  /** VAT rate of the current plan price (pack prices exclude VAT). */
  vatBasisPoints: number | null;
}) {
  const { t } = useLocale();
  const packages = useCreditPackages();
  const availability = usePaymentAvailability();
  // Unknown (loading / failed) stays enabled: the API still refuses with a
  // specific, localized error.
  const purchase = availability.data?.credit_purchase;
  const blocked = purchase?.ready === false;
  const [buying, setBuying] = useState<CreditPackage | null>(null);
  const [paying, setPaying] = useState<BillingInvoiceSummary | null>(null);
  const [pending, setPending] = useState(false);
  const items = packages.data?.items ?? [];
  const groups = UNITS.map((unit) => ({
    unit,
    packs: items.filter((p) => p.unit === unit && p.active !== false),
  })).filter((g) => g.packs.length > 0);

  return (
    <EntitySectionCard
      title={t("wallet.buy.title")}
      description={t("wallet.buy.description")}
    >
      <div className="space-y-5">
        {blocked && groups.length > 0 ? (
          <p
            role="status"
            className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm"
          >
            <Info className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
            {billingReasonText(t, purchase?.reason, "wallet.pay.not_ready")}
          </p>
        ) : null}
        {pending ? (
          <p className="flex items-start gap-2 rounded-md border border-sky-500/30 bg-sky-500/10 px-3 py-2 text-sm">
            <Info className="mt-0.5 size-4 shrink-0 text-sky-600 dark:text-sky-400" />
            {t("wallet.buy.pending")}
          </p>
        ) : null}
        {packages.isLoading ? <Skeleton className="h-40 w-full" /> : null}
        {packages.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void packages.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {packages.isSuccess && groups.length === 0 ? (
          <EmptyState
            className="py-6"
            title={t("wallet.buy.empty")}
            description={t("wallet.buy.empty_hint")}
          />
        ) : null}
        {groups.map((g) => (
          <section key={g.unit} className="space-y-2">
            <h3 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              {t(`wallet.buy.group.${g.unit}`)}
            </h3>
            <ul className="grid gap-3 sm:grid-cols-2 2xl:grid-cols-3">
              {g.packs.map((p) => (
                <PackTile
                  key={p.uuid}
                  pack={p}
                  canWrite={canWrite}
                  disabled={blocked}
                  vatBasisPoints={vatBasisPoints}
                  onBuy={() => setBuying(p)}
                />
              ))}
            </ul>
          </section>
        ))}
        {!canWrite && groups.length > 0 ? (
          <p className="text-muted-foreground text-xs">
            {t("wallet.buy.read_only")}
          </p>
        ) : null}
      </div>
      {buying ? (
        <ConfirmPurchase
          pack={buying}
          vatBasisPoints={vatBasisPoints}
          onClose={() => setBuying(null)}
          onInvoice={(invoice) => {
            setBuying(null);
            setPending(true);
            setPaying(invoice);
          }}
        />
      ) : null}
      {paying ? (
        <PayDialog
          invoice={paying}
          note={t("wallet.buy.pay_note")}
          onClose={() => setPaying(null)}
        />
      ) : null}
    </EntitySectionCard>
  );
}
