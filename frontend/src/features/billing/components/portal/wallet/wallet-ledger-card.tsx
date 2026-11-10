"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import {
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
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
  const listState = useServerListState({ initialPageSize: WALLET_PAGE_SIZE });
  const { limit, offset } = listState.params;
  const wallet = useTenantWallet(offset, limit);
  const entries = wallet.data?.entries;

  const columns = useMemo(
    () =>
      [
        createColumn<TenantWalletEntry>({
          accessorKey: "created_at",
          labelKey: "wallet.ledger.date",
          enableHiding: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground whitespace-nowrap tabular-nums">
              {row.original.created_at
                ? datetime(row.original.created_at, undefined, locale)
                : "—"}
            </span>
          ),
        }),
        createColumn<TenantWalletEntry>({
          accessorKey: "reason",
          labelKey: "wallet.ledger.reason",
          gridPrimary: true,
          cell: ({ row }) => {
            const e = row.original;
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
        }),
        createColumn<TenantWalletEntry>({
          accessorKey: "unit",
          labelKey: "wallet.ledger.unit",
          cell: ({ row }) => (
            <span className="text-muted-foreground whitespace-nowrap">
              {unitLabel(t, row.original.unit)}
            </span>
          ),
        }),
        createColumn<TenantWalletEntry>({
          accessorKey: "delta",
          labelKey: "wallet.ledger.amount",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) => {
            const e = row.original;
            const delta = e.delta ?? 0;
            return (
              <span
                className={cn(
                  "font-medium whitespace-nowrap",
                  delta > 0 && "text-emerald-700 dark:text-emerald-400",
                )}
              >
                {signedAmount(e.unit, delta, locale)}
              </span>
            );
          },
        }),
        createColumn<TenantWalletEntry>({
          accessorKey: "expires_at",
          labelKey: "wallet.ledger.expires",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) => {
            const e = row.original;
            return (
              <span className="text-muted-foreground whitespace-nowrap">
                {e.expires_at && (e.delta ?? 0) > 0
                  ? date(e.expires_at, undefined, locale)
                  : t("wallet.ledger.no_expiry")}
              </span>
            );
          },
        }),
      ] as ColumnDef<TenantWalletEntry, unknown>[],
    [locale, t],
  );

  return (
    <EntitySectionCard
      title={t("wallet.ledger.title")}
      description={t("wallet.ledger.description")}
    >
      <EntityTable
        columns={columns}
        data={entries?.items ?? []}
        getRowId={(e) =>
          e.uuid ?? `${e.created_at}-${e.unit}-${e.delta}-${e.reason}`
        }
        isLoading={wallet.isLoading}
        isError={wallet.isError}
        onRetry={() => void wallet.refetch()}
        emptyTitle={t("wallet.ledger.empty")}
        emptyDescription=""
        total={entries?.total ?? 0}
        state={listState.tableState}
        pageSizeOptions={[10, 20, 50]}
        features={{
          // The endpoint takes `limit` and `offset` only; the card is small.
          toolbar: false,
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
    </EntitySectionCard>
  );
}
