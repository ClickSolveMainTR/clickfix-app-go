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
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
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

  const move = (def: CustomFieldDefinition, dir: -1 | 1) => {
    const at = visible.findIndex((d) => d.uuid === def.uuid);
    const neighbour = visible[at + dir];
    if (!neighbour) return;
    const order = all.map((d) => d.uuid);
    const i = order.indexOf(def.uuid);
    const j = order.indexOf(neighbour.uuid);
    [order[i], order[j]] = [order[j], order[i]];
    setPendingOrder(order);
    reorder.mutate(order, {
      onSettled: () => setPendingOrder(null),
    });
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

  if (query.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

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

      {query.isLoading ? (
        <div className="flex flex-col gap-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-14 w-full" />
          ))}
        </div>
      ) : visible.length === 0 ? (
        <EmptyState
          title={t("custom_fields.empty_title")}
          description={t("custom_fields.empty_description")}
        />
      ) : (
        <Card className="divide-border divide-y p-0">
          <ul className="divide-border divide-y">
            {visible.map((def, index) => (
              <li
                key={def.uuid}
                className={cn(
                  "flex flex-wrap items-center gap-3 px-4 py-3",
                  def.archived && "bg-muted/40",
                )}
              >
                {canManage ? (
                  <div className="flex flex-col">
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      disabled={index === 0 || reorder.isPending}
                      aria-label={t("custom_fields.actions.move_up")}
                      onClick={() => move(def, -1)}
                    >
                      <ArrowUp className="size-3.5" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      disabled={
                        index === visible.length - 1 || reorder.isPending
                      }
                      aria-label={t("custom_fields.actions.move_down")}
                      onClick={() => move(def, 1)}
                    >
                      <ArrowDown className="size-3.5" />
                    </Button>
                  </div>
                ) : null}
                <div className="min-w-0 flex-1">
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
                      <Badge variant="secondary">
                        {t("custom_fields.archived")}
                      </Badge>
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
                  <div className="text-muted-foreground mt-0.5 flex flex-wrap items-center gap-2 text-xs">
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
                {canManage ? (
                  <div className="flex items-center gap-1">
                    {!def.archived ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("custom_fields.actions.edit")}
                        title={t("custom_fields.actions.edit")}
                        onClick={() =>
                          setDialog({ open: true, definition: def })
                        }
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
                ) : null}
              </li>
            ))}
          </ul>
        </Card>
      )}

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
