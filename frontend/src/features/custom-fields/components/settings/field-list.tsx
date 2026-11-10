"use client";

import {
  Archive,
  ArchiveRestore,
  ArrowDown,
  ArrowUp,
  GitBranch,
  Pencil,
  Plus,
} from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useArchiveCustomField,
  useReorderCustomFields,
  useRestoreCustomField,
} from "../../hooks/use-custom-field-mutations";
import {
  useCustomFieldDefinitions,
  useCustomFieldMeta,
} from "../../hooks/use-custom-fields";
import type { CustomFieldScope } from "../../services/custom-fields.service";
import type { CustomFieldDefinition } from "../../types";
import { FieldDefinitionDialog } from "./field-definition-dialog";

type FieldListProps = {
  scope: CustomFieldScope;
  canManage: boolean;
};

/** Fields of one entity: order, archive / restore, create / edit. */
export function FieldList({ scope, canManage }: FieldListProps) {
  const { t } = useLocale();
  const { confirm } = useDialogs();
  const [showArchived, setShowArchived] = useState(false);
  const [dialog, setDialog] = useState<{
    open: boolean;
    definition: CustomFieldDefinition | null;
  }>({ open: false, definition: null });
  // Optimistic order while a reorder request is in flight.
  const [pendingOrder, setPendingOrder] = useState<string[] | null>(null);

  // Archived fields are always loaded: the order endpoint needs every field.
  const query = useCustomFieldDefinitions({ ...scope, includeArchived: true });
  const meta = useCustomFieldMeta();
  const reorder = useReorderCustomFields(scope);
  const archive = useArchiveCustomField();
  const restore = useRestoreCustomField();

  const all = useMemo(() => {
    const items = [...(query.data ?? [])].sort(
      (a, b) => a.position - b.position,
    );
    if (!pendingOrder) return items;
    const index = new Map(pendingOrder.map((uuid, i) => [uuid, i]));
    return items.sort(
      (a, b) => (index.get(a.uuid) ?? 0) - (index.get(b.uuid) ?? 0),
    );
  }, [pendingOrder, query.data]);

  const visible = showArchived ? all : all.filter((d) => !d.archived);
  const archivedCount = all.filter((d) => d.archived).length;
  const activeCount = all.length - archivedCount;
  const limit = meta.data?.limits.fields_per_entity;
  const atLimit = Boolean(limit && activeCount >= limit);
  const labelOf = (key: string) => all.find((d) => d.key === key)?.label ?? key;

  const commitOrder = (order: string[]) => {
    setPendingOrder(order);
    reorder.mutate(order, {
      onSettled: () => setPendingOrder(null),
    });
  };

  const move = (def: CustomFieldDefinition, dir: -1 | 1) => {
    const at = visible.findIndex((d) => d.uuid === def.uuid);
    const neighbour = visible[at + dir];
    if (!neighbour) return;
    const order = all.map((d) => d.uuid);
    const i = order.indexOf(def.uuid);
    const j = order.indexOf(neighbour.uuid);
    [order[i], order[j]] = [order[j], order[i]];
    commitOrder(order);
  };

  /**
   * Drag reorder of the visible rows. Hidden (archived) fields keep their
   * slots: the endpoint needs the order of every field.
   */
  const reorderVisible = (rows: CustomFieldDefinition[]) => {
    if (reorder.isPending) return;
    const shown = new Set(rows.map((d) => d.uuid));
    let next = 0;
    commitOrder(
      all.map((d) => (shown.has(d.uuid) ? rows[next++].uuid : d.uuid)),
    );
  };

  const toggleArchive = async (def: CustomFieldDefinition) => {
    if (def.archived) {
      await restore.mutateAsync(def.uuid);
      return;
    }
    const ok = await confirm({
      title: t("custom_fields.archive_title"),
      description: t("custom_fields.archive_description", { name: def.label }),
      confirmLabel: t("custom_fields.actions.archive"),
      variant: "destructive",
    });
    if (ok) await archive.mutateAsync(def.uuid);
  };

  const columns: ColumnDef<CustomFieldDefinition, unknown>[] = [
    ...(canManage
      ? [
          createColumn<CustomFieldDefinition>({
            id: "move",
            labelKey: "custom_fields.columns.order",
            enableHiding: false,
            enableSorting: false,
            searchable: false,
            meta: { headerClassName: "w-10", cellClassName: "w-10" },
            cell: ({ row }) => {
              const index = visible.findIndex(
                (d) => d.uuid === row.original.uuid,
              );
              return (
                <div className="flex flex-col">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={index === 0 || reorder.isPending}
                    aria-label={t("custom_fields.actions.move_up")}
                    onClick={() => move(row.original, -1)}
                  >
                    <ArrowUp className="size-3.5" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={index === visible.length - 1 || reorder.isPending}
                    aria-label={t("custom_fields.actions.move_down")}
                    onClick={() => move(row.original, 1)}
                  >
                    <ArrowDown className="size-3.5" />
                  </Button>
                </div>
              );
            },
          }),
        ]
      : []),
    createColumn<CustomFieldDefinition>({
      id: "field",
      // Search matches the label and the key.
      accessorFn: (def) => `${def.label} ${def.key}`,
      labelKey: "custom_fields.columns.field",
      enableHiding: false,
      gridPrimary: true,
      cell: ({ row }) => {
        const def = row.original;
        return (
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span
                className={cn(
                  "font-medium",
                  def.archived && "text-muted-foreground line-through",
                )}
              >
                {def.label}
              </span>
              {def.required ? (
                <Badge variant="warning">
                  {t("custom_fields.badges.required")}
                </Badge>
              ) : null}
              {def.archived ? (
                <Badge variant="secondary">{t("custom_fields.archived")}</Badge>
              ) : null}
              {def.condition ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Badge variant="outline" className="gap-1">
                      <GitBranch className="size-3" />
                      {t("custom_fields.badges.conditional")}
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent>
                    {t("custom_fields.badges.conditional_hint", {
                      field: labelOf(def.condition.field),
                      operator: t(
                        `custom_fields.condition.operators.${def.condition.operator}`,
                      ),
                    })}
                  </TooltipContent>
                </Tooltip>
              ) : null}
            </div>
            <div className="text-muted-foreground mt-0.5 flex flex-wrap items-center gap-2 text-xs whitespace-normal">
              <code>{def.key}</code>
              <span aria-hidden>·</span>
              <span>{t(`custom_fields.types.${def.type}`)}</span>
              {def.description ? (
                <>
                  <span aria-hidden>·</span>
                  <span className="truncate">{def.description}</span>
                </>
              ) : null}
            </div>
          </div>
        );
      },
    }),
    ...(canManage
      ? [
          createColumn<CustomFieldDefinition>({
            id: "actions",
            labelKey: "common.actions",
            enableHiding: false,
            enableSorting: false,
            searchable: false,
            meta: { headerClassName: "text-end", cellClassName: "text-end" },
            cell: ({ row }) => {
              const def = row.original;
              return (
                <div className="flex items-center justify-end gap-1">
                  {!def.archived ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t("custom_fields.actions.edit")}
                      title={t("custom_fields.actions.edit")}
                      onClick={() => setDialog({ open: true, definition: def })}
                    >
                      <Pencil className="size-4" />
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    disabled={
                      archive.isPending ||
                      restore.isPending ||
                      (def.archived && atLimit)
                    }
                    aria-label={
                      def.archived
                        ? t("custom_fields.actions.restore")
                        : t("custom_fields.actions.archive")
                    }
                    title={
                      def.archived
                        ? t("custom_fields.actions.restore")
                        : t("custom_fields.actions.archive")
                    }
                    onClick={() => void toggleArchive(def)}
                  >
                    {def.archived ? (
                      <ArchiveRestore className="size-4" />
                    ) : (
                      <Archive className="size-4" />
                    )}
                  </Button>
                </div>
              );
            },
          }),
        ]
      : []),
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Switch
            id={`cf-show-archived-${scope.entityType}-${scope.objectTypeUuid ?? ""}`}
            checked={showArchived}
            onCheckedChange={setShowArchived}
          />
          <Label
            htmlFor={`cf-show-archived-${scope.entityType}-${scope.objectTypeUuid ?? ""}`}
            className="font-normal"
          >
            {t("custom_fields.show_archived", { count: archivedCount })}
          </Label>
        </div>
        <div className="flex items-center gap-3">
          {limit ? (
            <span className="text-muted-foreground text-xs tabular-nums">
              {t("custom_fields.limit", { count: activeCount, limit })}
            </span>
          ) : null}
          {canManage ? (
            <Button
              type="button"
              size="sm"
              disabled={atLimit}
              onClick={() => setDialog({ open: true, definition: null })}
            >
              <Plus className="size-4" />
              {t("custom_fields.actions.create")}
            </Button>
          ) : null}
        </div>
      </div>

      <EntityTable
        columns={columns}
        data={visible}
        getRowId={(def) => def.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("custom_fields.empty_title")}
        emptyDescription={t("custom_fields.empty_description")}
        onRowReorder={canManage ? reorderVisible : undefined}
        // Every field comes in one response and the order is the content:
        // no sorting, no pages; search in the browser.
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          sorting: false,
          columnFilters: false,
          columnVisibility: false,
          rowSelection: false,
          pagination: false,
          viewMode: false,
          density: false,
          rowReorder: canManage,
        }}
      />

      {canManage ? (
        <FieldDefinitionDialog
          open={dialog.open}
          onOpenChange={(open) => setDialog((d) => ({ ...d, open }))}
          scope={scope}
          definition={dialog.definition}
          siblings={all}
          maxOptions={meta.data?.limits.options}
        />
      ) : null}
    </div>
  );
}
