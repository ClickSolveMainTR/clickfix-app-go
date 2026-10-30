"use client";

import { ArrowRight, Minus, Plus } from "lucide-react";
import { useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { AppLocale } from "@/config/i18n";
import {
  ChoiceCard,
  ChoiceCards,
} from "@/features/billing/components/platform/org/choice-cards";
import {
  formatCount,
  formatMinor,
  parseMajorToMinor,
} from "@/features/billing/lib/money";
import type {
  BillingWalletEntryInput,
  BillingWalletUnit,
} from "@/features/billing/types";
import { useAdjustOrganizationWallet } from "@/features/platform-organizations/hooks/use-commerce";
import { endOfDayISO } from "@/features/platform-organizations/lib/access-dates";
import type { WalletCategory } from "@/features/platform-organizations/services/commerce.service";
import { isApiError } from "@/lib/api/client";
import { useLocale } from "@/providers/locale-provider";

export const WALLET_UNITS: BillingWalletUnit[] = [
  "ai_credits",
  "video_minutes",
  "TRY",
];

export const WALLET_UNIT_KEYS: Record<BillingWalletUnit, string> = {
  ai_credits: "commerce.org.units.ai_credits",
  video_minutes: "commerce.org.units.video_minutes",
  TRY: "commerce.org.units.try",
};

const UNIT_SHORT_KEYS: Record<BillingWalletUnit, string> = {
  ai_credits: "commerce.org.units.ai_credits",
  video_minutes: "commerce.org.units.video_minutes_short",
  TRY: "commerce.org.units.try",
};

export const WALLET_CATEGORIES: WalletCategory[] = [
  "goodwill",
  "correction",
  "promotion",
  "sales",
  "migration",
  "other",
];

export const WALLET_CATEGORY_KEYS: Record<WalletCategory, string> = {
  goodwill: "commerce.org.wallet_category.goodwill",
  correction: "commerce.org.wallet_category.correction",
  promotion: "commerce.org.wallet_category.promotion",
  sales: "commerce.org.wallet_category.sales",
  migration: "commerce.org.wallet_category.migration",
  other: "commerce.org.wallet_category.other",
};

/** Backend limit on `reason` (bytes, usecase/wallet.go). */
const REASON_MAX_BYTES = 500;

/** Wallet amount in its unit: TRY as money (kuruş), the rest as counts. */
export function formatWalletAmount(
  unit: string,
  value: number,
  locale: AppLocale,
) {
  return unit === "TRY"
    ? formatMinor(value, "TRY", locale)
    : formatCount(value, locale);
}

function parseAmount(raw: string, unit: BillingWalletUnit): number | null {
  const text = raw.trim().replace(/\s/g, "");
  if (!text) return null;
  const value =
    unit === "TRY"
      ? parseMajorToMinor(text)
      : /^\d+$/.test(text.replace(/\./g, ""))
        ? Number(text.replace(/\./g, ""))
        : null;
  if (value === null || value <= 0 || !Number.isSafeInteger(value)) return null;
  return value;
}

/**
 * "Kredi ekle / düş": unit, direction, amount, category, reason and an
 * optional expiry for additions, with the resulting balance before the
 * operator confirms. A debit never takes the balance under zero (the API
 * answers 409, shown as "Bakiye yetersiz").
 */
export function WalletAdjustDialog({
  organization,
  balances,
  onClose,
}: {
  organization: string;
  balances: Record<BillingWalletUnit, number>;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const adjust = useAdjustOrganizationWallet(organization);
  const [unit, setUnit] = useState<BillingWalletUnit>("ai_credits");
  const [direction, setDirection] = useState<"credit" | "debit">("credit");
  const [amount, setAmount] = useState("");
  const [category, setCategory] = useState<WalletCategory>("goodwill");
  const [reason, setReason] = useState("");
  const [expires, setExpires] = useState("");
  const [serverError, setServerError] = useState<string | null>(null);
  const [openedAt] = useState(() => Date.now());
  // One key per distinct payload: a retry (step-up prompt, dropped response)
  // replays the same key, an edited payload gets a fresh one.
  const attempt = useRef<{ signature: string; key: string } | null>(null);

  const value = parseAmount(amount, unit);
  const amountInvalid = amount.trim() !== "" && value === null;
  const debit = direction === "debit";
  const balance = balances[unit] ?? 0;
  const next = value === null ? null : balance + (debit ? -value : value);
  const insufficient = next !== null && next < 0;
  const expiresAt = !debit && expires ? endOfDayISO(expires) : undefined;
  const expiresInvalid =
    !debit &&
    expires !== "" &&
    (!expiresAt || new Date(expiresAt).getTime() <= openedAt);
  const trimmed = reason.trim();
  const reasonTooLong =
    new TextEncoder().encode(trimmed).length > REASON_MAX_BYTES;
  const valid =
    value !== null &&
    !insufficient &&
    trimmed !== "" &&
    !reasonTooLong &&
    !expiresInvalid;

  const submit = () => {
    if (!valid || value === null) return;
    setServerError(null);
    const delta = debit ? -value : value;
    const payload = {
      unit,
      delta,
      reason: trimmed,
      category,
      expires_at: expiresAt,
    };
    const signature = JSON.stringify(payload);
    if (attempt.current?.signature !== signature) {
      attempt.current = { signature, key: crypto.randomUUID() };
    }
    const body: BillingWalletEntryInput = {
      unit,
      delta,
      reason: trimmed,
      category,
      idempotency_key: attempt.current.key,
      ...(expiresAt ? { expires_at: expiresAt } : {}),
    };
    adjust.mutate(body, {
      onSuccess: onClose,
      onError: (error) => {
        if (isApiError(error) && error.code === "STEP_UP_REQUIRED") return;
        setServerError(
          isApiError(error) && error.status === 409
            ? t("commerce.org.wallet.insufficient")
            : isApiError(error)
              ? error.message
              : t("common.error_generic"),
        );
      },
    });
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-xl">
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <DialogHeader>
            <DialogTitle>{t("commerce.org.wallet.adjust_title")}</DialogTitle>
            <DialogDescription>
              {t("commerce.org.wallet.adjust_description")}
            </DialogDescription>
          </DialogHeader>

          <fieldset className="space-y-2">
            <legend className="mb-2 text-sm font-medium">
              {t("commerce.org.wallet.unit")}
            </legend>
            <ChoiceCards
              value={unit}
              onValueChange={(v) => setUnit(v as BillingWalletUnit)}
              className="grid-cols-1 sm:grid-cols-3"
              aria-label={t("commerce.org.wallet.unit")}
            >
              {WALLET_UNITS.map((u) => (
                <ChoiceCard
                  key={u}
                  value={u}
                  className="p-3"
                  title={
                    <span className="text-sm">{t(UNIT_SHORT_KEYS[u])}</span>
                  }
                  description={t("commerce.org.wallet.current_balance", {
                    amount: formatWalletAmount(u, balances[u] ?? 0, locale),
                  })}
                />
              ))}
            </ChoiceCards>
          </fieldset>

          <fieldset className="space-y-2">
            <legend className="mb-2 text-sm font-medium">
              {t("commerce.org.wallet.direction")}
            </legend>
            <ChoiceCards
              value={direction}
              onValueChange={(v) => setDirection(v as "credit" | "debit")}
              className="grid-cols-2"
              aria-label={t("commerce.org.wallet.direction")}
            >
              <ChoiceCard
                value="credit"
                className="p-3"
                icon={<Plus className="size-4" />}
                title={t("commerce.org.wallet.credit")}
              />
              <ChoiceCard
                value="debit"
                className="p-3"
                icon={<Minus className="size-4" />}
                title={t("commerce.org.wallet.debit")}
              />
            </ChoiceCards>
          </fieldset>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="wallet-amount" className="mb-2">
                {t("commerce.org.wallet.amount")}
              </Label>
              <Input
                id="wallet-amount"
                inputMode={unit === "TRY" ? "decimal" : "numeric"}
                autoComplete="off"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                placeholder={unit === "TRY" ? "250,00" : "500"}
                aria-invalid={amountInvalid}
              />
              <p
                className={
                  amountInvalid
                    ? "text-destructive text-xs"
                    : "text-muted-foreground text-xs"
                }
              >
                {amountInvalid
                  ? unit === "TRY"
                    ? t("commerce.org.wallet.amount_invalid_try")
                    : t("commerce.org.wallet.amount_invalid_count")
                  : unit === "TRY"
                    ? t("commerce.org.wallet.amount_hint_try")
                    : t("commerce.org.wallet.amount_hint_count")}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="wallet-category">
                {t("commerce.org.wallet.category")}
              </Label>
              <Select
                value={category}
                onValueChange={(v) => setCategory(v as WalletCategory)}
              >
                <SelectTrigger id="wallet-category" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {WALLET_CATEGORIES.map((c) => (
                    <SelectItem key={c} value={c}>
                      {t(WALLET_CATEGORY_KEYS[c])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          {!debit ? (
            <div className="space-y-2">
              <Label htmlFor="wallet-expires">
                {t("commerce.org.wallet.expires")}
              </Label>
              <DatePicker
                id="wallet-expires"
                value={expires}
                onChange={setExpires}
                placeholder={t("commerce.org.wallet.expires_never")}
                aria-invalid={expiresInvalid}
              />
              {expiresInvalid ? (
                <p className="text-destructive text-xs">
                  {t("billing.wallet.expires_invalid")}
                </p>
              ) : null}
            </div>
          ) : null}

          <div className="space-y-2">
            <Label htmlFor="wallet-reason">
              {t("commerce.org.common.reason")}
            </Label>
            <Textarea
              id="wallet-reason"
              required
              maxLength={REASON_MAX_BYTES}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("commerce.org.wallet.reason_placeholder")}
              aria-invalid={reasonTooLong}
            />
            <p
              className={
                reasonTooLong
                  ? "text-destructive text-xs"
                  : "text-muted-foreground text-xs"
              }
            >
              {reasonTooLong
                ? t("billing.wallet.reason_too_long")
                : t("commerce.org.common.reason_hint")}
            </p>
          </div>

          <div
            className={
              insufficient
                ? "border-destructive/40 bg-destructive/10 rounded-lg border px-4 py-3 text-sm"
                : "bg-muted/50 rounded-lg px-4 py-3 text-sm"
            }
            aria-live="polite"
          >
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-muted-foreground">
                {t("commerce.org.wallet.summary_current", {
                  amount: formatWalletAmount(unit, balance, locale),
                })}
              </span>
              <ArrowRight className="text-muted-foreground size-4" />
              <span className="font-semibold">
                {t("commerce.org.wallet.summary_next", {
                  amount:
                    next === null
                      ? "—"
                      : formatWalletAmount(unit, next, locale),
                })}
              </span>
            </div>
            {insufficient ? (
              <p className="text-destructive mt-1">
                {t("commerce.org.wallet.insufficient")}
              </p>
            ) : null}
          </div>

          {serverError ? (
            <p role="alert" className="text-destructive text-sm">
              {serverError}
            </p>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              variant={debit ? "destructive" : "default"}
              disabled={!valid || adjust.isPending}
            >
              {debit
                ? t("commerce.org.wallet.submit_debit")
                : t("commerce.org.wallet.submit_credit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
