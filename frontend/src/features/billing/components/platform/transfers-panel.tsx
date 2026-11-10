"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, Check, Download, X } from "lucide-react";
import { useMemo, useState } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { permissions } from "@/config/permissions";
import { BillingStatus } from "@/features/billing/components/billing-status";
import {
  useReviewTransfer,
  useTransferQueue,
} from "@/features/billing/hooks/use-platform-billing";
import { formatMinor } from "@/features/billing/lib/money";
import { platformBillingService } from "@/features/billing/services/platform-billing.service";
import type { BillingAdminTransfer } from "@/features/billing/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const STATUSES = ["reported", "approved", "rejected", "all"] as const;

function maskIban(iban: string) {
  return iban.length > 8 ? `${iban.slice(0, 4)} •••• ${iban.slice(-4)}` : iban;
}

/**
 * Havale/EFT approval queue: reported transfers first, with invoice total vs
 * reported amount, receipt download and approve / reject (reason required).
 * Approval settles the invoice atomically on the server.
 */
export function TransfersPanel() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.billing.subscriptionsWrite);
  const [status, setStatus] = useState<(typeof STATUSES)[number]>("reported");
  const listState = useServerListState({ initialPageSize: 20 });
  const { limit, offset } = listState.params;
  const query = useTransferQueue({
    limit,
    offset,
    status: status === "all" ? undefined : status,
  });
  const review = useReviewTransfer();
  const [rejecting, setRejecting] = useState<BillingAdminTransfer | null>(null);
  const [reason, setReason] = useState("");
  const [approving, setApproving] = useState<BillingAdminTransfer | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<BillingAdminTransfer>({
          id: "org",
          labelKey: "billing.transfers.columns.organization",
          cell: ({ row: { original: row } }) => (
            <span className="flex min-w-0 flex-col">
              <span className="truncate font-medium">
                {row.organization_name}
              </span>
              <span className="text-muted-foreground truncate text-xs">
                {datetime(row.created_at, undefined, locale)}
              </span>
            </span>
          ),
        }),
        createColumn<BillingAdminTransfer>({
          id: "reference",
          labelKey: "billing.transfers.columns.reference",
          cell: ({ row: { original: row } }) => (
            <code className="text-xs font-medium">{row.invoice_reference}</code>
          ),
        }),
        createColumn<BillingAdminTransfer>({
          id: "bank",
          labelKey: "billing.transfers.columns.bank",
          cell: ({ row: { original: row } }) => (
            <span className="flex flex-col">
              <span>{row.bank_name}</span>
              <span className="text-muted-foreground text-xs tabular-nums">
                {maskIban(row.iban)}
              </span>
            </span>
          ),
        }),
        createColumn<BillingAdminTransfer>({
          id: "amount",
          labelKey: "billing.transfers.columns.amount",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row: { original: row } }) => {
            const mismatch = row.amount_minor !== row.invoice_total_minor;
            return (
              <span className="inline-flex flex-col items-end">
                <span className="inline-flex items-center gap-1 font-medium">
                  {mismatch ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <AlertTriangle
                          className="size-3.5 text-amber-600"
                          aria-label={t("billing.transfers.mismatch")}
                        />
                      </TooltipTrigger>
                      <TooltipContent>
                        {t("billing.transfers.mismatch")}
                      </TooltipContent>
                    </Tooltip>
                  ) : null}
                  {formatMinor(row.amount_minor, "TRY", locale)}
                </span>
                <span className="text-muted-foreground text-xs">
                  {t("billing.transfers.invoice_total", {
                    amount: formatMinor(row.invoice_total_minor, "TRY", locale),
                  })}
                </span>
              </span>
            );
          },
        }),
        createColumn<BillingAdminTransfer>({
          id: "status",
          labelKey: "billing.transfers.columns.status",
          cell: ({ row: { original: row } }) => (
            <span className="flex flex-col items-start gap-0.5">
              <BillingStatus kind="transfer" status={row.status} />
              {row.rejection_reason ? (
                <span className="text-muted-foreground max-w-48 truncate text-xs">
                  {row.rejection_reason}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<BillingAdminTransfer>({
          id: "actions",
          labelKey: "billing.common.actions",
          enableHiding: false,
          enableSorting: false,
          meta: { headerClassName: "text-end", cellClassName: "text-end" },
          cell: ({ row: { original: row } }) => (
            <span className="inline-flex items-center gap-1">
              {row.has_receipt ? (
                <Button asChild variant="ghost" size="sm" className="h-8">
                  <a
                    href={platformBillingService.receiptUrl(
                      row.organization_uuid,
                      row.uuid,
                    )}
                    download
                  >
                    <Download className="size-3.5" />
                    {t("billing.transfers.receipt")}
                  </a>
                </Button>
              ) : (
                <span className="text-muted-foreground px-2 text-xs">
                  {t("billing.transfers.no_receipt")}
                </span>
              )}
              {canWrite && row.status === "reported" ? (
                <>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="h-8"
                    onClick={() => {
                      setReason("");
                      setRejecting(row);
                    }}
                  >
                    <X className="size-3.5" />
                    {t("billing.transfers.reject")}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    className="h-8"
                    onClick={() => setApproving(row)}
                  >
                    <Check className="size-3.5" />
                    {t("billing.transfers.approve")}
                  </Button>
                </>
              ) : null}
            </span>
          ),
        }),
      ] as ColumnDef<BillingAdminTransfer, unknown>[],
    [canWrite, locale, t],
  );

  return (
    <div className="space-y-3">
      <p className="text-muted-foreground text-sm">
        {t("billing.transfers.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={
          status === "reported"
            ? t("billing.transfers.empty_pending")
            : t("billing.transfers.empty")
        }
        emptyDescription=""
        total={query.data?.total ?? 0}
        state={listState.tableState}
        features={{
          // The endpoint takes `status`, `limit` and `offset` only.
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
        toolbarExtra={
          <Select
            value={status}
            onValueChange={(v) => {
              setStatus(v as (typeof STATUSES)[number]);
              listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
            }}
          >
            <SelectTrigger
              className="h-8 w-44"
              aria-label={t("billing.transfers.filter")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STATUSES.map((value) => (
                <SelectItem key={value} value={value}>
                  {value === "all"
                    ? t("billing.common.all")
                    : t(`billing.status.transfer.${value}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      <Dialog
        open={Boolean(approving)}
        onOpenChange={(open) => !open && setApproving(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("billing.transfers.approve_title")}</DialogTitle>
            <DialogDescription>
              {approving
                ? t("billing.transfers.approve_description", {
                    organization: approving.organization_name,
                    amount: formatMinor(approving.amount_minor, "TRY", locale),
                    reference: approving.invoice_reference,
                  })
                : null}
            </DialogDescription>
          </DialogHeader>
          {approving &&
          approving.amount_minor !== approving.invoice_total_minor ? (
            <p className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm">
              <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" />
              {t("billing.transfers.mismatch_warning", {
                amount: formatMinor(
                  approving.invoice_total_minor,
                  "TRY",
                  locale,
                ),
              })}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setApproving(null)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              disabled={review.isPending}
              onClick={() => {
                if (!approving) return;
                review.mutate(
                  { transfer: approving, approve: true, reason: "" },
                  { onSettled: () => setApproving(null) },
                );
              }}
            >
              {t("billing.transfers.approve")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(rejecting)}
        onOpenChange={(open) => !open && setRejecting(null)}
      >
        <DialogContent>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (!rejecting || !reason.trim()) return;
              review.mutate(
                { transfer: rejecting, approve: false, reason: reason.trim() },
                { onSettled: () => setRejecting(null) },
              );
            }}
          >
            <DialogHeader>
              <DialogTitle>{t("billing.transfers.reject_title")}</DialogTitle>
              <DialogDescription>
                {t("billing.transfers.reject_description")}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-2">
              <Label htmlFor="transfer-reject-reason">
                {t("billing.transfers.reject_reason")}
              </Label>
              <Textarea
                id="transfer-reject-reason"
                value={reason}
                maxLength={1000}
                required
                onChange={(e) => setReason(e.target.value)}
                placeholder={t("billing.transfers.reject_placeholder")}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setRejecting(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                variant="destructive"
                disabled={!reason.trim() || review.isPending}
              >
                {t("billing.transfers.reject")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
