"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ListChecks, Pencil, Trash2 } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { datetime } from "@/lib/utils";
import { useDialogs } from "@/providers/dialog-provider";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

import { useDeleteRecord } from "../../hooks/use-custom-field-mutations";
import {
  useCustomFieldDefinitions,
  useCustomFieldMeta,
  useCustomObjectRecords,
  useCustomObjectType,
} from "../../hooks/use-custom-fields";
import {
  buildFilterParams,
  CF_PREFIX,
  type CustomFieldFilterRow,
} from "../../lib/filters";
import type { CustomObjectRecord } from "../../types";
import { CustomFieldFilterBuilder } from "../custom-field-filter-builder";
import { CustomFieldValue } from "../custom-field-value";
import { objectFieldsTab } from "../settings/custom-fields-settings-page";
import { RecordSheet } from "./record-sheet";

/** `/objects/[typeUuid]`: records of one custom object type. */
export function ObjectRecordsPage({ typeUuid }: { typeUuid: string }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const { confirmDelete } = useDialogs();
  const canWrite = can(permissions.tenant.customObjects.write);
  const canManageFields = can(permissions.tenant.customFields.manage);

  const typeQuery = useCustomObjectType(typeUuid);
  const defsQuery = useCustomFieldDefinitions({
    entityType: "object",
    objectTypeUuid: typeUuid,
  });
  const meta = useCustomFieldMeta();
  const definitions = useMemo(
    () =>
      [...(defsQuery.data ?? [])]
        .filter((d) => !d.archived)
        .sort((a, b) => a.position - b.position),
    [defsQuery.data],
  );

  const listState = useServerListState({ initialSort: "-created_at" });
  const [filters, setFilters] = useState<CustomFieldFilterRow[]>([]);
  const filterParams = useMemo(
    () => buildFilterParams(filters, definitions),
    [definitions, filters],
  );
  const { setPagination } = listState;
  const onFiltersChange = useCallback(
    (rows: CustomFieldFilterRow[]) => {
      setFilters(rows);
      setPagination((p) => ({ ...p, pageIndex: 0 }));
    },
    [setPagination],
  );

  const params = useMemo(
    () => ({ ...listState.params, filters: filterParams }),
    [filterParams, listState.params],
  );
  const records = useCustomObjectRecords(typeUuid, params);
  const deleteRecord = useDeleteRecord(typeUuid);

  const [sheet, setSheet] = useState<{
    open: boolean;
    record: CustomObjectRecord | null;
  }>({ open: false, record: null });

  const sortableTypes = useMemo(() => {
    const set = new Set<string>();
    for (const ft of meta.data?.field_types ?? []) {
      if (ft.sortable) set.add(ft.type);
    }
    return set;
  }, [meta.data]);

  const handleDelete = useCallback(
    async (record: CustomObjectRecord) => {
      const ok = await confirmDelete({
        title: t("custom_fields.records.delete_title"),
        description: t("custom_fields.records.delete_description", {
          name: record.display_name,
        }),
      });
      if (ok) await deleteRecord.mutateAsync(record.uuid);
    },
    [confirmDelete, deleteRecord, t],
  );

  const columns = useMemo(() => {
    const cols: ColumnDef<CustomObjectRecord, unknown>[] = [
      createColumn<CustomObjectRecord>({
        id: "display_name",
        accessorKey: "display_name",
        labelKey: "custom_fields.records.display_name",
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) =>
          canWrite ? (
            <button
              type="button"
              className="text-start font-medium hover:underline"
              onClick={() => setSheet({ open: true, record: row.original })}
            >
              {row.original.display_name}
            </button>
          ) : (
            <span className="font-medium">{row.original.display_name}</span>
          ),
      }),
      ...definitions.map((def) =>
        createColumn<CustomObjectRecord>({
          id: `${CF_PREFIX}${def.key}`,
          accessorFn: (row) => row.custom_fields?.[def.key],
          // Labels are user data, not i18n keys.
          labelKey: "",
          label: def.label,
          enableSorting: sortableTypes.has(def.type),
          cell: ({ row }) => (
            <CustomFieldValue
              definition={def}
              value={row.original.custom_fields?.[def.key]}
            />
          ),
        }),
      ),
      createColumn<CustomObjectRecord>({
        id: "created_at",
        accessorKey: "created_at",
        labelKey: "custom_fields.records.created_at",
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm tabular-nums">
            {datetime(row.original.created_at, undefined, locale)}
          </span>
        ),
      }),
      createColumn<CustomObjectRecord>({
        id: "updated_at",
        accessorKey: "updated_at",
        labelKey: "custom_fields.records.updated_at",
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm tabular-nums">
            {datetime(row.original.updated_at, undefined, locale)}
          </span>
        ),
      }),
    ];
    if (canWrite) {
      cols.push(
        createColumn<CustomObjectRecord>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("custom_fields.actions.edit"),
                  icon: Pencil,
                  onSelect: () =>
                    setSheet({ open: true, record: row.original }),
                },
                {
                  id: "delete",
                  label: t("common.delete"),
                  icon: Trash2,
                  variant: "destructive",
                  onSelect: () => void handleDelete(row.original),
                },
              ]}
            />
          ),
        }),
      );
    }
    return cols;
  }, [canWrite, definitions, handleDelete, locale, sortableTypes, t]);

  const pageCount = Math.max(
    1,
    Math.ceil(
      (records.data?.total ?? 0) / (listState.pagination.pageSize || 20),
    ),
  );

  const objectType = typeQuery.data;
  const title = objectType?.plural_label ?? t("custom_fields.records.title");

  if (typeQuery.isError) {
    return (
      <ErrorState
        title={t("custom_fields.records.not_found")}
        onRetry={() => void typeQuery.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <EntityPage
      title={title}
      description={objectType?.description || undefined}
      permission={permissions.tenant.customObjects.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("custom_fields.records.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        {
          label: t("custom_fields.objects.title"),
          href: routes.cms.settings.customObjects,
        },
        { label: title },
      ]}
      actions={
        <div className="flex items-center gap-2">
          {canManageFields ? (
            <Button asChild size="sm" variant="outline">
              <Link
                href={routes.cms.settings.customFieldsTab(
                  objectFieldsTab(typeUuid),
                )}
              >
                <ListChecks className="size-4" />
                {t("custom_fields.objects.actions.fields")}
              </Link>
            </Button>
          ) : null}
          {objectType && !objectType.archived ? (
            <EntityCreateButton
              onClick={() => setSheet({ open: true, record: null })}
              label={t("custom_fields.records.create_named", {
                name: objectType.singular_label,
              })}
              permission={permissions.tenant.customObjects.write}
            />
          ) : null}
        </div>
      }
    >
      <EntityTable
        columns={columns}
        data={records.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={records.isLoading || defsQuery.isLoading}
        isError={records.isError}
        onRetry={() => void records.refetch()}
        emptyTitle={t("custom_fields.records.empty_title")}
        emptyDescription={t("custom_fields.records.empty_description")}
        pageCount={pageCount}
        manual={{ pagination: true, sorting: true, filtering: true }}
        state={listState.tableState}
        features={{
          persistKey: `custom-object-${typeUuid}-v1`,
          multiSort: false,
        }}
        toolbarExtra={
          <>
            <CustomFieldFilterBuilder
              definitions={definitions}
              rows={filters}
              onChange={onFiltersChange}
            />
            <EntityToolbar
              onRefresh={() => void records.refetch()}
              refreshDisabled={records.isFetching}
            />
          </>
        }
      />
      {objectType ? (
        <RecordSheet
          open={sheet.open}
          onOpenChange={(open) => setSheet((s) => ({ ...s, open }))}
          objectType={objectType}
          definitions={definitions}
          record={sheet.record}
        />
      ) : null}
    </EntityPage>
  );
}
