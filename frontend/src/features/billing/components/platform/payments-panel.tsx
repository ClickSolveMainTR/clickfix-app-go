"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Undo2 } from "lucide-react";
import Link from "next/link";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { PaymentProviderLabel } from "@/features/billing/components/payment-provider-label";
import { BillingStatus } from "@/features/billing/components/billing-status";
import {
  useAdminPayments,
  useRefundPayment,
} from "@/features/billing/hooks/use-platform-billing";
import {
  formatMinor,
  minorToInput,
  parseMajorToMinor,
} from "@/features/billing/lib/money";
import type { BillingAdminPayment } from "@/features/billing/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const STATUSES = ["failed", "unknown", "pending", "succeeded", "all"] as const;

function RefundDialog({
  payment,
  onClose,
}: {
  payment: BillingAdminPayment | null;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const refund = useRefundPayment();
  const [mode, setMode] = useState<"full" | "partial" | "cancel">("full");
  const [amount, setAmount] = useState("");
  const parsed = parseMajorToMinor(amount);
  const max = payment?.amount_minor ?? 0;
  const partialInvalid =
    mode === "partial" && (parsed === null || parsed <= 0 || parsed > max);

  return (
    <Dialog open={Boolean(payment)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!payment || partialInvalid) return;
            refund.mutate(
              {
                organization: payment.organization_uuid,
                payment: payment.uuid,
                cancel: mode === "cancel",
                amountMinor:
                  mode === "partial" && parsed ? parsed : payment.amount_minor,
              },
              { onSuccess: onClose },
            );
          }}
        >
          <DialogHeader>
            <DialogTitle>{t("billing.payments.refund_title")}</DialogTitle>
            <DialogDescription>
              {payment
                ? t("billing.payments.refund_description", {
                    organization: payment.organization_name,
                    amount: formatMinor(payment.amount_minor, "TRY", locale),
                  })
                : null}
            </DialogDescription>
          </DialogHeader>
          <RadioGroup
            value={mode}
            onValueChange={(v) => {
              setMode(v as typeof mode);
              if (v === "partial" && payment && !amount)
                setAmount(minorToInput(payment.amount_minor, locale));
            }}
            className="gap-3"
          >
            {(payment?.provider === "iyzico"
              ? (["full", "partial", "cancel"] as const)
              : (["full", "partial"] as const)
            ).map((value) => (
              <Label
                key={value}
                className="flex cursor-pointer items-start gap-3 rounded-md border p-3 font-normal"
              >
                <RadioGroupItem value={value} className="mt-0.5" />
                <span className="space-y-0.5">
                  <span className="block font-medium">
                    {t(`billing.payments.refund_${value}`)}
                  </span>
                  <span className="text-muted-foreground block text-xs">
                    {t(`billing.payments.refund_${value}_hint`)}
                  </span>
                </span>
              </Label>
            ))}
          </RadioGroup>
          {mode === "partial" ? (
            <div className="space-y-2">
              <Label htmlFor="refund-amount">
                {t("billing.payments.refund_amount")}
              </Label>
              <Input
                id="refund-amount"
                inputMode="decimal"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                aria-invalid={partialInvalid}
              />
              {partialInvalid ? (
                <p className="text-destructive text-xs">
                  {t("billing.payments.refund_amount_invalid", {
                    max: formatMinor(max, "TRY", locale),
                  })}
                </p>
              ) : null}
            </div>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={refund.isPending || partialInvalid}
            >
              {t("billing.payments.refund")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Payment attempts across organizations; failed ones by default. */
export function PaymentsPanel() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.billing.subscriptionsWrite);
  const [status, setStatus] = useState<(typeof STATUSES)[number]>("failed");
  const listState = useServerListState({ initialPageSize: 20 });
  const { limit, offset } = listState.params;
  const [refunding, setRefunding] = useState<BillingAdminPayment | null>(null);
  const query = useAdminPayments({
    limit,
    offset,
    status: status === "all" ? undefined : status,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<BillingAdminPayment>({
          id: "org",
          accessorKey: "organization_name",
          labelKey: "billing.payments.columns.organization",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={`${routes.platform.organizations.detail(row.original.organization_uuid)}?tab=billing`}
              className="font-medium hover:underline"
            >
              {row.original.organization_name}
            </Link>
          ),
        }),
        createColumn<BillingAdminPayment>({
          id: "reference",
          accessorKey: "invoice_reference",
          labelKey: "billing.payments.columns.reference",
          cell: ({ row }) => (
            <code className="text-xs">{row.original.invoice_reference}</code>
          ),
        }),
        createColumn<BillingAdminPayment>({
          id: "provider",
          accessorKey: "provider",
          labelKey: "billing.payments.columns.provider",
          cell: ({ row }) => (
            <span className="flex flex-col">
              <PaymentProviderLabel
                provider={row.original.provider}
                className="self-start"
              />
              <span className="text-muted-foreground text-xs">
                {t(`billing.payments.kind.${row.original.kind}`)}
              </span>
            </span>
          ),
        }),
        createColumn<BillingAdminPayment>({
          id: "amount",
          accessorKey: "amount_minor",
          labelKey: "billing.payments.columns.amount",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) => (
            <span
              className={
                row.original.kind === "refund" ? "text-muted-foreground" : ""
              }
            >
              {row.original.kind === "refund" ? "−" : ""}
              {formatMinor(
                row.original.amount_minor,
                row.original.currency,
                locale,
              )}
            </span>
          ),
        }),
        createColumn<BillingAdminPayment>({
          id: "status",
          accessorKey: "status",
          labelKey: "billing.payments.columns.status",
          cell: ({ row }) => (
            <span className="flex flex-col items-start gap-0.5">
              <BillingStatus kind="payment" status={row.original.status} />
              {row.original.failure_code ? (
                <code className="text-muted-foreground text-xs">
                  {row.original.failure_code}
                </code>
              ) : null}
            </span>
          ),
        }),
        createColumn<BillingAdminPayment>({
          id: "date",
          accessorFn: (row) => row.completed_at ?? row.created_at,
          labelKey: "billing.payments.columns.date",
          cell: ({ row }) => (
            <span className="text-muted-foreground tabular-nums">
              {datetime(
                row.original.completed_at ?? row.original.created_at,
                undefined,
                locale,
              )}
            </span>
          ),
        }),
        createColumn<BillingAdminPayment>({
          id: "actions",
          labelKey: "billing.common.actions",
          enableHiding: false,
          enableSorting: false,
          meta: { headerClassName: "text-end", cellClassName: "text-end" },
          cell: ({ row }) =>
            canWrite &&
            row.original.status === "succeeded" &&
            row.original.kind !== "refund" &&
            row.original.provider !== "bank_transfer" ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-8"
                onClick={() => setRefunding(row.original)}
              >
                <Undo2 className="size-3.5" />
                {t("billing.payments.refund")}
              </Button>
            ) : null,
        }),
      ] as ColumnDef<BillingAdminPayment, unknown>[],
    [canWrite, locale, t],
  );

  return (
    <div className="space-y-3">
      <p className="text-muted-foreground text-sm">
        {t("billing.payments.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={
          status === "failed"
            ? t("billing.payments.empty_failed")
            : t("billing.payments.empty")
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
              aria-label={t("billing.payments.filter")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STATUSES.map((value) => (
                <SelectItem key={value} value={value}>
                  {value === "all"
                    ? t("billing.common.all")
                    : t(`billing.status.payment.${value}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
      <RefundDialog payment={refunding} onClose={() => setRefunding(null)} />
    </div>
  );
}
