"use client";

import {
  ArrowUpDown,
  CreditCard,
  FileDown,
  Landmark,
  Ticket,
  Trash2,
  XCircle,
} from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BillingTable,
  type BillingColumn,
} from "@/features/billing/components/billing-table";
import { BillingStatus } from "@/features/billing/components/billing-status";
import { PaymentProviderLabel } from "@/features/billing/components/payment-provider-label";
import { BillingProfileForm } from "@/features/billing/components/portal/billing-profile-form";
import { PayDialog } from "@/features/billing/components/portal/pay-dialog";
import { PaymentReturnBanner } from "@/features/billing/components/portal/payment-return-banner";
import { PlanChangeDialog } from "@/features/billing/components/portal/plan-change-dialog";
import { CreditsSection } from "@/features/billing/components/portal/wallet/credits-section";
import { UsageMeters } from "@/features/billing/components/usage-meters";
import {
  useAttachCoupon,
  useBillingProfile,
  useCancelSubscription,
  useCatalog,
  useRemoveCard,
  useTenantCards,
  useTenantEntitlements,
  useTenantInvoices,
  useTenantSubscription,
  useTenantTransfers,
  useTenantUsage,
  useTrialInvoice,
} from "@/features/billing/hooks/use-tenant-billing";
import { formatMinor } from "@/features/billing/lib/money";
import { tenantBillingService } from "@/features/billing/services/tenant-billing.service";
import type {
  BillingInvoiceSummary,
  BillingPlan,
} from "@/features/billing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const CANCEL_REASONS = [
  "price",
  "missing_feature",
  "not_using",
  "switching",
  "other",
] as const;

function resolvePlan(plans: BillingPlan[] | undefined, versionUuid: string) {
  for (const plan of plans ?? []) {
    const version = plan.versions.find((v) => v.uuid === versionUuid);
    if (version) return { plan, version };
  }
  return null;
}

function daysUntil(iso: string) {
  return Math.max(0, Math.ceil((Date.parse(iso) - Date.now()) / 86_400_000));
}

function CancelDialog({
  open,
  periodEnd,
  onClose,
  onConfirm,
  onSwitch,
  pending,
}: {
  open: boolean;
  periodEnd: string;
  onClose: () => void;
  onConfirm: () => void;
  onSwitch: () => void;
  pending: boolean;
}) {
  const { t, locale } = useLocale();
  const [step, setStep] = useState<"reason" | "offer">("reason");
  const [reason, setReason] = useState<string>("");
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("billing.portal.cancel.title")}</DialogTitle>
          <DialogDescription>
            {step === "reason"
              ? t("billing.portal.cancel.reason_prompt")
              : t("billing.portal.cancel.offer_description", {
                  date: date(periodEnd, undefined, locale),
                })}
          </DialogDescription>
        </DialogHeader>
        {step === "reason" ? (
          <RadioGroup
            value={reason}
            onValueChange={setReason}
            className="gap-2"
          >
            {CANCEL_REASONS.map((r) => (
              <Label
                key={r}
                className="flex items-center gap-3 rounded-md border p-2.5 font-normal"
              >
                <RadioGroupItem value={r} />
                {t(`billing.portal.cancel.reasons.${r}`)}
              </Label>
            ))}
          </RadioGroup>
        ) : (
          <div className="space-y-3 text-sm">
            {reason === "price" || reason === "not_using" ? (
              <div className="rounded-lg border p-3">
                <p className="font-medium">
                  {t("billing.portal.cancel.offer_downgrade")}
                </p>
                <p className="text-muted-foreground mt-1 text-xs">
                  {t("billing.portal.cancel.offer_downgrade_hint")}
                </p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="mt-2"
                  onClick={onSwitch}
                >
                  <ArrowUpDown className="size-4" />
                  {t("billing.portal.change.cta")}
                </Button>
              </div>
            ) : (
              <p className="text-muted-foreground">
                {t("billing.portal.cancel.offer_support")}
              </p>
            )}
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("billing.portal.cancel.keep")}
          </Button>
          {step === "reason" ? (
            <Button
              type="button"
              disabled={!reason}
              onClick={() => setStep("offer")}
            >
              {t("billing.portal.cancel.continue")}
            </Button>
          ) : (
            <Button
              type="button"
              variant="destructive"
              disabled={pending}
              onClick={onConfirm}
            >
              {t("billing.portal.cancel.confirm")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CouponForm() {
  const { t } = useLocale();
  const coupon = useAttachCoupon();
  const [code, setCode] = useState("");
  return (
    <form
      className="flex gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (code.trim())
          coupon.mutate(code.trim(), { onSuccess: () => setCode("") });
      }}
    >
      <div className="relative flex-1">
        <Ticket className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={code}
          onChange={(e) => setCode(e.target.value.toUpperCase())}
          placeholder={t("billing.portal.coupon.placeholder")}
          aria-label={t("billing.portal.coupon.label")}
          className="ps-8 font-mono uppercase"
          maxLength={64}
        />
      </div>
      <Button
        type="submit"
        variant="outline"
        disabled={!code.trim() || coupon.isPending}
      >
        {t("billing.portal.coupon.apply")}
      </Button>
    </form>
  );
}

function OverviewCard({
  canWrite,
  onChangePlan,
  onPay,
  onPayTrial,
  trialPending,
  openInvoice,
}: {
  canWrite: boolean;
  onChangePlan: () => void;
  onPayTrial: () => void;
  trialPending: boolean;
  onPay: (invoice: BillingInvoiceSummary) => void;
  openInvoice: BillingInvoiceSummary | undefined;
}) {
  const { t, locale } = useLocale();
  const sub = useTenantSubscription();
  const catalog = useCatalog();
  const cancel = useCancelSubscription();
  const [canceling, setCanceling] = useState(false);
  const s = sub.data;
  const resolved = s ? resolvePlan(catalog.data?.items, s.version_uuid) : null;
  const price = resolved?.version.prices.find((p) => p.uuid === s?.price_uuid);

  if (sub.isLoading) return <Skeleton className="h-48 w-full rounded-xl" />;
  if (sub.isError)
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void sub.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  if (!s)
    return (
      <EntitySectionCard title={t("billing.portal.overview.title")}>
        <EmptyState
          className="py-8"
          title={t("billing.portal.overview.none")}
          description={t("billing.portal.overview.none_hint")}
        />
      </EntitySectionCard>
    );

  const trialDays = s.trial_ends_at ? daysUntil(s.trial_ends_at) : 0;
  return (
    <EntitySectionCard title={t("billing.portal.overview.title")}>
      <div className="space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="space-y-1">
            <p className="text-2xl font-semibold tracking-tight">
              {resolved?.plan.name ?? t("billing.portal.overview.custom_plan")}
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <BillingStatus kind="subscription" status={s.status} />
              {s.cancel_at_period_end ? (
                <Badge variant="outline">
                  {t("billing.portal.overview.cancels_on", {
                    date: date(s.period_end, undefined, locale),
                  })}
                </Badge>
              ) : null}
            </div>
          </div>
          {price ? (
            <p className="text-end">
              <span className="text-xl font-semibold tabular-nums">
                {formatMinor(price.amount_minor, price.currency, locale)}
              </span>
              <span className="text-muted-foreground block text-xs">
                {t(`billing.interval.${price.interval}`)} ·{" "}
                {price.tax_inclusive
                  ? t("billing.portal.overview.vat_included")
                  : t("billing.portal.overview.vat_excluded", {
                      rate: price.vat_basis_points / 100,
                    })}
              </span>
            </p>
          ) : null}
        </div>
        <dl className="grid gap-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("billing.portal.overview.period")}
            </dt>
            <dd className="tabular-nums">
              {date(s.period_start, undefined, locale)} –{" "}
              {date(s.period_end, undefined, locale)}
            </dd>
          </div>
          {s.status === "trialing" && s.trial_ends_at ? (
            <div>
              <dt className="text-muted-foreground text-xs">
                {t("billing.portal.overview.trial")}
              </dt>
              <dd>
                {t("billing.portal.overview.trial_left", { days: trialDays })}
              </dd>
            </div>
          ) : null}
          {s.status === "active" && !s.cancel_at_period_end ? (
            <div>
              <dt className="text-muted-foreground text-xs">
                {t("billing.portal.overview.renews")}
              </dt>
              <dd className="tabular-nums">
                {date(s.period_end, undefined, locale)}
              </dd>
            </div>
          ) : null}
        </dl>
        {s.status === "trialing" && canWrite && !s.cancel_at_period_end ? (
          <p className="text-muted-foreground text-xs">
            {t("billing.portal.overview.pay_trial_hint")}
          </p>
        ) : null}
        {openInvoice ? (
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm">
            <span>
              {t("billing.portal.overview.open_invoice", {
                amount: formatMinor(
                  openInvoice.total_minor,
                  openInvoice.currency,
                  locale,
                ),
                date: date(openInvoice.due_at, undefined, locale),
              })}
            </span>
            {canWrite ? (
              <Button
                type="button"
                size="sm"
                onClick={() => onPay(openInvoice)}
              >
                <CreditCard className="size-4" />
                {t("billing.portal.invoices.pay")}
              </Button>
            ) : null}
          </div>
        ) : null}
        {canWrite ? (
          <div className="flex flex-wrap gap-2 border-t pt-4">
            {(s.status === "trialing" || s.status === "pending_payment") &&
            !s.cancel_at_period_end ? (
              <Button
                type="button"
                size="sm"
                disabled={trialPending}
                onClick={onPayTrial}
              >
                <CreditCard className="size-4" />
                {t("billing.portal.overview.pay_trial")}
              </Button>
            ) : null}
            {s.status === "active" && !s.cancel_at_period_end ? (
              <Button type="button" size="sm" onClick={onChangePlan}>
                <ArrowUpDown className="size-4" />
                {t("billing.portal.change.cta")}
              </Button>
            ) : null}
            {!s.cancel_at_period_end &&
            ["active", "trialing", "past_due", "grace"].includes(s.status) ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="text-destructive"
                onClick={() => setCanceling(true)}
              >
                <XCircle className="size-4" />
                {t("billing.portal.cancel.cta")}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      {canceling ? (
        <CancelDialog
          open
          periodEnd={s.period_end}
          pending={cancel.isPending}
          onClose={() => setCanceling(false)}
          onSwitch={() => {
            setCanceling(false);
            onChangePlan();
          }}
          onConfirm={() =>
            cancel.mutate(s.uuid, { onSettled: () => setCanceling(false) })
          }
        />
      ) : null}
    </EntitySectionCard>
  );
}

function UsageCard() {
  const { t } = useLocale();
  const ents = useTenantEntitlements();
  const usage = useTenantUsage();
  const labels = new Map<string, string>(
    Object.keys(ents.data?.values ?? {}).map((k) => {
      const key = `billing.portal.features.${k}`;
      const label = t(key);
      return [k, label === key ? k : label];
    }),
  );
  return (
    <EntitySectionCard title={t("billing.portal.usage.title")}>
      {ents.isLoading ? <Skeleton className="h-32 w-full" /> : null}
      {ents.data ? (
        <UsageMeters
          values={ents.data.values}
          usage={usage.data ?? []}
          labels={labels}
        />
      ) : null}
    </EntitySectionCard>
  );
}

function InvoicesCard({
  canWrite,
  onPay,
}: {
  canWrite: boolean;
  onPay: (invoice: BillingInvoiceSummary) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [offset, setOffset] = useState(0);
  const invoices = useTenantInvoices(offset);
  const [downloading, setDownloading] = useState<string | null>(null);

  const download = async (inv: BillingInvoiceSummary) => {
    setDownloading(inv.uuid);
    try {
      const { blob, filename } = await tenantBillingService.downloadInvoice(
        slug,
        inv.uuid,
        "pdf",
      );
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename ?? `${inv.number ?? inv.payment_reference}.pdf`;
      a.click();
      URL.revokeObjectURL(url);
    } finally {
      setDownloading(null);
    }
  };

  const columns: BillingColumn<BillingInvoiceSummary>[] = [
    {
      key: "number",
      header: t("billing.portal.invoices.number"),
      cell: (inv) => (
        <span className="flex flex-col">
          <code className="text-xs font-medium">
            {inv.number ?? inv.payment_reference}
          </code>
          <span className="text-muted-foreground text-xs tabular-nums">
            {date(inv.period_start, undefined, locale)} –{" "}
            {date(inv.period_end, undefined, locale)}
          </span>
        </span>
      ),
    },
    {
      key: "status",
      header: t("billing.portal.invoices.status"),
      cell: (inv) => <BillingStatus kind="invoice" status={inv.status} />,
    },
    {
      key: "due",
      header: t("billing.portal.invoices.due"),
      cell: (inv) => (
        <span className="tabular-nums">
          {date(inv.due_at, undefined, locale)}
        </span>
      ),
    },
    {
      key: "total",
      header: t("billing.portal.invoices.total"),
      align: "end",
      cell: (inv) => (
        <span className="font-medium">
          {formatMinor(inv.total_minor, inv.currency, locale)}
        </span>
      ),
    },
    {
      key: "actions",
      header: <span className="sr-only">{t("billing.common.actions")}</span>,
      align: "end",
      cell: (inv) => (
        <span className="inline-flex gap-1">
          {inv.status === "open" && canWrite ? (
            <Button
              type="button"
              size="sm"
              className="h-8"
              onClick={() => onPay(inv)}
            >
              {t("billing.portal.invoices.pay")}
            </Button>
          ) : null}
          {inv.pdf_url ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-8"
              disabled={downloading === inv.uuid}
              onClick={() => void download(inv)}
            >
              <FileDown className="size-3.5" />
              PDF
            </Button>
          ) : null}
        </span>
      ),
    },
  ];

  return (
    <EntitySectionCard title={t("billing.portal.invoices.title")}>
      <BillingTable
        caption={t("billing.portal.invoices.title")}
        columns={columns}
        rows={invoices.data?.items}
        getRowId={(inv) => inv.uuid}
        isLoading={invoices.isLoading}
        isError={invoices.isError}
        onRetry={() => void invoices.refetch()}
        emptyTitle={t("billing.portal.invoices.empty")}
        page={
          invoices.data
            ? {
                total: invoices.data.total,
                limit: 10,
                offset,
                onOffsetChange: setOffset,
              }
            : undefined
        }
      />
    </EntitySectionCard>
  );
}

function PaymentMethodsCard({ canWrite }: { canWrite: boolean }) {
  const { t, locale } = useLocale();
  const cards = useTenantCards();
  const transfers = useTenantTransfers();
  const remove = useRemoveCard();
  const [removing, setRemoving] = useState<string | null>(null);
  const pending = (transfers.data?.items ?? []).filter(
    (tr) => tr.status === "reported",
  );
  return (
    <EntitySectionCard title={t("billing.portal.cards.title")}>
      <div className="space-y-4">
        {cards.isLoading ? <Skeleton className="h-16 w-full" /> : null}
        {cards.data && cards.data.items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("billing.portal.cards.empty")}
          </p>
        ) : null}
        <ul className="space-y-2">
          {(cards.data?.items ?? []).map((card) => (
            <li
              key={card.uuid}
              className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2"
            >
              <span className="flex items-center gap-3">
                <CreditCard className="text-muted-foreground size-4" />
                <span className="tabular-nums">
                  {card.brand ? `${card.brand} ` : ""}•••• {card.last4}
                </span>
                <PaymentProviderLabel
                  provider={card.provider}
                  className="text-muted-foreground text-xs"
                />
              </span>
              {canWrite ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  aria-label={t("billing.portal.cards.remove")}
                  onClick={() => setRemoving(card.uuid)}
                >
                  <Trash2 className="size-4" />
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
        <p className="text-muted-foreground text-xs">
          {t("billing.portal.cards.hint")}
        </p>
        {pending.length ? (
          <div className="space-y-1 border-t pt-3">
            <p className="flex items-center gap-2 text-sm font-medium">
              <Landmark className="size-4" />
              {t("billing.portal.cards.transfers_pending")}
            </p>
            <ul className="text-muted-foreground space-y-0.5 text-xs">
              {pending.map((tr) => (
                <li key={tr.uuid} className="tabular-nums">
                  {tr.payment_reference} ·{" "}
                  {formatMinor(tr.amount_minor, "TRY", locale)} ·{" "}
                  {date(tr.created_at, undefined, locale)}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
      <ConfirmDialog
        open={Boolean(removing)}
        title={t("billing.portal.cards.remove_title")}
        description={t("billing.portal.cards.remove_description")}
        confirmLabel={t("billing.portal.cards.remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setRemoving(null)}
        onConfirm={() => {
          if (removing) remove.mutate(removing);
          setRemoving(null);
        }}
      />
    </EntitySectionCard>
  );
}

function ProfileCard({ canWrite }: { canWrite: boolean }) {
  const { t } = useLocale();
  const profile = useBillingProfile();
  return (
    <EntitySectionCard title={t("billing.portal.profile.title")}>
      {profile.isLoading ? <Skeleton className="h-64 w-full" /> : null}
      {profile.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void profile.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {profile.isSuccess ? (
        <div className="space-y-3">
          {profile.data === null ? (
            <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm">
              {t("billing.portal.profile.missing")}
            </p>
          ) : null}
          <BillingProfileForm profile={profile.data} disabled={!canWrite} />
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/**
 * Customer billing portal (CF-33) at `/billing` (the gateways return here):
 * current plan, trial and renewal, open invoice prompt, usage vs limits,
 * plan change with proration preview, cancel with reason and retention
 * offer, credits (balances, packs, alerts / auto top-up, ledger; CF-143),
 * invoices with PDF, saved cards and pending transfers, coupon and billing
 * profile.
 */
export function BillingPortalPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.billing.tenantWrite);
  const sub = useTenantSubscription();
  const catalog = useCatalog();
  const ents = useTenantUsage();
  const firstInvoices = useTenantInvoices(0);
  const [paying, setPaying] = useState<BillingInvoiceSummary | null>(null);
  const [changing, setChanging] = useState(false);
  const trialInvoice = useTrialInvoice();
  const search = useSearchParams();
  const payTrial = () => {
    if (!sub.data) return;
    trialInvoice.mutate(sub.data.uuid, { onSuccess: setPaying });
  };
  // `/billing?pay=trial` comes from the sign-up funnel (CF-42) when the
  // customer chose to pay right away; open the payment once.
  const autoPay = useRef(false);
  useEffect(() => {
    if (autoPay.current || search.get("pay") !== "trial") return;
    const current = sub.data;
    if (
      !current ||
      (current.status !== "trialing" && current.status !== "pending_payment") ||
      !canWrite
    )
      return;
    autoPay.current = true;
    trialInvoice.mutate(current.uuid, { onSuccess: setPaying });
  }, [search, sub.data, canWrite, trialInvoice]);
  const openInvoice = firstInvoices.data?.items.find(
    (i) => i.status === "open",
  );
  const s = sub.data;
  const currentPrice = s
    ? resolvePlan(catalog.data?.items, s.version_uuid)?.version.prices.find(
        (p) => p.uuid === s.price_uuid,
      )
    : undefined;
  const seatsInUse = ents.data?.find((u) => u.key === "seats")?.quantity ?? 1;

  return (
    <EntityPage
      title={t("billing.portal.title")}
      description={t("billing.portal.description")}
      permission={permissions.billing.tenantRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("billing.portal.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("billing.portal.title") },
      ]}
    >
      <div className="space-y-10">
        <PaymentReturnBanner />
        <div className="grid gap-6 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
          <div className="min-w-0 space-y-6">
            <OverviewCard
              canWrite={canWrite}
              onPayTrial={payTrial}
              trialPending={trialInvoice.isPending}
              openInvoice={openInvoice}
              onPay={setPaying}
              onChangePlan={() => setChanging(true)}
            />
            <UsageCard />
          </div>
          <div className="min-w-0 space-y-6">
            <PaymentMethodsCard canWrite={canWrite} />
            {canWrite ? (
              <EntitySectionCard title={t("billing.portal.coupon.title")}>
                <CouponForm />
              </EntitySectionCard>
            ) : null}
          </div>
        </div>
        <CreditsSection
          canWrite={canWrite}
          vatBasisPoints={currentPrice?.vat_basis_points ?? null}
        />
        <section
          aria-labelledby="invoices-heading"
          className="grid gap-6 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]"
        >
          <h2 id="invoices-heading" className="sr-only">
            {t("billing.portal.invoices.title")}
          </h2>
          <div className="min-w-0">
            <InvoicesCard canWrite={canWrite} onPay={setPaying} />
          </div>
          <div className="min-w-0">
            <ProfileCard canWrite={canWrite} />
          </div>
        </section>
      </div>
      {paying ? (
        <PayDialog invoice={paying} onClose={() => setPaying(null)} />
      ) : null}
      {changing && s ? (
        <PlanChangeDialog
          subscription={s}
          currentPrice={currentPrice}
          seatsInUse={seatsInUse}
          onClose={() => setChanging(false)}
          onPayNow={() => {
            void firstInvoices.refetch().then((r) => {
              const inv = r.data?.items.find((i) => i.status === "open");
              if (inv) setPaying(inv);
            });
          }}
        />
      ) : null}
    </EntityPage>
  );
}
