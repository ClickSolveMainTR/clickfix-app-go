"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CreditCard, Landmark } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { BillingStatus } from "@/features/billing/components/billing-status";
import { useAdminSubscriptions } from "@/features/billing/hooks/use-platform-billing";
import { formatMinor, monthlyEquivalent } from "@/features/billing/lib/money";
import type {
  AdminListParams,
  BillingAdminSubscription,
} from "@/features/billing/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FILTERS = [
  "live",
  "all",
  "trialing",
  "active",
  "past_due",
  "grace",
  "suspended",
  "paused",
  "canceled",
  "expired",
] as const;

/** Next date that matters for the row: trial end, grace end or renewal. */
function nextDate(row: BillingAdminSubscription) {
  if (row.status === "trialing" && row.trial_ends_at) return row.trial_ends_at;
  if (row.status === "grace" && row.grace_ends_at) return row.grace_ends_at;
  return row.period_end;
}

export function SubscriptionsPanel() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const [filter, setFilter] = useState<(typeof FILTERS)[number]>("live");
  const listState = useServerListState({ initialPageSize: 20 });
  const { limit, offset, q } = listState.params;
  const params = useMemo<AdminListParams>(
    () => ({
      limit,
      offset,
      q,
      live: filter === "live" ? "true" : undefined,
      status: filter === "live" || filter === "all" ? undefined : filter,
    }),
    [filter, limit, offset, q],
  );
  const query = useAdminSubscriptions(params);

  const columns = useMemo(
    () =>
      [
        createColumn<BillingAdminSubscription>({
          id: "org",
          accessorKey: "organization_name",
          labelKey: "billing.subscriptions.columns.organization",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex min-w-0 flex-col">
              <span className="truncate font-medium">
                {row.original.organization_name}
              </span>
              <code className="text-muted-foreground truncate text-xs">
                {row.original.organization_slug}
              </code>
            </span>
          ),
        }),
        createColumn<BillingAdminSubscription>({
          id: "plan",
          accessorKey: "plan_name",
          labelKey: "billing.subscriptions.columns.plan",
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span>{row.original.plan_name}</span>
              <span className="text-muted-foreground text-xs">
                v{row.original.version_number} ·{" "}
                {t(`billing.interval.${row.original.price_interval}`)}
              </span>
            </span>
          ),
        }),
        createColumn<BillingAdminSubscription>({
          id: "status",
          accessorKey: "status",
          labelKey: "billing.subscriptions.columns.status",
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1.5">
              <BillingStatus kind="subscription" status={row.original.status} />
              {row.original.cancel_at_period_end ? (
                <Badge variant="outline">
                  {t("billing.subscriptions.cancel_scheduled")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<BillingAdminSubscription>({
          id: "mrr",
          accessorFn: (row) =>
            monthlyEquivalent(row.price_amount_minor, row.price_interval),
          labelKey: "billing.subscriptions.columns.mrr",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) =>
            row.original.status === "trialing" ? (
              <span className="text-muted-foreground">—</span>
            ) : (
              formatMinor(
                monthlyEquivalent(
                  row.original.price_amount_minor,
                  row.original.price_interval,
                ),
                row.original.price_currency,
                locale,
              )
            ),
        }),
        createColumn<BillingAdminSubscription>({
          id: "next",
          accessorFn: nextDate,
          labelKey: "billing.subscriptions.columns.next",
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="tabular-nums">
                {date(nextDate(row.original), undefined, locale)}
              </span>
              <span className="text-muted-foreground text-xs">
                {row.original.status === "trialing"
                  ? t("billing.subscriptions.next_trial_end")
                  : row.original.status === "grace"
                    ? t("billing.subscriptions.next_grace_end")
                    : row.original.cancel_at_period_end
                      ? t("billing.subscriptions.next_cancel")
                      : t("billing.subscriptions.next_renewal")}
              </span>
            </span>
          ),
        }),
        createColumn<BillingAdminSubscription>({
          id: "method",
          accessorFn: (row) => row.card_last4 ?? "",
          labelKey: "billing.subscriptions.columns.method",
          cell: ({ row }) => {
            const r = row.original;
            return r.card_last4 ? (
              <span className="flex items-center gap-1.5">
                <CreditCard className="text-muted-foreground size-3.5" />
                <span className="tabular-nums">
                  {r.card_brand ? `${r.card_brand} ` : ""}•••• {r.card_last4}
                </span>
              </span>
            ) : r.pending_transfers > 0 ? (
              <span className="flex items-center gap-1.5">
                <Landmark className="size-3.5 text-amber-600" />
                {t("billing.subscriptions.transfer_pending", {
                  count: r.pending_transfers,
                })}
              </span>
            ) : (
              <span className="text-muted-foreground">
                {t("billing.subscriptions.no_method")}
              </span>
            );
          },
        }),
      ] as ColumnDef<BillingAdminSubscription, unknown>[],
    [locale, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={query.isLoading}
      isError={query.isError}
      onRetry={() => void query.refetch()}
      emptyTitle={
        q || filter !== "live"
          ? t("billing.subscriptions.empty_filtered")
          : t("billing.subscriptions.empty")
      }
      emptyDescription={t("billing.subscriptions.empty_hint")}
      onRowClick={(row) =>
        router.push(
          `${routes.platform.organizations.detail(row.organization_uuid)}?tab=billing`,
        )
      }
      searchPlaceholder={t("billing.subscriptions.search")}
      total={query.data?.total ?? 0}
      state={listState.tableState}
      features={{
        // The endpoint takes `q`, `status`, `live`, `limit` and `offset`.
        sorting: false,
        columnFilters: false,
        rowSelection: false,
        viewMode: false,
      }}
      toolbarExtra={
        <Select
          value={filter}
          onValueChange={(v) => {
            setFilter(v as (typeof FILTERS)[number]);
            listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
        >
          <SelectTrigger
            className="h-8 sm:w-52"
            aria-label={t("billing.subscriptions.filter")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FILTERS.map((value) => (
              <SelectItem key={value} value={value}>
                {value === "live" || value === "all"
                  ? t(`billing.subscriptions.filter_${value}`)
                  : t(`billing.status.subscription.${value}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
    />
  );
}
