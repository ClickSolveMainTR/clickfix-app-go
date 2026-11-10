"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { BellRing, Bot, RefreshCcw, Video, Wallet } from "lucide-react";
import { useState, type ReactNode } from "react";

import {
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import {
  WALLET_CATEGORY_KEYS,
  WALLET_UNIT_KEYS,
  WalletAdjustDialog,
  formatWalletAmount,
} from "@/features/billing/components/platform/org/wallet-adjust-dialog";
import { useOrgWallet } from "@/features/billing/hooks/use-platform-billing";
import { formatCount } from "@/features/billing/lib/money";
import type {
  BillingWalletEntry,
  BillingWalletSummary,
  BillingWalletUnit,
} from "@/features/billing/types";
import type { WalletCategory } from "@/features/platform-organizations/services/commerce.service";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const PAGE_SIZE = 20;

const AI_ACTION_LABEL_KEYS: Record<BillingWalletSummary["ai_action"], string> =
  {
    continue: "billing.wallet.ai_action.continue",
    stop_and_handoff: "billing.wallet.ai_action.stop_and_handoff",
  };

/**
 * Ledger reasons written by the billing engine are stored in English; known
 * ones are shown in the operator's language, free text as written.
 */
const SYSTEM_REASON_KEYS: Record<string, string> = {
  "Plan period AI allowance": "commerce.org.ledger_reason.plan_ai",
  "Plan period video minutes": "commerce.org.ledger_reason.plan_video",
  "Credit expiry": "commerce.org.ledger_reason.expiry",
  "Credit package purchase": "commerce.org.ledger_reason.package",
  "Purchased credits": "commerce.org.ledger_reason.package",
  "Invoice payment": "commerce.org.ledger_reason.invoice_payment",
  "Invoice overpayment": "commerce.org.ledger_reason.overpayment",
  "Payment refunded to wallet": "commerce.org.ledger_reason.refund_to_wallet",
  "Refund credit reservation": "commerce.org.ledger_reason.refund_reservation",
  "Refund refused: credits restored":
    "commerce.org.ledger_reason.refund_refused",
  "Referral reward": "commerce.org.ledger_reason.referral",
  "Trial extended": "commerce.org.ledger_reason.trial_extended",
  "AI usage": "commerce.org.ledger_reason.ai_usage",
  "Video call minutes": "commerce.org.ledger_reason.video_usage",
};

function isCategory(value: string | null | undefined): value is WalletCategory {
  return Boolean(value && value in WALLET_CATEGORY_KEYS);
}

function BalanceCard({
  icon,
  title,
  value,
  hint,
  tone = "default",
}: {
  icon: ReactNode;
  title: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: "default" | "warning";
}) {
  return (
    <div className="bg-card flex items-start gap-4 rounded-xl border p-5">
      <span
        className={cn(
          "flex size-10 shrink-0 items-center justify-center rounded-lg",
          tone === "warning"
            ? "bg-amber-500/15 text-amber-700 dark:text-amber-300"
            : "bg-primary/10 text-primary",
        )}
      >
        {icon}
      </span>
      <div className="min-w-0 space-y-1">
        <p className="text-muted-foreground text-sm">{title}</p>
        <div className="font-display text-2xl font-semibold tracking-tight tabular-nums">
          {value}
        </div>
        {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
      </div>
    </div>
  );
}

function SettingRow({
  icon,
  title,
  state,
  on,
  hint,
}: {
  icon: ReactNode;
  title: string;
  state: string;
  on: boolean;
  hint: string;
}) {
  return (
    <li className="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
      <span className="bg-muted text-muted-foreground mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg">
        {icon}
      </span>
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm font-medium">{title}</span>
          <Badge variant={on ? "success" : "outline"}>{state}</Badge>
        </div>
        <p className="text-muted-foreground text-xs">{hint}</p>
      </div>
    </li>
  );
}

function SettingsCard({ data }: { data: BillingWalletSummary }) {
  const { t, locale } = useLocale();
  const s = data.settings;
  return (
    <EntitySectionCard
      title={t("commerce.org.wallet.settings_title")}
      description={t("commerce.org.wallet.settings_description")}
    >
      <ul className="divide-y">
        <SettingRow
          icon={<RefreshCcw className="size-4" />}
          title={t("commerce.org.wallet.auto_topup")}
          on={s.auto_topup}
          state={
            s.auto_topup
              ? t("commerce.org.common.on")
              : t("commerce.org.common.off")
          }
          hint={
            s.auto_topup
              ? t("commerce.org.wallet.auto_topup_on", {
                  amount: formatCount(s.threshold, locale),
                })
              : s.auto_topup_available
                ? t("commerce.org.wallet.auto_topup_off")
                : t("commerce.org.wallet.auto_topup_unavailable")
          }
        />
        <SettingRow
          icon={<BellRing className="size-4" />}
          title={t("commerce.org.wallet.alert")}
          on={s.alert_threshold !== null}
          state={
            s.alert_threshold !== null
              ? t("commerce.org.wallet.alert_custom")
              : t("commerce.org.wallet.alert_default")
          }
          hint={
            s.alert_threshold !== null
              ? t("commerce.org.wallet.alert_on", {
                  amount: formatCount(s.alert_threshold, locale),
                })
              : t("commerce.org.wallet.alert_default_hint")
          }
        />
        <SettingRow
          icon={<Bot className="size-4" />}
          title={t("commerce.org.wallet.ai_action")}
          on={data.ai_action === "continue"}
          state={
            data.ai_action === "continue"
              ? t("commerce.org.wallet.ai_action_continue")
              : t("commerce.org.wallet.ai_action_stop")
          }
          hint={t(AI_ACTION_LABEL_KEYS[data.ai_action])}
        />
      </ul>
      <p className="text-muted-foreground mt-4 text-xs">
        {t("commerce.org.wallet.settings_owner_hint")}
      </p>
    </EntitySectionCard>
  );
}

/**
 * Organization page → "Cüzdan ve krediler" (CF-136, CF-143): TRY, AI credit
 * and video minute balances, the customer's auto top-up and low balance
 * alert (read-only), the ledger and, with `platform.subscriptions.write`,
 * manual credit / debit with a category.
 */
export function OrganizationWalletTab({
  organizationUuid,
}: {
  organizationUuid: string;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.billing.subscriptionsWrite);
  const listState = useServerListState({ initialPageSize: PAGE_SIZE });
  const { limit, offset } = listState.params;
  const [adding, setAdding] = useState(false);
  const wallet = useOrgWallet(organizationUuid, { limit, offset });
  const data = wallet.data;
  const lowAI =
    data !== undefined &&
    data.settings.alert_threshold !== null &&
    data.ai_credits <= data.settings.alert_threshold;

  const unitLabel = (unit: string | undefined) =>
    unit && unit in WALLET_UNIT_KEYS
      ? t(WALLET_UNIT_KEYS[unit as BillingWalletUnit])
      : (unit ?? "—");
  const reasonLabel = (reason: string | undefined) => {
    if (!reason) return "—";
    const key = SYSTEM_REASON_KEYS[reason];
    return key ? t(key) : reason;
  };

  const columns: ColumnDef<BillingWalletEntry, unknown>[] = [
    createColumn<BillingWalletEntry>({
      id: "created_at",
      meta: {
        headerClassName: "whitespace-nowrap",
        cellClassName: "whitespace-nowrap",
      },
      labelKey: "commerce.org.wallet.col_date",
      cell: ({ row: { original: row } }) =>
        row.created_at ? (
          <time
            dateTime={row.created_at}
            title={datetime(row.created_at, undefined, locale)}
          >
            {date(row.created_at, undefined, locale)}
          </time>
        ) : (
          "—"
        ),
    }),
    createColumn<BillingWalletEntry>({
      id: "reason",
      labelKey: "commerce.org.wallet.col_reason",
      cell: ({ row: { original: row } }) => (
        <span className="flex min-w-0 flex-col gap-1">
          <span className="line-clamp-2 break-words">
            {reasonLabel(row.reason)}
          </span>
          <span className="flex flex-wrap gap-1">
            <Badge variant="outline" className="font-normal">
              {unitLabel(row.unit)}
            </Badge>
            {isCategory(row.category) ? (
              <Badge variant="secondary" className="font-normal">
                {t(WALLET_CATEGORY_KEYS[row.category])}
              </Badge>
            ) : null}
          </span>
        </span>
      ),
    }),
    createColumn<BillingWalletEntry>({
      id: "expires_at",
      meta: {
        headerClassName: "whitespace-nowrap",
        cellClassName: "whitespace-nowrap",
      },
      labelKey: "commerce.org.wallet.col_expires",
      cell: ({ row: { original: row } }) =>
        row.expires_at ? date(row.expires_at, undefined, locale) : "—",
    }),
    createColumn<BillingWalletEntry>({
      id: "delta",
      labelKey: "commerce.org.wallet.col_amount",
      meta: {
        headerClassName: "text-end whitespace-nowrap",
        cellClassName: "text-end tabular-nums whitespace-nowrap",
      },
      cell: ({ row: { original: row } }) => {
        const delta = row.delta ?? 0;
        return (
          <span
            className={cn(
              "font-medium tabular-nums",
              delta < 0
                ? "text-destructive"
                : "text-emerald-700 dark:text-emerald-400",
            )}
          >
            {delta > 0 ? "+" : ""}
            {formatWalletAmount(row.unit ?? "", delta, locale)}
          </span>
        );
      },
    }),
  ];

  return (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-3">
        <BalanceCard
          icon={<Bot className="size-5" />}
          title={t("commerce.org.units.ai_credits")}
          tone={lowAI ? "warning" : "default"}
          value={
            data ? (
              formatCount(data.ai_credits, locale)
            ) : (
              <Skeleton className="h-8 w-24" />
            )
          }
          hint={lowAI ? t("commerce.org.wallet.low_balance") : undefined}
        />
        <BalanceCard
          icon={<Video className="size-5" />}
          title={t("commerce.org.units.video_minutes")}
          value={
            data ? (
              formatCount(data.video_minutes, locale)
            ) : (
              <Skeleton className="h-8 w-24" />
            )
          }
        />
        <BalanceCard
          icon={<Wallet className="size-5" />}
          title={t("commerce.org.units.try_balance")}
          value={
            data ? (
              formatWalletAmount("TRY", data.try_minor, locale)
            ) : (
              <Skeleton className="h-8 w-32" />
            )
          }
        />
      </div>
      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <EntitySectionCard
          title={t("commerce.org.wallet.ledger")}
          badge={data ? formatCount(data.entries.total, locale) : undefined}
          actions={
            canWrite ? (
              <Button
                type="button"
                size="sm"
                disabled={!data}
                onClick={() => setAdding(true)}
              >
                {t("commerce.org.wallet.adjust_title")}
              </Button>
            ) : null
          }
        >
          <EntityTable
            columns={columns}
            data={data?.entries.items ?? []}
            getRowId={(row) => row.uuid ?? row.idempotency_key ?? ""}
            isLoading={wallet.isLoading}
            isError={wallet.isError}
            onRetry={() => void wallet.refetch()}
            emptyTitle={t("billing.wallet.empty")}
            emptyDescription={t("billing.wallet.empty_hint")}
            total={data?.entries.total ?? 0}
            state={listState.tableState}
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
        {data ? (
          <SettingsCard data={data} />
        ) : (
          <Skeleton className="h-64 w-full rounded-xl" />
        )}
      </div>
      {adding && data ? (
        <WalletAdjustDialog
          organization={organizationUuid}
          balances={{
            TRY: data.try_minor,
            ai_credits: data.ai_credits,
            video_minutes: data.video_minutes,
          }}
          onClose={() => setAdding(false)}
        />
      ) : null}
    </div>
  );
}
