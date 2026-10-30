"use client";

import { useState } from "react";

import { EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import {
  BillingTable,
  type BillingColumn,
} from "@/features/billing/components/billing-table";
import {
  categoryLabel,
  reasonLabel,
  signedAmount,
  unitLabel,
} from "@/features/billing/components/portal/wallet/wallet-format";
import {
  useTenantWallet,
  WALLET_PAGE_SIZE,
} from "@/features/billing/hooks/use-tenant-billing";
import type { TenantWalletEntry } from "@/features/billing/services/tenant-billing.service";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Credit ledger with readable reasons, newest first, paged by ten. */
export function WalletLedgerCard() {
  const { t, locale } = useLocale();
  const [offset, setOffset] = useState(0);
  const wallet = useTenantWallet(offset);
  const entries = wallet.data?.entries;

  const columns: BillingColumn<TenantWalletEntry>[] = [
    {
      key: "date",
      header: t("wallet.ledger.date"),
      cell: (e) => (
        <span className="text-muted-foreground whitespace-nowrap tabular-nums">
          {e.created_at ? datetime(e.created_at, undefined, locale) : "—"}
        </span>
      ),
    },
    {
      key: "reason",
      header: t("wallet.ledger.reason"),
      cell: (e) => {
        const category = categoryLabel(t, e.category ?? null);
        return (
          <span className="flex flex-col gap-1">
            <span>{reasonLabel(t, e.reason)}</span>
            {category ? (
              <Badge variant="outline" className="w-fit font-normal">
                {t("wallet.ledger.by_platform", { category })}
              </Badge>
            ) : null}
          </span>
        );
      },
    },
    {
      key: "unit",
      header: t("wallet.ledger.unit"),
      cell: (e) => (
        <span className="text-muted-foreground whitespace-nowrap">
          {unitLabel(t, e.unit)}
        </span>
      ),
    },
    {
      key: "amount",
      header: t("wallet.ledger.amount"),
      align: "end",
      cell: (e) => {
        const delta = e.delta ?? 0;
        return (
          <span
            className={cn(
              "font-medium whitespace-nowrap tabular-nums",
              delta > 0 && "text-emerald-700 dark:text-emerald-400",
            )}
          >
            {signedAmount(e.unit, delta, locale)}
          </span>
        );
      },
    },
    {
      key: "expires",
      header: t("wallet.ledger.expires"),
      align: "end",
      cell: (e) => (
        <span className="text-muted-foreground whitespace-nowrap tabular-nums">
          {e.expires_at && (e.delta ?? 0) > 0
            ? date(e.expires_at, undefined, locale)
            : t("wallet.ledger.no_expiry")}
        </span>
      ),
    },
  ];

  return (
    <EntitySectionCard
      title={t("wallet.ledger.title")}
      description={t("wallet.ledger.description")}
    >
      <BillingTable
        caption={t("wallet.ledger.title")}
        columns={columns}
        rows={entries?.items}
        getRowId={(e) =>
          e.uuid ?? `${e.created_at}-${e.unit}-${e.delta}-${e.reason}`
        }
        isLoading={wallet.isLoading}
        isError={wallet.isError}
        onRetry={() => void wallet.refetch()}
        emptyTitle={t("wallet.ledger.empty")}
        page={
          entries
            ? {
                total: entries.total,
                limit: WALLET_PAGE_SIZE,
                offset,
                onOffsetChange: setOffset,
              }
            : undefined
        }
      />
    </EntitySectionCard>
  );
}
