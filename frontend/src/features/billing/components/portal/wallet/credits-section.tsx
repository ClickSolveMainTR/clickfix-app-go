"use client";

import { CreditPacksCard } from "@/features/billing/components/portal/wallet/credit-packs-card";
import { WalletBalances } from "@/features/billing/components/portal/wallet/wallet-balances";
import { WalletLedgerCard } from "@/features/billing/components/portal/wallet/wallet-ledger-card";
import { WalletSettingsCard } from "@/features/billing/components/portal/wallet/wallet-settings-card";
import {
  useTenantEntitlements,
  useTenantWallet,
} from "@/features/billing/hooks/use-tenant-billing";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Krediler" block of the billing page (CF-143): balances, packs to buy,
 * low balance alert + auto top-up, and the credit ledger.
 */
export function CreditsSection({
  canWrite,
  vatBasisPoints,
}: {
  canWrite: boolean;
  vatBasisPoints: number | null;
}) {
  const { t } = useLocale();
  const wallet = useTenantWallet(0);
  const ents = useTenantEntitlements();
  const allowance = ents.data?.values["ai.credits"];
  // Backend default when no level is saved: a fifth of the plan allowance.
  const recommended =
    allowance && allowance.kind === "limit" && !allowance.unlimited
      ? Math.floor(allowance.limit / 5)
      : null;
  const settings = wallet.data?.settings;
  const alertLevel = settings
    ? (settings.alert_threshold ?? recommended)
    : null;

  return (
    <section aria-labelledby="credits-heading" className="space-y-4">
      <div className="space-y-1">
        <h2
          id="credits-heading"
          className="text-lg font-semibold tracking-tight"
        >
          {t("wallet.section.title")}
        </h2>
        <p className="text-muted-foreground text-sm">
          {t("wallet.section.description")}
        </p>
      </div>
      <WalletBalances
        wallet={wallet.data}
        isLoading={wallet.isLoading}
        alertLevel={alertLevel}
      />
      <div className="grid gap-6 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <div className="min-w-0">
          <CreditPacksCard
            canWrite={canWrite}
            vatBasisPoints={vatBasisPoints}
          />
        </div>
        <div className="min-w-0">
          <WalletSettingsCard
            settings={settings}
            canWrite={canWrite}
            recommended={recommended}
          />
        </div>
      </div>
      <WalletLedgerCard />
    </section>
  );
}
