"use client";

import { Check, Eye } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { grantValueText } from "@/features/billing/components/platform/catalog/entitlements-editor";
import {
  highlightGrants,
  priceBreakdown,
  yearlySavingPercent,
} from "@/features/billing/components/platform/catalog/catalog-lib";
import { formatMinor } from "@/features/billing/lib/money";
import { featureLabel } from "@/features/billing/lib/plans";
import type {
  BillingFeature,
  BillingVersionInput,
} from "@/features/billing/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Fiyat tablosu önizlemesi": how this version would read as a card on the
 * pricing page (TRY prices with and without KDV, yearly saving, trial and the
 * six most telling entitlements). Updates live while the draft is edited.
 */
export function PricePreviewCard({
  planName,
  features,
  value,
}: {
  planName: string;
  features: BillingFeature[];
  value: BillingVersionInput;
}) {
  const { t, locale } = useLocale();
  const monthly = value.prices.find(
    (p) => p.interval === "monthly" && p.currency === "TRY",
  );
  const yearly = value.prices.find(
    (p) => p.interval === "yearly" && p.currency === "TRY",
  );
  const m = monthly ? priceBreakdown(monthly) : null;
  const y = yearly ? priceBreakdown(yearly) : null;
  const saving = yearlySavingPercent(m?.gross, y?.gross);
  const fmt = (v: number) => formatMinor(v, "TRY", locale, { compact: true });
  const top = highlightGrants(features, value.entitlements);
  const trial = value.trial_days ?? 0;

  return (
    <div className="space-y-2">
      <p className="text-muted-foreground flex items-center gap-1.5 text-xs font-medium">
        <Eye className="size-3.5" />
        {t("credit_products.preview.title")}
      </p>
      <div className="bg-card rounded-xl border p-5 shadow-sm">
        <div className="flex items-start justify-between gap-2">
          <h3 className="text-base font-semibold">{planName}</h3>
          {trial > 0 ? (
            <Badge variant="secondary" className="shrink-0 font-normal">
              {t("credit_products.preview.trial", { days: trial })}
            </Badge>
          ) : null}
        </div>

        <div className="mt-4">
          {m ? (
            <>
              <p className="flex items-baseline gap-1">
                <span className="text-3xl font-semibold tracking-tight tabular-nums">
                  {fmt(m.net)}
                </span>
                <span className="text-muted-foreground text-sm">
                  {t("credit_products.preview.per_month")}
                </span>
              </p>
              <p className="text-muted-foreground text-xs tabular-nums">
                {t("credit_products.preview.vat_line", {
                  gross: fmt(m.gross),
                })}
              </p>
            </>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("credit_products.preview.no_monthly")}
            </p>
          )}
        </div>

        {y ? (
          <div className="bg-muted/50 mt-3 rounded-lg px-3 py-2 text-xs tabular-nums">
            <div className="flex flex-wrap items-center gap-x-2">
              <span className="text-foreground font-medium">
                {t("credit_products.preview.yearly", { price: fmt(y.net) })}
              </span>
              {saving ? (
                <Badge variant="success" className="px-1.5 py-0 font-normal">
                  {t("credit_products.price.saving", { percent: saving })}
                </Badge>
              ) : null}
            </div>
            <p className="text-muted-foreground mt-0.5">
              {t("credit_products.preview.yearly_detail", {
                monthly: fmt(Math.round(y.net / 12)),
                gross: fmt(y.gross),
              })}
            </p>
          </div>
        ) : null}

        <ul className="mt-4 space-y-2 border-t pt-4 text-sm">
          {top.length === 0 ? (
            <li className="text-muted-foreground text-xs">
              {t("credit_products.preview.no_features")}
            </li>
          ) : (
            top.map(({ feature, grant }) => (
              <li key={feature.key} className="flex items-start gap-2">
                <Check className="text-primary mt-0.5 size-4 shrink-0" />
                <span className="flex min-w-0 flex-1 justify-between gap-2">
                  <span className="truncate">
                    {featureLabel(feature, locale)}
                  </span>
                  {feature.kind === "boolean" ? null : (
                    <span className="shrink-0 font-medium tabular-nums">
                      {grantValueText(t, locale, feature, grant)}
                    </span>
                  )}
                </span>
              </li>
            ))
          )}
        </ul>
      </div>
      <p className="text-muted-foreground text-xs">
        {t("credit_products.preview.hint")}
      </p>
    </div>
  );
}
