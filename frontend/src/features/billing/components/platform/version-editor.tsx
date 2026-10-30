"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { EntitlementsEditor } from "@/features/billing/components/platform/catalog/entitlements-editor";
import { PricePreviewCard } from "@/features/billing/components/platform/catalog/price-preview-card";
import { PricesEditor } from "@/features/billing/components/platform/catalog/prices-editor";
import type {
  BillingFeature,
  BillingVersionInput,
} from "@/features/billing/types";
import { useLocale } from "@/providers/locale-provider";

function Section({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-semibold">{title}</h3>
        {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
      </div>
      {children}
    </section>
  );
}

/**
 * Commercial terms of one plan version: trial days, prices (typed in TL with
 * KDV, stored in kuruş) and a value for every catalog feature, grouped into
 * readable sections, next to a live pricing-card preview. Published
 * versions render read-only (DB-immutable).
 */
export function VersionEditor({
  features,
  value,
  onChange,
  readOnly,
  planName,
  filled,
}: {
  features: BillingFeature[];
  value: BillingVersionInput;
  onChange: (next: BillingVersionInput) => void;
  readOnly: boolean;
  planName: string;
  /** Feature keys prefilled from the catalog default. */
  filled?: string[];
}) {
  const { t } = useLocale();
  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_300px]">
      <div className="min-w-0 space-y-8">
        <Section
          title={t("credit_products.version.terms")}
          hint={t("credit_products.version.terms_hint")}
        >
          <div className="max-w-xs space-y-1.5">
            <Label htmlFor="version-trial" className="text-xs font-medium">
              {t("billing.versions.trial_days")}
            </Label>
            <Input
              id="version-trial"
              type="number"
              min={0}
              max={365}
              disabled={readOnly}
              className="tabular-nums"
              value={value.trial_days ?? 0}
              onChange={(e) => {
                const n = Number(e.target.value);
                if (Number.isInteger(n) && n >= 0 && n <= 365)
                  onChange({ ...value, trial_days: n });
              }}
            />
            <p className="text-muted-foreground text-xs">
              {t("billing.versions.trial_hint")}
            </p>
          </div>
          <PricesEditor
            prices={value.prices}
            readOnly={readOnly}
            onChange={(prices) => onChange({ ...value, prices })}
          />
        </Section>

        <Section
          title={t("billing.versions.entitlements")}
          hint={
            filled?.length
              ? t("credit_products.version.filled_hint", {
                  count: filled.length,
                })
              : undefined
          }
        >
          <EntitlementsEditor
            features={features}
            grants={value.entitlements}
            readOnly={readOnly}
            filled={filled}
            onChange={(entitlements) => onChange({ ...value, entitlements })}
          />
        </Section>
      </div>
      <aside className="xl:sticky xl:top-4 xl:self-start">
        <PricePreviewCard
          planName={planName}
          features={features}
          value={value}
        />
      </aside>
    </div>
  );
}
