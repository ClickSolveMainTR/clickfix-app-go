"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  priceBreakdown,
  yearlySavingPercent,
} from "@/features/billing/components/platform/catalog/catalog-lib";
import {
  BILLING_CURRENCIES,
  formatMinor,
  minorToInput,
  parseMajorToMinor,
} from "@/features/billing/lib/money";
import type { BillingPrice } from "@/features/billing/types";
import { useLocale } from "@/providers/locale-provider";

const INTERVALS = ["monthly", "yearly"] as const;

/** Major-unit (TL) text input bound to a minor-unit amount; commits on blur. */
function MoneyInput({
  id,
  value,
  onChange,
  currency,
}: {
  id: string;
  value: number;
  onChange: (minor: number) => void;
  currency: string;
}) {
  const { locale } = useLocale();
  const [text, setText] = useState(() => minorToInput(value, locale));
  const [prev, setPrev] = useState(value);
  if (prev !== value) {
    setPrev(value);
    setText(minorToInput(value, locale));
  }
  const parsed = parseMajorToMinor(text);
  const symbol = currency === "TRY" ? "₺" : currency === "USD" ? "$" : "€";
  return (
    <div className="relative">
      <span className="text-muted-foreground pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-sm">
        {symbol}
      </span>
      <Input
        id={id}
        inputMode="decimal"
        value={text}
        aria-invalid={parsed === null}
        className="ps-7 text-end tabular-nums"
        onChange={(e) => {
          setText(e.target.value);
          const next = parseMajorToMinor(e.target.value);
          if (next !== null && next !== value) {
            // Typing drives the value; do not reformat the text under the caret.
            setPrev(next);
            onChange(next);
          }
        }}
        onBlur={() => {
          if (parsed === null) setText(minorToInput(value, locale));
          else setText(minorToInput(parsed, locale));
        }}
      />
    </div>
  );
}

function Field({
  label,
  htmlFor,
  children,
  hint,
}: {
  label: string;
  htmlFor?: string;
  children: React.ReactNode;
  hint?: string;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor} className="text-xs font-medium">
        {label}
      </Label>
      {children}
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
    </div>
  );
}

function Breakdown({
  price,
  monthly,
}: {
  price: BillingPrice;
  monthly?: BillingPrice;
}) {
  const { t, locale } = useLocale();
  const b = priceBreakdown(price);
  const fmt = (v: number) => formatMinor(v, price.currency, locale);
  const saving =
    price.interval === "yearly" && monthly
      ? yearlySavingPercent(
          priceBreakdown(monthly).gross,
          priceBreakdown(price).gross,
        )
      : null;
  return (
    <p className="text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs tabular-nums">
      <span>
        {t("credit_products.price.net")}{" "}
        <span className="text-foreground">{fmt(b.net)}</span>
      </span>
      <span>
        {t("credit_products.price.vat_amount", {
          rate: price.vat_basis_points / 100,
        })}{" "}
        <span className="text-foreground">{fmt(b.vat)}</span>
      </span>
      <span>
        {t("credit_products.price.gross")}{" "}
        <span className="text-foreground font-medium">{fmt(b.gross)}</span>
      </span>
      {price.interval === "yearly" ? (
        <span>
          {t("credit_products.price.per_month", {
            price: fmt(Math.round(b.gross / 12)),
          })}
          {saving ? (
            <span className="ms-1 text-emerald-700 dark:text-emerald-400">
              {t("credit_products.price.saving", { percent: saving })}
            </span>
          ) : null}
        </span>
      ) : null}
    </p>
  );
}

/**
 * Prices of a version as one card per interval × currency. Amounts are typed
 * in TL (major units) and stored in kuruş; KDV is a percentage on screen and
 * basis points in the API.
 */
export function PricesEditor({
  prices,
  onChange,
  readOnly,
}: {
  prices: BillingPrice[];
  onChange: (prices: BillingPrice[]) => void;
  readOnly: boolean;
}) {
  const { t, locale } = useLocale();
  const update = (i: number, patch: Partial<BillingPrice>) =>
    onChange(prices.map((p, j) => (j === i ? { ...p, ...patch } : p)));
  const used = new Set(prices.map((p) => `${p.interval}:${p.currency}`));
  const free = INTERVALS.flatMap((interval) =>
    BILLING_CURRENCIES.map((currency) => ({ interval, currency })),
  ).filter((c) => !used.has(`${c.interval}:${c.currency}`));
  const duplicate = used.size !== prices.length;
  const monthlyOf = (currency: string) =>
    prices.find((p) => p.interval === "monthly" && p.currency === currency);
  const add = (c: { interval: BillingPrice["interval"]; currency: string }) =>
    onChange([
      ...prices,
      {
        interval: c.interval,
        currency: c.currency as BillingPrice["currency"],
        amount_minor: 0,
        seat_amount_minor: 0,
        tax_inclusive: false,
        vat_basis_points: 2000,
        tiers: [],
      },
    ]);
  const yearlyTry = free.find(
    (c) => c.interval === "yearly" && c.currency === "TRY",
  );

  return (
    <div className="space-y-3">
      {prices.map((price, i) => {
        const id = `price-${i}`;
        const title = t("credit_products.price.card_title", {
          interval: t(`billing.interval.${price.interval}`),
          currency: price.currency,
        });
        return (
          <div
            key={`${i}-${price.interval}-${price.currency}`}
            className="bg-card space-y-3 rounded-lg border p-3 sm:p-4"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              {readOnly ? (
                <h4 className="text-sm font-medium">{title}</h4>
              ) : (
                <div className="flex flex-wrap items-center gap-2">
                  <Select
                    value={price.interval}
                    onValueChange={(v) =>
                      update(i, { interval: v as BillingPrice["interval"] })
                    }
                  >
                    <SelectTrigger
                      className="h-8 w-28"
                      aria-label={t("billing.versions.price.interval")}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {INTERVALS.map((v) => (
                        <SelectItem key={v} value={v}>
                          {t(`billing.interval.${v}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Select
                    value={price.currency}
                    onValueChange={(v) =>
                      update(i, { currency: v as BillingPrice["currency"] })
                    }
                  >
                    <SelectTrigger
                      className="h-8 w-24"
                      aria-label={t("billing.versions.price.currency")}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {BILLING_CURRENCIES.map((c) => (
                        <SelectItem key={c} value={c}>
                          {c}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}
              {readOnly ? (
                (price.tiers?.length ?? 0) > 0 ? (
                  <span className="text-muted-foreground text-xs">
                    {t("billing.versions.price.tiers", {
                      count: price.tiers?.length ?? 0,
                    })}
                  </span>
                ) : null
              ) : (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="text-muted-foreground h-8"
                  disabled={prices.length <= 1}
                  onClick={() => onChange(prices.filter((_, j) => j !== i))}
                >
                  <Trash2 className="size-3.5" />
                  {t("billing.versions.price.remove")}
                </Button>
              )}
            </div>

            {readOnly ? (
              <dl className="grid gap-3 text-sm sm:grid-cols-4">
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("credit_products.price.amount")}
                  </dt>
                  <dd className="font-medium tabular-nums">
                    {formatMinor(price.amount_minor, price.currency, locale)}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("credit_products.price.seat")}
                  </dt>
                  <dd className="tabular-nums">
                    {formatMinor(
                      price.seat_amount_minor ?? 0,
                      price.currency,
                      locale,
                    )}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("credit_products.price.vat_rate")}
                  </dt>
                  <dd className="tabular-nums">
                    %{price.vat_basis_points / 100}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("credit_products.price.entered_as")}
                  </dt>
                  <dd>
                    {price.tax_inclusive
                      ? t("credit_products.price.vat_included")
                      : t("credit_products.price.vat_excluded")}
                  </dd>
                </div>
              </dl>
            ) : (
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <Field
                  label={t("credit_products.price.amount")}
                  htmlFor={`${id}-amount`}
                >
                  <MoneyInput
                    id={`${id}-amount`}
                    value={price.amount_minor}
                    currency={price.currency}
                    onChange={(v) => update(i, { amount_minor: v })}
                  />
                </Field>
                <Field
                  label={t("credit_products.price.seat")}
                  htmlFor={`${id}-seat`}
                >
                  <MoneyInput
                    id={`${id}-seat`}
                    value={price.seat_amount_minor ?? 0}
                    currency={price.currency}
                    onChange={(v) => update(i, { seat_amount_minor: v })}
                  />
                </Field>
                <Field
                  label={t("credit_products.price.vat_rate")}
                  htmlFor={`${id}-vat`}
                >
                  <div className="relative">
                    <span className="text-muted-foreground pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-sm">
                      %
                    </span>
                    <Input
                      id={`${id}-vat`}
                      type="number"
                      min={0}
                      max={100}
                      step={1}
                      className="ps-7 text-end tabular-nums"
                      value={price.vat_basis_points / 100}
                      onChange={(e) => {
                        const pct = Number(e.target.value);
                        if (Number.isFinite(pct) && pct >= 0 && pct <= 100)
                          update(i, {
                            vat_basis_points: Math.round(pct * 100),
                          });
                      }}
                    />
                  </div>
                </Field>
                <Field label={t("credit_products.price.entered_as")}>
                  <label className="flex h-9 items-center gap-2 text-sm">
                    <Switch
                      checked={Boolean(price.tax_inclusive)}
                      onCheckedChange={(v) => update(i, { tax_inclusive: v })}
                      aria-label={t("billing.versions.price.tax_inclusive")}
                    />
                    {price.tax_inclusive
                      ? t("credit_products.price.vat_included")
                      : t("credit_products.price.vat_excluded")}
                  </label>
                </Field>
              </div>
            )}
            <Breakdown price={price} monthly={monthlyOf(price.currency)} />
          </div>
        );
      })}
      {duplicate ? (
        <p className="text-destructive text-xs">
          {t("billing.versions.price.duplicate")}
        </p>
      ) : null}
      {!readOnly && free.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {yearlyTry ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => add(yearlyTry)}
            >
              <Plus className="size-4" />
              {t("credit_products.price.add_yearly")}
            </Button>
          ) : null}
          {free.some((c) => c !== yearlyTry) ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => add(free.find((c) => c !== yearlyTry) ?? free[0])}
            >
              <Plus className="size-4" />
              {t("billing.versions.price.add")}
            </Button>
          ) : null}
        </div>
      ) : null}
      <p className="text-muted-foreground text-xs">
        {t("credit_products.price.seat_hint")}
      </p>
    </div>
  );
}
