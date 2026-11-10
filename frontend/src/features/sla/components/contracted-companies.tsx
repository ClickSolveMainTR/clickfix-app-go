"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Building2, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { contactsService } from "@/features/contacts/services/contacts.service";
import { isApiError } from "@/lib/api";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  slaService,
  type SlaContractedCompany,
  type SlaPolicy,
} from "../services/sla.service";

/**
 * B2B contracts (CF-46): a company's contracted SLA policy applies to its
 * conversations before any condition match.
 */
export function ContractedCompanies({
  orgSlug,
  policies,
  canWrite,
}: {
  orgSlug: string;
  policies: SlaPolicy[];
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { confirmDelete } = useDialogs();
  const qc = useQueryClient();
  const key = ["tenant", orgSlug, "sla", "companies"] as const;
  const list = useQuery({
    queryKey: key,
    queryFn: slaService.contractedCompanies,
  });
  const [company, setCompany] = useState("");
  const [policy, setPolicy] = useState("");

  const set = useMutation({
    mutationFn: (p: { company: string; policy: string | null }) =>
      slaService.setCompanyPolicy(p.company, p.policy),
    onSuccess: async () => {
      setCompany("");
      await Promise.all([
        qc.invalidateQueries({ queryKey: key }),
        qc.invalidateQueries({
          queryKey: ["tenant", orgSlug, "sla", "policies"],
        }),
      ]);
      appToast.success(t("sla.toast.saved"));
    },
    onError: (error: unknown) =>
      appToast.error(isApiError(error) ? error.message : t("sla.toast.failed")),
  });

  const searchCompanies = async (q: string): Promise<ComboboxOption[]> => {
    const page = await contactsService.companies({ q, limit: 20, offset: 0 });
    return page.items.map((c) => ({
      value: c.uuid,
      label: c.name,
      description: c.tier || c.domain || undefined,
    }));
  };

  const columns = useMemo(
    () =>
      [
        createColumn<SlaContractedCompany>({
          accessorKey: "name",
          labelKey: "sla.contracts.columns.company",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="block max-w-[18rem] truncate">
              {row.original.name}
              {row.original.tier && (
                <span className="text-muted-foreground ml-2 text-xs">
                  {row.original.tier}
                </span>
              )}
            </span>
          ),
        }),
        createColumn<SlaContractedCompany>({
          id: "policy",
          accessorFn: (c) => c.policy.uuid,
          labelKey: "sla.contracts.columns.policy",
          searchable: false,
          filterVariant: "faceted",
          filterOptions: policies.map((p) => ({
            value: p.uuid,
            label: p.name,
          })),
          sortingFn: (a, b) =>
            a.original.policy.name.localeCompare(b.original.policy.name),
          cell: ({ row }) => (
            <span className="text-muted-foreground block truncate text-xs">
              {row.original.policy.name}
            </span>
          ),
        }),
        ...(canWrite
          ? [
              createColumn<SlaContractedCompany>({
                id: "actions",
                labelKey: "common.actions",
                enableHiding: false,
                enableSorting: false,
                meta: {
                  headerClassName: "text-right",
                  cellClassName: "text-right",
                },
                cell: ({ row }) => (
                  <Button
                    disabled={set.isPending}
                    size="icon"
                    variant="ghost"
                    aria-label={t("sla.contracts.remove")}
                    onClick={async () => {
                      if (
                        await confirmDelete({
                          title: t("sla.contracts.confirm_remove", {
                            name: row.original.name,
                          }),
                          description: t("sla.contracts.confirm_remove_hint"),
                          confirmLabel: t("sla.contracts.remove"),
                        })
                      )
                        set.mutate({
                          company: row.original.company_uuid,
                          policy: null,
                        });
                    }}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<SlaContractedCompany, unknown>[],
    [canWrite, confirmDelete, policies, set, t],
  );

  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <Building2 className="size-4" />
            {t("sla.contracts.title")}
          </h3>
          <p className="text-muted-foreground text-xs">
            {t("sla.contracts.hint")}
          </p>
        </div>
        {canWrite && policies.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            <AsyncCombobox
              className="min-w-56 flex-1"
              value={company}
              onValueChange={setCompany}
              loadOptions={searchCompanies}
              placeholder={t("sla.contracts.company")}
              searchPlaceholder={t("sla.contracts.search")}
              emptyText={t("sla.contracts.no_company")}
            />
            <Select value={policy} onValueChange={setPolicy}>
              <SelectTrigger className="w-56">
                <SelectValue placeholder={t("sla.contracts.policy")} />
              </SelectTrigger>
              <SelectContent>
                {policies.map((p) => (
                  <SelectItem key={p.uuid} value={p.uuid}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              size="sm"
              disabled={!company || !policy || set.isPending}
              onClick={() => set.mutate({ company, policy })}
            >
              {t("sla.contracts.assign")}
            </Button>
          </div>
        )}
        <EntityTable
          columns={columns}
          data={list.data ?? []}
          getRowId={(c) => c.company_uuid}
          isLoading={list.isLoading}
          isError={list.isError}
          onRetry={() => void list.refetch()}
          emptyTitle={t("sla.contracts.empty")}
          emptyDescription=""
          searchPlaceholder={t("sla.contracts.filter")}
          // Every contract comes in one response: search, the policy filter,
          // sort and pages in the browser.
          manual={{ filtering: false, sorting: false, pagination: false }}
          initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
          features={{
            rowSelection: false,
            viewMode: false,
            density: false,
          }}
        />
      </CardContent>
    </Card>
  );
}
