"use client";

import {
  Check,
  Clock,
  Copy,
  CreditCard,
  Info,
  Landmark,
  Upload,
} from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BillingProfileForm } from "@/features/billing/components/portal/billing-profile-form";
import {
  useBillingProfile,
  useCheckout,
  usePaymentAvailability,
  useReportTransfer,
  useTenantBankAccounts,
} from "@/features/billing/hooks/use-tenant-billing";
import { billingErrorText } from "@/features/billing/lib/errors";
import { formatIban } from "@/features/billing/lib/iban";
import { formatMinor } from "@/features/billing/lib/money";
import type { BillingInvoiceSummary } from "@/features/billing/types";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const MAX_RECEIPT = 10 * 1024 * 1024;
const RECEIPT_TYPES = ["application/pdf", "image/png", "image/jpeg"];

function CopyField({ label, value }: { label: string; value: string }) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <div className="flex items-center justify-between gap-3 py-2">
      <div className="min-w-0">
        <p className="text-muted-foreground text-xs">{label}</p>
        <p className="truncate font-mono text-sm tabular-nums">{value}</p>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-8 shrink-0"
        aria-label={t("billing.portal.pay.copy", { label })}
        onClick={() => {
          void navigator.clipboard?.writeText(value.replace(/\s/g, ""));
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1500);
        }}
      >
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
      </Button>
    </div>
  );
}

/** Errors that mean card checkout cannot work now: switch to transfer. */
function isCardUnavailable(error: unknown) {
  return (
    error instanceof ApiError &&
    (error.code === "PAYMENT_GATEWAY_NOT_CONFIGURED" ||
      error.code === "PAYMENT_PROVIDER_DISABLED")
  );
}

function CardTab({
  invoice,
  unavailable,
  sandbox,
  onUnavailable,
}: {
  invoice: BillingInvoiceSummary;
  unavailable: boolean;
  sandbox: boolean;
  onUnavailable: () => void;
}) {
  const { t } = useLocale();
  const checkout = useCheckout();
  const [store, setStore] = useState(true);
  const [autoPay, setAutoPay] = useState(true);
  // One key per attempt: a double click or a network retry reuses the same
  // payment and hosted page. A finished attempt (failed / expired) gets a
  // new key so "try again" opens a fresh page.
  const [key, setKey] = useState(() => crypto.randomUUID());
  if (unavailable)
    return (
      <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm">
        {t("wallet.pay.card_unavailable")}
      </p>
    );
  const failed =
    checkout.error && !isCardUnavailable(checkout.error)
      ? billingErrorText(t, checkout.error, "wallet.pay.card_failed")
      : null;
  // The browser is leaving for the provider page after a success.
  const busy = checkout.isPending || checkout.isSuccess;
  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("billing.portal.pay.card_hint")}
      </p>
      {sandbox ? (
        <p className="rounded-md border border-sky-500/30 bg-sky-500/10 px-3 py-2 text-xs">
          {t("billing.portal.pay.sandbox")}
        </p>
      ) : null}
      <Label className="flex items-start gap-3 font-normal">
        <Checkbox
          checked={store}
          disabled={busy}
          onCheckedChange={(v) => {
            setStore(v === true);
            setKey(crypto.randomUUID());
          }}
          className="mt-0.5"
        />
        <span>{t("billing.portal.pay.store_card")}</span>
      </Label>
      <Label
        className={cn(
          "flex items-start gap-3 font-normal",
          !store && "opacity-50",
        )}
      >
        <Checkbox
          checked={store && autoPay}
          disabled={!store || busy}
          onCheckedChange={(v) => {
            setAutoPay(v === true);
            setKey(crypto.randomUUID());
          }}
          className="mt-0.5"
        />
        <span>{t("billing.portal.pay.auto_pay")}</span>
      </Label>
      {failed ? (
        <p role="alert" className="text-destructive text-sm">
          {failed}
        </p>
      ) : null}
      <DialogFooter>
        <Button
          type="button"
          disabled={busy}
          onClick={() =>
            checkout.mutate(
              {
                invoice: invoice.uuid,
                storeCard: store,
                autoPay,
                idempotencyKey: key,
              },
              {
                onError: (error) => {
                  if (isCardUnavailable(error)) onUnavailable();
                  if (
                    error instanceof ApiError &&
                    error.code === "CHECKOUT_FINISHED"
                  )
                    setKey(crypto.randomUUID());
                },
              },
            )
          }
        >
          <CreditCard className="size-4" />
          {busy
            ? t("billing.portal.pay.redirecting")
            : failed
              ? t("billing.portal.pay.retry")
              : t("billing.portal.pay.card_cta")}
        </Button>
      </DialogFooter>
    </div>
  );
}

function TransferTab({
  invoice,
  cardUnavailable,
  onDone,
}: {
  invoice: BillingInvoiceSummary;
  cardUnavailable: boolean;
  onDone: () => void;
}) {
  const { t, locale } = useLocale();
  const accounts = useTenantBankAccounts();
  const report = useReportTransfer();
  const active = (accounts.data?.items ?? []).filter((a) => a.active);
  const [account, setAccount] = useState<string>("");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const selected = active.find((a) => a.uuid === account) ?? active[0];

  if (accounts.isLoading) return <Skeleton className="h-48 w-full" />;
  if (active.length === 0)
    return (
      <p className="text-muted-foreground text-sm">
        {t("billing.portal.pay.no_bank")}
      </p>
    );

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (!selected || !file) return;
        report.mutate(
          {
            invoice: invoice.uuid,
            bankAccount: selected.uuid,
            amountMinor: invoice.total_minor,
            file,
            note: note.trim(),
          },
          { onSuccess: onDone },
        );
      }}
    >
      {cardUnavailable ? (
        <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm">
          {t("wallet.pay.card_unavailable")}
        </p>
      ) : null}
      <ol className="text-muted-foreground list-decimal space-y-1 ps-5 text-sm">
        <li>{t("billing.portal.pay.step_send")}</li>
        <li>{t("billing.portal.pay.step_reference")}</li>
        <li>{t("billing.portal.pay.step_receipt")}</li>
      </ol>
      {active.length > 1 ? (
        <RadioGroup
          value={selected?.uuid}
          onValueChange={setAccount}
          className="gap-2"
          aria-label={t("billing.portal.pay.bank")}
        >
          {active.map((a) => (
            <Label
              key={a.uuid}
              className="flex items-center gap-3 rounded-md border p-2.5 font-normal"
            >
              <RadioGroupItem value={a.uuid} />
              {a.bank_name}
            </Label>
          ))}
        </RadioGroup>
      ) : null}
      {selected ? (
        <div className="divide-y rounded-lg border px-3">
          <CopyField
            label={t("billing.portal.pay.holder")}
            value={selected.account_name}
          />
          <CopyField label="IBAN" value={formatIban(selected.iban)} />
          <CopyField
            label={t("billing.portal.pay.reference")}
            value={invoice.payment_reference}
          />
          <CopyField
            label={t("billing.portal.pay.amount")}
            value={formatMinor(invoice.total_minor, invoice.currency, locale)}
          />
        </div>
      ) : null}
      <div className="space-y-2">
        <Label htmlFor="receipt-file">{t("billing.portal.pay.receipt")}</Label>
        <Input
          id="receipt-file"
          type="file"
          accept=".pdf,.png,.jpg,.jpeg"
          aria-invalid={Boolean(fileError)}
          onChange={(e) => {
            const f = e.target.files?.[0] ?? null;
            if (
              f &&
              (f.size > MAX_RECEIPT || !RECEIPT_TYPES.includes(f.type))
            ) {
              setFile(null);
              setFileError(t("billing.portal.pay.receipt_invalid"));
              return;
            }
            setFileError(null);
            setFile(f);
          }}
        />
        <p
          className={cn(
            "text-xs",
            fileError ? "text-destructive" : "text-muted-foreground",
          )}
        >
          {fileError ?? t("billing.portal.pay.receipt_hint")}
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="transfer-note">{t("billing.portal.pay.note")}</Label>
        <Input
          id="transfer-note"
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>
      <DialogFooter>
        <Button type="submit" disabled={!file || report.isPending}>
          <Upload className="size-4" />
          {t("billing.portal.pay.transfer_cta")}
        </Button>
      </DialogFooter>
    </form>
  );
}

/**
 * Shown when neither card checkout (no payment provider yet) nor a bank
 * account is available: the invoice stays open and the platform team
 * follows up, so the customer is never left at a dead end.
 */
function PaymentNotReady({ onClose }: { onClose: () => void }) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <div className="flex gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm">
        <Clock className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
        <div className="space-y-1">
          <p className="font-medium">{t("wallet.pay.not_ready")}</p>
          <p className="text-muted-foreground">
            {t("wallet.pay.invoice_stays_open")}
          </p>
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.close")}
        </Button>
      </DialogFooter>
    </div>
  );
}

/**
 * Pay an open invoice: card through the gateway's hosted page (PayTR /
 * iyzico; optional saved card + auto-pay consent) or havale/EFT with the
 * seller IBAN, the invoice reference and a receipt upload. Billing details
 * are required first (e-Arşiv); the form opens inline when missing. When no
 * payment provider is configured the card tab steps aside for transfer, and
 * with no bank account either the dialog says the platform team will reach
 * out (the invoice stays open). `note` adds a line above the methods, e.g.
 * that purchased credits arrive once the payment is confirmed.
 */
export function PayDialog({
  invoice,
  onClose,
  note,
}: {
  invoice: BillingInvoiceSummary | null;
  onClose: () => void;
  note?: string;
}) {
  const { t, locale } = useLocale();
  const profile = useBillingProfile();
  const accounts = useTenantBankAccounts();
  const availability = usePaymentAvailability();
  const [tab, setTab] = useState<"card" | "transfer">("card");
  const [cardFailed, setCardFailed] = useState(false);
  // Known up front (CF-154); a checkout error still switches over.
  const cardUnavailable = cardFailed || availability.data?.card.ready === false;
  const sandbox = availability.data?.card.mode === "sandbox";
  if (!invoice) return null;
  const needsProfile = profile.data === null;
  const hasBank = (accounts.data?.items ?? []).some((a) => a.active);
  const notReady = cardUnavailable && accounts.isSuccess && !hasBank;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t("billing.portal.pay.title", {
              amount: formatMinor(
                invoice.total_minor,
                invoice.currency,
                locale,
              ),
            })}
          </DialogTitle>
          <DialogDescription>
            {needsProfile
              ? t("billing.portal.pay.profile_required")
              : t("billing.portal.pay.description", {
                  reference: invoice.payment_reference,
                })}
          </DialogDescription>
        </DialogHeader>
        {note && !needsProfile ? (
          <p className="bg-muted/60 flex items-start gap-2 rounded-md px-3 py-2 text-sm">
            <Info className="text-muted-foreground mt-0.5 size-4 shrink-0" />
            {note}
          </p>
        ) : null}
        {profile.isLoading ? <Skeleton className="h-40 w-full" /> : null}
        {needsProfile ? (
          <BillingProfileForm
            profile={null}
            submitLabel={t("billing.portal.pay.profile_continue")}
          />
        ) : notReady ? (
          <PaymentNotReady onClose={onClose} />
        ) : profile.data ? (
          <Tabs
            value={cardUnavailable ? "transfer" : tab}
            onValueChange={(v) =>
              setTab(v === "transfer" ? "transfer" : "card")
            }
            className="space-y-4"
          >
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="card" disabled={cardUnavailable}>
                <CreditCard className="size-4" />
                {t("billing.portal.pay.card")}
              </TabsTrigger>
              <TabsTrigger value="transfer">
                <Landmark className="size-4" />
                {t("billing.portal.pay.transfer")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="card">
              <CardTab
                invoice={invoice}
                unavailable={cardUnavailable}
                sandbox={sandbox}
                onUnavailable={() => {
                  setCardFailed(true);
                  setTab("transfer");
                }}
              />
            </TabsContent>
            <TabsContent value="transfer">
              <TransferTab
                invoice={invoice}
                cardUnavailable={cardUnavailable}
                onDone={onClose}
              />
            </TabsContent>
          </Tabs>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
