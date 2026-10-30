"use client";

import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BankAccountsPanel } from "@/features/billing/components/platform/bank-accounts-panel";
import { CreditProductsPanel } from "@/features/billing/components/platform/catalog/credit-products-panel";
import { PaymentsPanel } from "@/features/billing/components/platform/payments-panel";
import { PlansPanel } from "@/features/billing/components/platform/plans-panel";
import { SubscriptionsPanel } from "@/features/billing/components/platform/subscriptions-panel";
import { TransfersPanel } from "@/features/billing/components/platform/transfers-panel";
import { useBillingStats } from "@/features/billing/hooks/use-platform-billing";
import { formatCount, formatMinor } from "@/features/billing/lib/money";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const TABS = [
  "subscriptions",
  "transfers",
  "payments",
  "plans",
  "credits",
  "bank",
] as const;
type Tab = (typeof TABS)[number];

function resolveTab(value?: string): Tab {
  return (TABS as readonly string[]).includes(value ?? "")
    ? (value as Tab)
    : "subscriptions";
}

function Stat({
  label,
  value,
  hint,
}: {
  label: string;
  value: React.ReactNode;
  hint?: string;
}) {
  return (
    <div className="bg-card rounded-xl border px-4 py-3">
      <p className="text-muted-foreground text-xs font-medium">{label}</p>
      <p className="mt-1 text-xl font-semibold tracking-tight tabular-nums">
        {value}
      </p>
      {hint ? (
        <p className="text-muted-foreground mt-0.5 text-xs">{hint}</p>
      ) : null}
    </div>
  );
}

/** A count from the API, 0 when missing or not a finite number. */
function count(v: unknown) {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

function StatsRow() {
  const { t, locale } = useLocale();
  const stats = useBillingStats();
  if (stats.isLoading) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-[74px] rounded-xl" />
        ))}
      </div>
    );
  }
  const data = stats.data;
  if (!data) return null;
  // Every card guards its number: a missing field must render 0, never NaN.
  // `live` falls back to the sum of the live statuses it is made of.
  const by: Record<string, number | undefined> = data.by_status ?? {};
  const live =
    typeof data.live === "number" && Number.isFinite(data.live)
      ? data.live
      : Object.values(by).reduce<number>((sum, v) => sum + count(v), 0);
  const atRisk = count(by.past_due) + count(by.grace) + count(by.suspended);
  const mrr = Object.entries(data.mrr_minor ?? {}).filter(
    ([, v]) => count(v) > 0,
  );
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <Stat
        label={t("billing.stats.live")}
        value={formatCount(live, locale)}
        hint={t("billing.stats.active_hint", {
          count: formatCount(count(by.active), locale),
        })}
      />
      <Stat
        label={t("billing.stats.trialing")}
        value={formatCount(count(by.trialing), locale)}
      />
      <Stat
        label={t("billing.stats.at_risk")}
        value={formatCount(atRisk, locale)}
        hint={t("billing.stats.at_risk_hint")}
      />
      <Stat
        label={t("billing.stats.mrr")}
        value={
          mrr.length
            ? mrr
                .map(([currency, minor]) =>
                  formatMinor(minor, currency, locale, { compact: true }),
                )
                .join(" · ")
            : formatMinor(0, "TRY", locale, { compact: true })
        }
        hint={t("billing.stats.mrr_hint")}
      />
    </div>
  );
}

/**
 * Platform billing management (CF-39): live subscription stats, the
 * subscription list across organizations, the bank transfer approval queue,
 * payments (failed by default, refunds), the plan catalog, credit products
 * (CF-143) and the seller's bank accounts. Per-organization operations live on the organization page
 * (`?tab=billing`).
 */
export function BillingAdminPage({ initialTab }: { initialTab?: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const [tab, setTab] = useState<Tab>(() => resolveTab(initialTab));
  const open = (next: string) => {
    const resolved = resolveTab(next);
    setTab(resolved);
    router.replace(`${pathname}?tab=${resolved}`, { scroll: false });
  };
  const canPlans = can(permissions.billing.plansRead);
  const canCredits = can(permissions.billing.subscriptionsRead);
  const visible = (value: Tab) =>
    value === "plans" ? canPlans : value === "credits" ? canCredits : true;

  return (
    <EntityPage
      title={t("billing.admin.title")}
      description={t("billing.admin.description")}
      permission={[
        permissions.billing.subscriptionsRead,
        permissions.billing.plansRead,
      ]}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("billing.admin.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("billing.admin.title") },
      ]}
    >
      <div className="space-y-6">
        {can(permissions.billing.subscriptionsRead) ? <StatsRow /> : null}
        <Tabs value={tab} onValueChange={open} className="space-y-4">
          <div className="-mx-1 overflow-x-auto px-1 pb-1">
            <TabsList className="h-auto w-max">
              {TABS.filter(visible).map((value) => (
                <TabsTrigger key={value} value={value}>
                  {value === "credits"
                    ? t("credit_products.tab")
                    : t(`billing.admin.tabs.${value}`)}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
          <TabsContent value="subscriptions">
            <SubscriptionsPanel />
          </TabsContent>
          <TabsContent value="transfers">
            <TransfersPanel />
          </TabsContent>
          <TabsContent value="payments">
            <PaymentsPanel />
          </TabsContent>
          {canPlans ? (
            <TabsContent value="plans">
              <PlansPanel />
            </TabsContent>
          ) : null}
          {canCredits ? (
            <TabsContent value="credits">
              <CreditProductsPanel />
            </TabsContent>
          ) : null}
          <TabsContent value="bank">
            <BankAccountsPanel />
          </TabsContent>
        </Tabs>
      </div>
    </EntityPage>
  );
}
