"use client";

import { CreditCard, Landmark, Search } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import {
  BillingTable,
  type BillingColumn,
} from "@/features/billing/components/billing-table";
import { BillingStatus } from "@/features/billing/components/billing-status";
import { useAdminSubscriptions } from "@/features/billing/hooks/use-platform-billing";
import { formatMinor, monthlyEquivalent } from "@/features/billing/lib/money";
import type {
  AdminListParams,
  BillingAdminSubscription,
} from "@/features/billing/types";
import { useDebounce } from "@/hooks/use-debounce";
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

const LIMIT = 20;

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
  const [search, setSearch] = useState("");
  const [offset, setOffset] = useState(0);
  const q = useDebounce(search, 300);
  const params = useMemo<AdminListParams>(
    () => ({
      limit: LIMIT,
      offset,
      q: q.trim() || undefined,
      live: filter === "live" ? "true" : undefined,
      status: filter === "live" || filter === "all" ? undefined : filter,
    }),
    [filter, offset, q],
  );
  const query = useAdminSubscriptions(params);

  const columns: BillingColumn<BillingAdminSubscription>[] = [
    {
      key: "org",
      header: t("billing.subscriptions.columns.organization"),
      cell: (row) => (
        <span className="flex min-w-0 flex-col">
          <span className="truncate font-medium">{row.organization_name}</span>
          <code className="text-muted-foreground truncate text-xs">
            {row.organization_slug}
          </code>
        </span>
      ),
    },
    {
      key: "plan",
      header: t("billing.subscriptions.columns.plan"),
      cell: (row) => (
        <span className="flex flex-col">
          <span>{row.plan_name}</span>
          <span className="text-muted-foreground text-xs">
            v{row.version_number} ·{" "}
            {t(`billing.interval.${row.price_interval}`)}
          </span>
        </span>
      ),
    },
    {
      key: "status",
      header: t("billing.subscriptions.columns.status"),
      cell: (row) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <BillingStatus kind="subscription" status={row.status} />
          {row.cancel_at_period_end ? (
            <Badge variant="outline">
              {t("billing.subscriptions.cancel_scheduled")}
            </Badge>
          ) : null}
        </span>
      ),
    },
    {
      key: "mrr",
      header: t("billing.subscriptions.columns.mrr"),
      align: "end",
      cell: (row) =>
        row.status === "trialing" ? (
          <span className="text-muted-foreground">—</span>
        ) : (
          formatMinor(
            monthlyEquivalent(row.price_amount_minor, row.price_interval),
            row.price_currency,
            locale,
          )
        ),
    },
    {
      key: "next",
      header: t("billing.subscriptions.columns.next"),
      cell: (row) => (
        <span className="flex flex-col">
          <span className="tabular-nums">
            {date(nextDate(row), undefined, locale)}
          </span>
          <span className="text-muted-foreground text-xs">
            {row.status === "trialing"
              ? t("billing.subscriptions.next_trial_end")
              : row.status === "grace"
                ? t("billing.subscriptions.next_grace_end")
                : row.cancel_at_period_end
                  ? t("billing.subscriptions.next_cancel")
                  : t("billing.subscriptions.next_renewal")}
          </span>
        </span>
      ),
    },
    {
      key: "method",
      header: t("billing.subscriptions.columns.method"),
      cell: (row) =>
        row.card_last4 ? (
          <span className="flex items-center gap-1.5">
            <CreditCard className="text-muted-foreground size-3.5" />
            <span className="tabular-nums">
              {row.card_brand ? `${row.card_brand} ` : ""}•••• {row.card_last4}
            </span>
          </span>
        ) : row.pending_transfers > 0 ? (
          <span className="flex items-center gap-1.5">
            <Landmark className="size-3.5 text-amber-600" />
            {t("billing.subscriptions.transfer_pending", {
              count: row.pending_transfers,
            })}
          </span>
        ) : (
          <span className="text-muted-foreground">
            {t("billing.subscriptions.no_method")}
          </span>
        ),
    },
  ];

  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="relative sm:max-w-xs sm:flex-1">
          <Search className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2" />
          <Input
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setOffset(0);
            }}
            placeholder={t("billing.subscriptions.search")}
            aria-label={t("billing.subscriptions.search")}
            className="ps-8"
          />
        </div>
        <Select
          value={filter}
          onValueChange={(v) => {
            setFilter(v as (typeof FILTERS)[number]);
            setOffset(0);
          }}
        >
          <SelectTrigger
            className="sm:w-52"
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
      </div>
      <BillingTable
        caption={t("billing.admin.tabs.subscriptions")}
        columns={columns}
        rows={query.data?.items}
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
        page={
          query.data
            ? {
                total: query.data.total,
                limit: LIMIT,
                offset,
                onOffsetChange: setOffset,
              }
            : undefined
        }
      />
    </div>
  );
}
