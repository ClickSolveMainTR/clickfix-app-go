"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  KeyRound,
  Pencil,
  Plus,
  ShieldCheck,
  Wrench,
} from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
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

const CALLS_PAGE = 20;

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
  const [offset, setOffset] = useState(0);
  const calls = useQuery({
    queryKey: ["tenant", slug, "ai", "tool-calls", offset],
    queryFn: () => aiSettingsService.toolCalls(slug, offset, CALLS_PAGE),
    placeholderData: keepPreviousData,
  });
  const [editing, setEditing] = useState<AIToolView | "new" | null>(null);
  const items = tools.data?.items ?? [];
  const callItems = calls.data?.items ?? [];
  const callTotal = calls.data?.total ?? 0;

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
        {!calls.data && (
          <QueryState
            loading={calls.isLoading}
            error={calls.isError}
            onRetry={() => void calls.refetch()}
          />
        )}
        {calls.data && callItems.length === 0 && (
          <p className="text-muted-foreground rounded-md border border-dashed p-6 text-center text-sm">
            {t("ai_settings.tools.calls_empty")}
          </p>
        )}
        {callItems.length > 0 && (
          <Card className="py-0">
            <CardContent className="overflow-x-auto p-0">
              <table className="w-full min-w-[36rem] text-sm">
                <thead className="text-muted-foreground border-b text-xs">
                  <tr>
                    <th className="px-3 py-2 text-start font-medium">
                      {t("ai_settings.runs.created")}
                    </th>
                    <th className="px-3 py-2 text-start font-medium">
                      {t("ai_settings.tools.name")}
                    </th>
                    <th className="px-3 py-2 text-start font-medium">
                      {t("ai_settings.runs.status_label")}
                    </th>
                    <th className="px-3 py-2 text-start font-medium">
                      {t("ai_settings.tools.error_code")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {callItems.map((call) => (
                    <tr key={call.uuid} className="border-b last:border-0">
                      <td className="px-3 py-2 whitespace-nowrap">
                        {datetime(
                          call.created_at,
                          "dd.MM.yyyy HH:mm:ss",
                          locale,
                        )}
                      </td>
                      <td className="px-3 py-2">
                        <code className="text-xs">{call.tool_name}</code>
                      </td>
                      <td className="px-3 py-2">
                        <Badge variant={CALL_VARIANT[call.status] ?? "outline"}>
                          {t(`ai_settings.tools.call_status.${call.status}`)}
                        </Badge>
                      </td>
                      <td className="text-muted-foreground px-3 py-2">
                        {call.error_code ? (
                          <code className="text-xs">{call.error_code}</code>
                        ) : (
                          "-"
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </CardContent>
          </Card>
        )}
        {callTotal > CALLS_PAGE && (
          <div className="flex items-center justify-between gap-2 text-sm">
            <span className="text-muted-foreground">
              {t("ai_settings.pager", {
                from: offset + 1,
                to: Math.min(offset + CALLS_PAGE, callTotal),
                total: callTotal,
              })}
            </span>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={offset === 0}
                onClick={() => setOffset((o) => Math.max(0, o - CALLS_PAGE))}
              >
                <ChevronLeft className="size-4" />
                {t("ai_settings.prev")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={offset + CALLS_PAGE >= callTotal}
                onClick={() => setOffset((o) => o + CALLS_PAGE)}
              >
                {t("ai_settings.next")}
                <ChevronRight className="size-4" />
              </Button>
            </div>
          </div>
        )}
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
