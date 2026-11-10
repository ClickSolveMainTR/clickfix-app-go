"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  usePlanSubscribers,
  useSchedulePlanMigration,
} from "@/features/billing/hooks/use-credit-products";
import { formatCount, formatMinor } from "@/features/billing/lib/money";
import type {
  BillingAdminSubscription,
  BillingPlan,
  BillingVersion,
} from "@/features/billing/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const MAX = 100;

type Candidate = {
  row: BillingAdminSubscription;
  /** Why it cannot move; null when selectable. */
  blocked: "cancel" | "price" | null;
};

/**
 * "Aboneleri taşı": schedule subscribers of a plan's older versions onto a
 * published version. Each move happens at that subscriber's next renewal, on
 * the same billing period and currency; at most 100 per batch.
 */
export function MigrateSubscribersDialog({
  open,
  onClose,
  plan,
  target,
}: {
  open: boolean;
  onClose: () => void;
  plan: BillingPlan;
  target: BillingVersion;
}) {
  const { t, locale } = useLocale();
  const subscribers = usePlanSubscribers(plan.uuid, open);
  const schedule = useSchedulePlanMigration(plan.uuid);
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [reason, setReason] = useState("");

  const candidates = useMemo<Candidate[]>(() => {
    const offered = new Set(
      target.prices.map((p) => `${p.interval}:${p.currency}`),
    );
    return (subscribers.data?.items ?? [])
      .filter((row) => row.version_uuid !== target.uuid)
      .map((row) => ({
        row,
        blocked: row.cancel_at_period_end
          ? ("cancel" as const)
          : offered.has(`${row.price_interval}:${row.price_currency}`)
            ? null
            : ("price" as const),
      }))
      .sort(
        (a, b) =>
          Number(Boolean(a.blocked)) - Number(Boolean(b.blocked)) ||
          a.row.organization_name.localeCompare(b.row.organization_name),
      );
  }, [subscribers.data, target]);

  // Rows filtered out by the search stay selected, as before.
  const selected = useMemo(
    () => candidates.filter((c) => selection[c.row.uuid]),
    [candidates, selection],
  );

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<Candidate>(),
        createColumn<Candidate>({
          id: "organization",
          accessorFn: (c) => c.row.organization_name,
          labelKey: "credit_products.migrate.columns.organization",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row: { original: c } }) => (
            <span className="flex flex-wrap items-center gap-2 font-medium">
              <span className="truncate">{c.row.organization_name}</span>
              <Badge variant="secondary" className="font-normal">
                {t("credit_products.migrate.on_version", {
                  version: c.row.version_number,
                })}
              </Badge>
            </span>
          ),
        }),
        createColumn<Candidate>({
          id: "price",
          accessorFn: (c) => c.row.price_amount_minor,
          labelKey: "credit_products.migrate.columns.price",
          searchable: false,
          meta: { cellClassName: "text-xs tabular-nums whitespace-nowrap" },
          cell: ({ row: { original: c } }) => (
            <>
              {t(`billing.interval.${c.row.price_interval}`)} ·{" "}
              {formatMinor(
                c.row.price_amount_minor,
                c.row.price_currency,
                locale,
              )}
            </>
          ),
        }),
        createColumn<Candidate>({
          id: "renews",
          accessorFn: (c) => c.row.period_end,
          labelKey: "credit_products.migrate.columns.renews",
          searchable: false,
          meta: { cellClassName: "text-xs tabular-nums whitespace-nowrap" },
          cell: ({ row: { original: c } }) =>
            date(c.row.period_end, undefined, locale),
        }),
        createColumn<Candidate>({
          id: "blocked",
          accessorFn: (c) => c.blocked ?? "",
          labelKey: "credit_products.migrate.columns.status",
          searchable: false,
          cell: ({ row: { original: c } }) =>
            c.blocked ? (
              <span className="text-xs text-amber-700 dark:text-amber-400">
                {t(`credit_products.migrate.blocked_${c.blocked}`)}
              </span>
            ) : null,
        }),
      ] as ColumnDef<Candidate, unknown>[],
    [locale, t],
  );

  const close = () => {
    if (schedule.isPending) return;
    setSelection({});
    setReason("");
    onClose();
  };

  const submit = async () => {
    const rows = selected;
    await schedule.mutateAsync({
      version_uuid: target.uuid,
      subscription_uuids: rows.map((c) => c.row.uuid),
      organization_uuids: rows.map((c) => c.row.organization_uuid),
      reason: reason.trim(),
    });
    close();
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !v && close()}>
      <DialogContent className="flex max-h-[92dvh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("credit_products.migrate.title", { version: target.version })}
          </DialogTitle>
          <DialogDescription>
            {t("credit_products.migrate.description")}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto">
          <EntityTable
            columns={columns}
            data={candidates}
            getRowId={(c) => c.row.uuid}
            getRowCanSelect={(c) => !c.blocked}
            maxSelection={MAX}
            isLoading={subscribers.isLoading}
            isError={subscribers.isError}
            onRetry={() => void subscribers.refetch()}
            emptyTitle={t("credit_products.migrate.empty")}
            emptyDescription={t("credit_products.migrate.empty_hint")}
            searchPlaceholder={t("credit_products.migrate.search")}
            state={{
              rowSelection: selection,
              onRowSelectionChange: setSelection,
            }}
            manual={{ filtering: false, sorting: false, pagination: false }}
            toolbarExtra={
              <p className="text-muted-foreground text-xs tabular-nums">
                {t("credit_products.migrate.selected", {
                  count: formatCount(selected.length, locale),
                  max: MAX,
                })}
              </p>
            }
            features={{
              // One page: "select all" takes every row the search shows (up to
              // MAX), as before. The list is capped by the server (`truncated`).
              pagination: false,
              columnFilters: false,
              viewMode: false,
              density: false,
              columnVisibility: false,
              columnResizing: false,
              columnPinning: false,
              columnOrdering: false,
            }}
          />
          {subscribers.data?.truncated ? (
            <p className="text-muted-foreground text-xs">
              {t("credit_products.migrate.truncated")}
            </p>
          ) : null}

          <div className="space-y-1.5">
            <Label htmlFor="migrate-reason">
              {t("credit_products.migrate.reason")}
            </Label>
            <Textarea
              id="migrate-reason"
              rows={2}
              maxLength={500}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("credit_products.migrate.reason_placeholder")}
            />
            <p className="text-muted-foreground text-xs">
              {t("credit_products.migrate.reason_hint")}
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={close}
            disabled={schedule.isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={
              selected.length === 0 || !reason.trim() || schedule.isPending
            }
            onClick={() => void submit()}
          >
            {t("credit_products.migrate.submit", {
              count: formatCount(selected.length, locale),
            })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
