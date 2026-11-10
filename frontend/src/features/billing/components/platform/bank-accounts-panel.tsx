"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppForm, AppInput, AppSwitch, FormActions } from "@/components/forms";
import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { permissions } from "@/config/permissions";
import {
  useBankAccounts,
  useSaveBankAccount,
} from "@/features/billing/hooks/use-platform-billing";
import { formatIban, isValidTrIban } from "@/features/billing/lib/iban";
import type { BillingBankAccount } from "@/features/billing/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function createSchema(t: (key: string) => string) {
  return z.object({
    bank_name: z
      .string()
      .trim()
      .min(1, t("billing.validation.required"))
      .max(200),
    account_name: z
      .string()
      .trim()
      .min(1, t("billing.validation.required"))
      .max(200),
    iban: z.string().refine(isValidTrIban, t("billing.validation.iban")),
    branch: z.string().max(200),
    description: z.string().max(1000),
    active: z.boolean(),
  });
}
type Values = z.infer<ReturnType<typeof createSchema>>;

/** Seller bank accounts shown to customers for havale/EFT (TRY, TR IBAN). */
export function BankAccountsPanel() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.billing.subscriptionsWrite);
  const query = useBankAccounts();
  const save = useSaveBankAccount();
  const [editing, setEditing] = useState<BillingBankAccount | "new" | null>(
    null,
  );
  const schema = useMemo(() => createSchema(t), [t]);
  const current = editing && editing !== "new" ? editing : null;

  const columns = useMemo(
    () =>
      [
        createColumn<BillingBankAccount>({
          id: "bank",
          accessorKey: "bank_name",
          labelKey: "billing.bank.columns.bank",
          cell: ({ row: { original: row } }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.bank_name}</span>
              {row.branch ? (
                <span className="text-muted-foreground text-xs">
                  {row.branch}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<BillingBankAccount>({
          id: "holder",
          accessorKey: "account_name",
          labelKey: "billing.bank.columns.holder",
          cell: ({ row: { original: row } }) => row.account_name,
        }),
        createColumn<BillingBankAccount>({
          id: "iban",
          accessorKey: "iban",
          labelKey: "billing.bank.columns.iban",
          cell: ({ row: { original: row } }) => (
            <code className="text-xs tabular-nums">{formatIban(row.iban)}</code>
          ),
        }),
        createColumn<BillingBankAccount>({
          id: "status",
          accessorKey: "active",
          searchable: false,
          labelKey: "billing.bank.columns.status",
          cell: ({ row: { original: row } }) =>
            row.active ? (
              <Badge variant="success">{t("billing.bank.active")}</Badge>
            ) : (
              <Badge variant="outline">{t("billing.bank.inactive")}</Badge>
            ),
        }),
        createColumn<BillingBankAccount>({
          id: "actions",
          labelKey: "billing.common.actions",
          enableHiding: false,
          enableSorting: false,
          meta: { headerClassName: "text-end", cellClassName: "text-end" },
          cell: ({ row: { original: row } }) =>
            canWrite ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-8"
                onClick={() => setEditing(row)}
              >
                <Pencil className="size-3.5" />
                {t("billing.common.edit")}
              </Button>
            ) : null,
        }),
      ] as ColumnDef<BillingBankAccount, unknown>[],
    [canWrite, t],
  );

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          {t("billing.bank.hint")}
        </p>
        {canWrite ? (
          <Button type="button" size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("billing.bank.add")}
          </Button>
        ) : null}
      </div>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("billing.bank.empty")}
        emptyDescription={t("billing.bank.empty_hint")}
        // The list is short and unpaged: search, sort and pages in the browser.
        manual={{ filtering: false, sorting: false, pagination: false }}
        initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
        features={{
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => !open && setEditing(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {current ? t("billing.bank.edit_title") : t("billing.bank.add")}
            </DialogTitle>
            <DialogDescription>{t("billing.bank.form_hint")}</DialogDescription>
          </DialogHeader>
          <AppForm<Values>
            key={current?.uuid ?? "new"}
            className="space-y-4"
            schema={schema}
            defaultValues={{
              bank_name: current?.bank_name ?? "",
              account_name: current?.account_name ?? "",
              iban: current ? formatIban(current.iban) : "TR",
              branch: current?.branch ?? "",
              description: current?.description ?? "",
              active: current?.active ?? true,
            }}
            onSubmit={async (values) => {
              await save.mutateAsync({
                uuid: current?.uuid,
                bank_name: values.bank_name.trim(),
                account_name: values.account_name.trim(),
                iban: values.iban.replace(/\s/g, "").toUpperCase(),
                branch: values.branch.trim(),
                description: values.description.trim(),
                active: values.active,
              });
              setEditing(null);
            }}
          >
            {(form) => (
              <>
                <div className="grid gap-4 sm:grid-cols-2">
                  <AppInput
                    name="bank_name"
                    label={t("billing.bank.fields.bank_name")}
                    autoComplete="off"
                  />
                  <AppInput
                    name="branch"
                    label={t("billing.bank.fields.branch")}
                  />
                </div>
                <AppInput
                  name="account_name"
                  label={t("billing.bank.fields.account_name")}
                />
                <AppInput
                  name="iban"
                  label="IBAN"
                  inputMode="text"
                  autoComplete="off"
                  spellCheck={false}
                  className="font-mono"
                />
                <AppInput
                  name="description"
                  label={t("billing.bank.fields.description")}
                />
                <AppSwitch name="active" label={t("billing.bank.active")} />
                <FormActions>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setEditing(null)}
                  >
                    {t("common.cancel")}
                  </Button>
                  <Button type="submit" disabled={form.formState.isSubmitting}>
                    {t("billing.common.save")}
                  </Button>
                </FormActions>
              </>
            )}
          </AppForm>
        </DialogContent>
      </Dialog>
    </div>
  );
}
