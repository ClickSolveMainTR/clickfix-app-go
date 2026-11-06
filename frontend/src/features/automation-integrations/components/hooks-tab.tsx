"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import {
  EntityRowActions,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  automationIntegrationsService,
  type HookSubscription,
} from "../services/automation-integrations.service";
import { useIntegrationsError } from "./api-keys-tab";

/**
 * REST hook subscriptions of the organization. Zapier, Make and n8n create
 * them through `POST /subscriptions`; removing one here stops deliveries.
 */
export function HooksTab({
  slug,
  canDelete,
}: {
  slug: string;
  canDelete: boolean;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useIntegrationsError();
  const key = ["tenant", slug, "webhooks"] as const;
  const listState = useServerListState({ initialPageSize: 50 });
  const { limit, offset } = listState.params;
  const hooks = useQuery({
    // "page" keeps this apart from the unpaged list other screens cache
    // under the same prefix.
    queryKey: [...key, "page", limit, offset],
    queryFn: () => automationIntegrationsService.hooksPage(slug, limit, offset),
    placeholderData: keepPreviousData,
  });
  const [deleting, setDeleting] = useState<HookSubscription | null>(null);
  const remove = useMutation({
    mutationFn: (uuid: string) =>
      automationIntegrationsService.removeHook(slug, uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("automation_integrations.hooks.deleted_toast"));
    },
    onError,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<HookSubscription>({
          accessorKey: "name",
          labelKey: "automation_integrations.hooks.columns.name",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="font-medium">{row.original.name}</p>
              <p className="text-muted-foreground max-w-xs truncate font-mono text-xs font-normal">
                {row.original.url}
              </p>
            </div>
          ),
        }),
        createColumn<HookSubscription>({
          accessorKey: "enabled",
          labelKey: "automation_integrations.keys.columns.status",
          cell: ({ row }) =>
            row.original.enabled ? (
              <StatusChip
                label={t("automation_integrations.hooks.enabled")}
                tone="success"
              />
            ) : (
              <StatusChip label={t("automation_integrations.hooks.disabled")} />
            ),
        }),
        createColumn<HookSubscription>({
          id: "events",
          accessorFn: (row) => row.events.join(" "),
          labelKey: "automation_integrations.hooks.columns.events",
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.events.map((e) => (
                <Badge key={e} variant="outline" className="font-mono">
                  {e}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<HookSubscription>({
          accessorKey: "created_at",
          labelKey: "automation_integrations.hooks.columns.created",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {relativeDatetime(row.original.created_at, locale)}
            </span>
          ),
        }),
        ...(canDelete
          ? [
              createColumn<HookSubscription>({
                id: "actions",
                labelKey: "common.actions",
                enableHiding: false,
                meta: { headerClassName: "text-right" },
                cell: ({ row }) => (
                  <EntityRowActions
                    actions={[
                      {
                        id: "delete",
                        label: t("automation_integrations.hooks.delete"),
                        icon: Trash2,
                        variant: "destructive",
                        onSelect: () => setDeleting(row.original),
                      },
                    ]}
                  />
                ),
              }),
            ]
          : []),
      ] as ColumnDef<HookSubscription, unknown>[],
    [canDelete, locale, t],
  );

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("automation_integrations.hooks.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={hooks.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={hooks.isLoading}
        isError={hooks.isError}
        errorTitle={t("automation_integrations.error.title")}
        errorDescription={t("automation_integrations.error.description")}
        onRetry={() => void hooks.refetch()}
        emptyTitle={t("automation_integrations.hooks.empty_title")}
        emptyDescription={t("automation_integrations.hooks.empty_description")}
        total={hooks.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: "tenant-webhooks-v1",
          // The endpoint takes `limit` and `offset` only.
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
        }}
      />
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("automation_integrations.hooks.delete_title", {
          name: deleting?.name ?? "",
        })}
        description={t("automation_integrations.hooks.delete_description")}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}
