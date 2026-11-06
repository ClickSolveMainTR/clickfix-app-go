"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { KeyRound, Pencil, Plus, ShieldCheck, Wrench } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge, type BadgeProps } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import {
  aiSettingsService,
  BUILTIN_TOOLS,
  type AIAgentToolCall,
  type AIToolView,
} from "../services/ai-settings.service";
import { QueryState } from "./parts";
import type { AISettingsTabProps } from "./tab-registry";
import { ToolDialog } from "./tool-dialog";

export const toolsKey = (slug: string) =>
  ["tenant", slug, "ai", "tools"] as const;

const CALL_VARIANT: Record<AIAgentToolCall["status"], BadgeProps["variant"]> = {
  completed: "success",
  started: "secondary",
  failed: "danger",
  dry_run: "outline",
};

function host(url: string) {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/** "Araçlar" tab (CF-67): built-in tools, organization HTTP tools, call log. */
export function ToolsTab({ slug, canWrite }: AISettingsTabProps) {
  const { t, locale } = useLocale();
  const tools = useQuery({
    queryKey: toolsKey(slug),
    queryFn: () => aiSettingsService.tools(slug),
  });
  const callsState = useServerListState({ initialPageSize: 20 });
  const { limit, offset } = callsState.params;
  const calls = useQuery({
    queryKey: ["tenant", slug, "ai", "tool-calls", limit, offset],
    queryFn: () => aiSettingsService.toolCalls(slug, offset, limit),
    placeholderData: keepPreviousData,
  });
  const callColumns = useMemo(
    () =>
      [
        createColumn<AIAgentToolCall>({
          accessorKey: "created_at",
          labelKey: "ai_settings.runs.created",
          enableHiding: false,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {datetime(row.original.created_at, "dd.MM.yyyy HH:mm:ss", locale)}
            </span>
          ),
        }),
        createColumn<AIAgentToolCall>({
          accessorKey: "tool_name",
          labelKey: "ai_settings.tools.name",
          gridPrimary: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.tool_name}</code>
          ),
        }),
        createColumn<AIAgentToolCall>({
          accessorKey: "status",
          labelKey: "ai_settings.runs.status_label",
          cell: ({ row }) => (
            <Badge variant={CALL_VARIANT[row.original.status] ?? "outline"}>
              {t(`ai_settings.tools.call_status.${row.original.status}`)}
            </Badge>
          ),
        }),
        createColumn<AIAgentToolCall>({
          accessorKey: "error_code",
          labelKey: "ai_settings.tools.error_code",
          cell: ({ row }) =>
            row.original.error_code ? (
              <code className="text-muted-foreground text-xs">
                {row.original.error_code}
              </code>
            ) : (
              <span className="text-muted-foreground">-</span>
            ),
        }),
      ] as ColumnDef<AIAgentToolCall, unknown>[],
    [locale, t],
  );
  const [editing, setEditing] = useState<AIToolView | "new" | null>(null);
  const items = tools.data?.items ?? [];

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="size-4" />
            {t("ai_settings.tools.builtin_title")}
          </CardTitle>
          <p className="text-muted-foreground text-sm">
            {t("ai_settings.tools.builtin_hint")}
          </p>
        </CardHeader>
        <CardContent>
          <ul className="grid gap-2 sm:grid-cols-2">
            {BUILTIN_TOOLS.map((tool) => (
              <li key={tool.name} className="rounded-md border p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <code className="text-sm font-medium">{tool.name}</code>
                  {tool.requiresApproval && (
                    <Badge variant="warning">
                      {t("ai_settings.tools.requires_approval")}
                    </Badge>
                  )}
                </div>
                <p className="text-muted-foreground mt-1 text-xs">
                  {t(`ai_settings.tools.builtin_desc.${tool.name}`)}
                </p>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>

      <section className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <h3 className="text-base font-semibold">
              {t("ai_settings.tools.http_title")}
            </h3>
            <p className="text-muted-foreground max-w-2xl text-sm">
              {t("ai_settings.tools.http_hint")}
            </p>
          </div>
          {canWrite && (
            <Button size="sm" onClick={() => setEditing("new")}>
              <Plus className="size-4" />
              {t("ai_settings.tools.new")}
            </Button>
          )}
        </div>
        {!tools.data && (
          <QueryState
            loading={tools.isLoading}
            error={tools.isError}
            onRetry={() => void tools.refetch()}
          />
        )}
        {tools.data && items.length === 0 && (
          <EmptyState
            className="py-8"
            icon={<Wrench className="size-6" />}
            title={t("ai_settings.tools.empty_title")}
            description={t("ai_settings.tools.empty")}
          />
        )}
        <ul className="space-y-3">
          {items.map((tool) => (
            <li key={tool.uuid}>
              <Card className="py-0">
                <CardContent className="flex flex-wrap items-start gap-3 py-4">
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <code className="font-medium">{tool.name}</code>
                      <Badge variant={tool.enabled ? "success" : "secondary"}>
                        {tool.enabled
                          ? t("ai_settings.tools.enabled")
                          : t("ai_settings.tools.disabled")}
                      </Badge>
                      {tool.requires_approval && (
                        <Badge variant="warning">
                          {t("ai_settings.tools.requires_approval")}
                        </Badge>
                      )}
                      {tool.has_auth && (
                        <Badge variant="outline" className="gap-1">
                          <KeyRound className="size-3" />
                          {t("ai_settings.tools.has_auth")}
                        </Badge>
                      )}
                    </div>
                    {tool.description && (
                      <p className="text-muted-foreground text-sm">
                        {tool.description}
                      </p>
                    )}
                    <p className="text-muted-foreground flex flex-wrap gap-x-3 text-xs">
                      <span className="break-all">{host(tool.url)}</span>
                      <span>
                        {t("ai_settings.tools.limit_value", {
                          count: tool.daily_limit,
                        })}
                      </span>
                    </p>
                  </div>
                  {canWrite && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setEditing(tool)}
                    >
                      <Pencil className="size-4" />
                      {t("ai_settings.tools.edit")}
                    </Button>
                  )}
                </CardContent>
              </Card>
            </li>
          ))}
        </ul>
      </section>

      <section className="space-y-3">
        <div className="space-y-1">
          <h3 className="text-base font-semibold">
            {t("ai_settings.tools.calls_title")}
          </h3>
          <p className="text-muted-foreground text-sm">
            {t("ai_settings.tools.calls_hint")}
          </p>
        </div>
        <EntityTable
          columns={callColumns}
          data={calls.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={calls.isLoading}
          isError={calls.isError}
          onRetry={() => void calls.refetch()}
          emptyDescription={t("ai_settings.tools.calls_empty")}
          total={calls.data?.total ?? 0}
          state={callsState.tableState}
          features={{
            // The endpoint takes `limit` and `offset` only.
            toolbar: false,
            sorting: false,
            globalFilter: false,
            columnFilters: false,
            rowSelection: false,
            viewMode: false,
          }}
        />
      </section>

      {editing !== null && (
        <ToolDialog
          slug={slug}
          tool={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => setEditing(null)}
        />
      )}
    </div>
  );
}
