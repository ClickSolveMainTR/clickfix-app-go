"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { formatScore, thresholdLabel } from "../lib/format";
import {
  coachingService,
  type GameRule,
  type GameRuleInput,
} from "../services/coaching.service";
import { RuleDialog } from "./rule-dialog";

const MAX_RULES = 100;

/**
 * Badge / points rules (CF-109). Everyone with schedules.read sees them;
 * quality managers add, edit and switch them off (there is no delete).
 */
export function RulesTab({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const key = ["tenant", slug, "gamification", "rules"];
  const [editing, setEditing] = useState<GameRule | "new" | null>(null);

  const rules = useQuery({
    queryKey: key,
    queryFn: () => coachingService.rules(slug),
  });
  const list = useMemo(() => rules.data ?? [], [rules.data]);

  const save = useMutation({
    mutationFn: (body: GameRuleInput) => coachingService.saveRule(slug, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "gamification"],
      });
      setEditing(null);
      appToast.success(t("coaching.rules.saved"));
    },
  });

  const { mutate } = save;
  const toggle = useCallback(
    (rule: GameRule, enabled: boolean) =>
      mutate({
        name: rule.name,
        metric: rule.metric,
        threshold: rule.threshold,
        points: rule.points,
        min_samples: rule.min_samples,
        enabled,
      }),
    [mutate],
  );

  const columns = useMemo(() => {
    // A switched-off rule stays listed, dimmed.
    const dim = (r: GameRule) => cn(!r.enabled && "opacity-60");
    return [
      createColumn<GameRule>({
        accessorKey: "name",
        labelKey: "coaching.rules.name",
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span
            className={cn(
              "block max-w-[14rem] truncate font-medium",
              dim(row.original),
            )}
          >
            {row.original.name}
          </span>
        ),
      }),
      createColumn<GameRule>({
        id: "metric",
        accessorFn: (r) => t(`coaching.metric.${r.metric}`),
        labelKey: "coaching.rules.metric",
        cell: ({ row, getValue }) => (
          <span className={dim(row.original)}>{getValue() as string}</span>
        ),
      }),
      createColumn<GameRule>({
        accessorKey: "threshold",
        labelKey: "coaching.rules.threshold",
        searchable: false,
        cell: ({ row }) => (
          <span
            className={cn("whitespace-nowrap tabular-nums", dim(row.original))}
          >
            {thresholdLabel(
              row.original.metric,
              row.original.threshold,
              locale,
            )}
          </span>
        ),
      }),
      createColumn<GameRule>({
        accessorKey: "points",
        labelKey: "coaching.rules.points",
        searchable: false,
        meta: {
          headerClassName: "text-right",
          cellClassName: "text-right tabular-nums",
        },
        cell: ({ row }) => (
          <span className={dim(row.original)}>
            {formatScore(row.original.points, locale, 0)}
          </span>
        ),
      }),
      createColumn<GameRule>({
        accessorKey: "min_samples",
        labelKey: "coaching.rules.min_samples",
        searchable: false,
        meta: {
          headerClassName: "text-right",
          cellClassName: "text-right tabular-nums",
        },
        cell: ({ row }) => (
          <span className={dim(row.original)}>
            {formatScore(row.original.min_samples, locale, 0)}
          </span>
        ),
      }),
      createColumn<GameRule>({
        accessorKey: "enabled",
        labelKey: "coaching.rules.enabled",
        searchable: false,
        cell: ({ row }) => (
          <Switch
            checked={row.original.enabled}
            disabled={!canWrite || save.isPending}
            aria-label={t("coaching.rules.enabled")}
            onCheckedChange={(v) => toggle(row.original, v)}
          />
        ),
      }),
      ...(canWrite
        ? [
            createColumn<GameRule>({
              id: "actions",
              labelKey: "common.actions",
              enableHiding: false,
              enableSorting: false,
              meta: { headerClassName: "w-12", cellClassName: "w-12" },
              cell: ({ row }) => (
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t("coaching.rules.edit")}
                  onClick={() => setEditing(row.original)}
                >
                  <Pencil className="size-4" />
                </Button>
              ),
            }),
          ]
        : []),
    ] as ColumnDef<GameRule, unknown>[];
  }, [canWrite, locale, save.isPending, t, toggle]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground max-w-2xl text-sm">
          {t("coaching.rules.description")}
        </p>
        {canWrite && (
          <Button
            disabled={list.length >= MAX_RULES}
            onClick={() => setEditing("new")}
          >
            <Plus className="size-4" />
            {t("coaching.rules.new")}
          </Button>
        )}
      </div>

      <EntityTable
        columns={columns}
        data={list}
        getRowId={(r) => r.uuid}
        isLoading={rules.isLoading}
        isError={rules.isError}
        errorTitle={t("coaching.error.title")}
        errorDescription={
          isApiError(rules.error)
            ? rules.error.message
            : t("coaching.error.description")
        }
        onRetry={() => void rules.refetch()}
        emptyTitle={t("coaching.rules.empty_title")}
        emptyDescription={t("coaching.rules.empty")}
        // One unpaged list (at most 100 rules): search, sort and pages in the
        // browser.
        manual={{ filtering: false, sorting: false, pagination: false }}
        initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
        features={{
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
      <p className="text-muted-foreground text-xs">
        {t("coaching.rules.hint")}
      </p>

      {editing && (
        <RuleDialog
          rule={editing === "new" ? null : editing}
          names={list.map((r) => r.name)}
          pending={save.isPending}
          onOpenChange={(open) => !open && setEditing(null)}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
    </div>
  );
}
