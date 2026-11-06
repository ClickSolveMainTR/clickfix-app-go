"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus, Zap } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { ruleKeys, useRules } from "../hooks/use-rules";
import { countConditions, toInput, triggerLabel } from "../lib/rules";
import {
  rulesService,
  type Rule,
  type RuleExecution,
} from "../services/rules.service";

const EMPTY_RULES: Rule[] = [];

/** The rules come in one request: search, sort and paging run here. */
const CLIENT_MODE = { filtering: false, sorting: false, pagination: false };

/**
 * Automation rules of the organization (CF-94): ordered trigger → condition
 * → action rules and the run log of every evaluation. Rows open the editor.
 */
export function RulesPage() {
  const { t, locale } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantAutomationsRead);
  const canWrite = perms.includes(Permission.TenantAutomationsWrite);
  const list = useRules(canRead);
  const items = list.data?.items ?? EMPTY_RULES;

  const columns = useMemo(
    () =>
      [
        createColumn<Rule>({
          accessorKey: "position",
          labelKey: "rules.columns.position",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs tabular-nums">
              {row.original.position}
            </span>
          ),
        }),
        createColumn<Rule>({
          accessorKey: "name",
          labelKey: "rules.columns.name",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => {
            const r = row.original;
            return (
              <div>
                <Link
                  href={routes.cms.rules.detail(r.uuid)}
                  className="font-medium hover:underline"
                >
                  {r.name}
                </Link>
                <p className="text-muted-foreground text-xs font-normal">
                  {triggerLabel(t, r.trigger_event)}
                  {r.trigger_event === "scheduled" &&
                    ` · ${t("rules.every_minutes", { count: r.schedule_minutes })}`}
                </p>
              </div>
            );
          },
        }),
        createColumn<Rule>({
          id: "logic",
          accessorFn: (row) => row.actions.length,
          labelKey: "rules.columns.logic",
          enableSorting: false,
          enableGlobalFilter: false,
          cell: ({ row }) => {
            const r = row.original;
            return (
              <div className="flex flex-wrap items-center gap-1 text-xs">
                <Badge variant="outline">
                  {t("rules.summary.conditions", {
                    count: countConditions(r.conditions),
                  })}
                </Badge>
                <Badge variant="outline">
                  {t("rules.summary.actions", { count: r.actions.length })}
                </Badge>
                {r.stop_on_match && (
                  <Badge variant="secondary">{t("rules.summary.stops")}</Badge>
                )}
              </div>
            );
          },
        }),
        createColumn<Rule>({
          accessorKey: "enabled",
          labelKey: "rules.columns.enabled",
          enableGlobalFilter: false,
          cell: ({ row }) => (
            <RuleToggle rule={row.original} canWrite={canWrite} />
          ),
        }),
        createColumn<Rule>({
          accessorKey: "updated_at",
          labelKey: "rules.columns.updated",
          enableGlobalFilter: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {datetime(row.original.updated_at, "dd.MM.yyyy HH:mm", locale)}
            </span>
          ),
        }),
      ] as ColumnDef<Rule, unknown>[],
    [canWrite, locale, t],
  );

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Zap className="size-7" />}
        title={t("rules.title")}
        description={t("rules.description")}
        actions={
          canWrite ? (
            <Button size="sm" asChild>
              <Link href={routes.cms.rules.detail("new")}>
                <Plus className="size-4" />
                {t("rules.new")}
              </Link>
            </Button>
          ) : undefined
        }
      />
      {org && !canRead && (
        <p className="text-muted-foreground text-sm">{t("rules.forbidden")}</p>
      )}
      {canRead && (
        <Tabs defaultValue="rules">
          <TabsList>
            <TabsTrigger value="rules">{t("rules.tabs.rules")}</TabsTrigger>
            <TabsTrigger value="log">{t("rules.tabs.log")}</TabsTrigger>
          </TabsList>
          <TabsContent value="rules" className="mt-4 space-y-4">
            <EntityTable
              columns={columns}
              data={items}
              getRowId={(row) => row.uuid}
              isLoading={list.isLoading}
              isError={list.isError}
              errorTitle={t("rules.error.title")}
              errorDescription={t("rules.error.description")}
              onRetry={() => void list.refetch()}
              emptyDescription={t("rules.empty")}
              manual={CLIENT_MODE}
              initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
              features={{
                persistKey: "tenant-automation-rules-v1",
                columnFilters: false,
                rowSelection: false,
              }}
            />
            <p className="text-muted-foreground text-xs">
              {t("rules.order_note")}
            </p>
          </TabsContent>
          <TabsContent value="log" className="mt-4">
            <RunLog rules={items} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function RuleToggle({ rule: r, canWrite }: { rule: Rule; canWrite: boolean }) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const qc = useQueryClient();
  const toggle = useMutation({
    mutationFn: (enabled: boolean) =>
      rulesService.update(org?.slug ?? "", r.uuid, {
        ...toInput(r),
        enabled,
      }),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: ruleKeys.all(org?.slug) }),
  });
  const enabled = toggle.isPending ? Boolean(toggle.variables) : r.enabled;
  return (
    <Switch
      checked={enabled}
      disabled={!canWrite || toggle.isPending}
      aria-label={t("rules.fields.enabled")}
      onCheckedChange={(v) => toggle.mutate(v)}
    />
  );
}

const TONE: Record<
  RuleExecution["status"],
  "success" | "warning" | "danger" | "secondary"
> = {
  succeeded: "success",
  running: "warning",
  failed: "danger",
  skipped: "secondary",
};

/** Every rule evaluation, newest first: matched, failed step and error. */
function RunLog({ rules }: { rules: Rule[] }) {
  const { t, locale } = useLocale();
  const { active: org } = useActiveOrganization();
  const listState = useServerListState({ initialPageSize: 25 });
  const { limit, offset } = listState.params;
  const log = useQuery({
    queryKey: [...ruleKeys.all(org?.slug), "executions", limit, offset],
    queryFn: () => rulesService.executions(org?.slug ?? "", limit, offset),
    enabled: Boolean(org),
    placeholderData: (prev) => prev,
  });
  const byId = useMemo(() => new Map(rules.map((r) => [r.id, r])), [rules]);

  const columns = useMemo(
    () =>
      [
        createColumn<RuleExecution>({
          accessorKey: "created_at",
          labelKey: "rules.log.when",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {datetime(row.original.created_at, "dd.MM HH:mm:ss", locale)}
            </span>
          ),
        }),
        createColumn<RuleExecution>({
          accessorKey: "rule_id",
          labelKey: "rules.log.rule",
          gridPrimary: true,
          cell: ({ row }) => {
            const rule = byId.get(row.original.rule_id);
            return rule ? (
              <Link
                href={routes.cms.rules.detail(rule.uuid)}
                className="hover:underline"
              >
                {rule.name}
              </Link>
            ) : (
              <span className="text-muted-foreground text-xs">
                {t("rules.log.deleted_rule")}
              </span>
            );
          },
        }),
        createColumn<RuleExecution>({
          accessorKey: "conversation_uuid",
          labelKey: "rules.log.conversation",
          cell: ({ row }) => (
            <Link
              href={`${routes.cms.inbox}?c=${row.original.conversation_uuid}`}
              className="font-mono text-xs hover:underline"
            >
              {row.original.conversation_uuid.slice(0, 8)}
            </Link>
          ),
        }),
        createColumn<RuleExecution>({
          accessorKey: "status",
          labelKey: "rules.log.result",
          cell: ({ row }) => (
            <Badge variant={TONE[row.original.status] ?? "secondary"}>
              {t(`rules.log.status.${row.original.status}`)}
            </Badge>
          ),
        }),
        createColumn<RuleExecution>({
          id: "detail",
          accessorFn: (row) => row.error ?? "",
          labelKey: "rules.log.detail",
          cell: ({ row }) => {
            const e = row.original;
            return (
              <span className="text-muted-foreground block max-w-xs text-xs">
                {e.status === "skipped"
                  ? t("rules.log.not_matched")
                  : e.status === "failed"
                    ? t("rules.log.failed_at", {
                        step: e.action_index + 1,
                        error: e.error || "—",
                      })
                    : e.status === "succeeded"
                      ? t("rules.log.ran", { count: e.action_index })
                      : "—"}
              </span>
            );
          },
        }),
      ] as ColumnDef<RuleExecution, unknown>[],
    [byId, locale, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={log.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={log.isLoading}
      isError={log.isError}
      errorTitle={t("rules.error.title")}
      errorDescription={t("rules.error.description")}
      onRetry={() => void log.refetch()}
      emptyDescription={t("rules.log.empty")}
      total={log.data?.total ?? 0}
      state={listState.tableState}
      features={{
        persistKey: "tenant-automation-log-v1",
        // The endpoint takes `limit` and `offset` only.
        sorting: false,
        globalFilter: false,
        columnFilters: false,
        rowSelection: false,
      }}
    />
  );
}
