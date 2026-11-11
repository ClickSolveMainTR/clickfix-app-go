"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ClipboardList, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  requestFormsService,
  type RequestForm,
  type RequestFormInput,
} from "../services/request-forms.service";
import { CHANNELS, FormDialog } from "./form-dialog";
import { useDialogs } from "@/providers/dialog-provider";

/**
 * Dynamic request forms (CF-52): start forms per channel built from system
 * fields and the organization's custom fields, with conditional and
 * "required before resolving" fields. Uses the custom field permissions.
 */
export function RequestFormsPage() {
  const { confirmDelete } = useDialogs();
  const { t } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantCustomFieldsRead);
  const canWrite = perms.includes(Permission.TenantCustomFieldsManage);
  const key = ["tenant", org?.slug, "request-forms"] as const;
  const forms = useQuery({
    queryKey: key,
    queryFn: requestFormsService.list,
    enabled: Boolean(org) && canRead,
  });
  const [editing, setEditing] = useState<RequestForm | "new" | null>(null);
  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("request_forms.toast.failed"),
    );
  const save = useMutation({
    mutationFn: (body: RequestFormInput) =>
      editing && editing !== "new"
        ? requestFormsService.update(editing.uuid, body)
        : requestFormsService.create(body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("request_forms.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => requestFormsService.remove(uuid),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("request_forms.toast.deleted"));
    },
    onError,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<RequestForm>({
          accessorKey: "name",
          labelKey: "request_forms.columns.name",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{row.original.name}</span>
                {!row.original.is_active && (
                  <Badge variant="secondary">
                    {t("request_forms.inactive")}
                  </Badge>
                )}
              </div>
              {row.original.description && (
                <p className="text-muted-foreground text-sm whitespace-normal">
                  {row.original.description}
                </p>
              )}
            </div>
          ),
        }),
        createColumn<RequestForm>({
          id: "channels",
          accessorFn: (f) => f.channels.join(" "),
          labelKey: "request_forms.columns.channels",
          enableSorting: false,
          searchable: false,
          filterVariant: "faceted",
          filterOptions: CHANNELS.map((c) => ({
            value: c,
            labelKey: `request_forms.channel.${c}`,
            label: t(`request_forms.channel.${c}`),
          })),
          // A form serves several channels: match any of the picked ones.
          filterFn: (row, _id, value: string[] | undefined) =>
            !value?.length ||
            row.original.channels.some((c) => value.includes(c)),
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.channels.map((c) => (
                <Badge key={c} variant="outline">
                  {t(`request_forms.channel.${c}`)}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<RequestForm>({
          id: "fields",
          accessorFn: (f) => f.fields.length,
          labelKey: "request_forms.columns.fields",
          searchable: false,
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right tabular-nums",
          },
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {t("request_forms.field_count", {
                count: row.original.fields.length,
              })}
            </span>
          ),
        }),
        ...(canWrite
          ? [
              createColumn<RequestForm>({
                id: "actions",
                labelKey: "common.actions",
                enableHiding: false,
                enableSorting: false,
                meta: {
                  headerClassName: "text-right",
                  cellClassName: "text-right",
                },
                cell: ({ row }) => (
                  <div className="flex justify-end gap-1">
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("request_forms.edit")}
                      onClick={() => setEditing(row.original)}
                    >
                      <Pencil className="size-4" />
                    </Button>
                    <Button
                      disabled={remove.isPending}
                      size="icon"
                      variant="ghost"
                      aria-label={t("request_forms.delete")}
                      onClick={async () => {
                        if (
                          await confirmDelete({
                            title: t("request_forms.confirm_delete"),
                          })
                        )
                          remove.mutate(row.original.uuid);
                      }}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<RequestForm, unknown>[],
    [canWrite, confirmDelete, remove, t],
  );

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<ClipboardList className="size-7" />}
        title={t("request_forms.title")}
        description={t("request_forms.description")}
        actions={
          canWrite ? (
            <Button size="sm" onClick={() => setEditing("new")}>
              <Plus className="size-4" />
              {t("request_forms.new")}
            </Button>
          ) : undefined
        }
      />
      {org && !canRead && (
        <p className="text-muted-foreground text-sm">
          {t("request_forms.forbidden")}
        </p>
      )}
      {(!org || canRead) && (
        <EntityTable
          columns={columns}
          data={forms.data ?? []}
          getRowId={(f) => f.uuid}
          isLoading={forms.isLoading || !org}
          isError={forms.isError}
          errorTitle={t("request_forms.error.title")}
          errorDescription={t("request_forms.error.description")}
          onRetry={() => void forms.refetch()}
          emptyTitle={t("request_forms.empty")}
          emptyDescription=""
          // Every form comes in one response: search, the channel filter,
          // sort and pages in the browser.
          manual={{ filtering: false, sorting: false, pagination: false }}
          initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
          features={{
            rowSelection: false,
            viewMode: false,
          }}
        />
      )}
      {editing && (
        <FormDialog
          open
          onOpenChange={(o) => !o && setEditing(null)}
          form={editing === "new" ? null : editing}
          pending={save.isPending}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
    </div>
  );
}
