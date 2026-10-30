"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Plus, Zap } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
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

const LOG_PAGE = 25;

/**
 * Automation rules of the organization (CF-94): ordered trigger → condition
 * → action rules and the run log of every evaluation. Rows open the editor.
 */
export function RulesPage() {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantAutomationsRead);
  const canWrite = perms.includes(Permission.TenantAutomationsWrite);
  const list = useRules(canRead);
  const items = list.data?.items ?? [];

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
            {list.isLoading && <Loading label={t("common.loading")} />}
            {list.isError && (
              <ErrorState
                title={t("rules.error.title")}
                description={t("rules.error.description")}
                onRetry={() => void list.refetch()}
              />
            )}
            {list.isSuccess && items.length === 0 && (
              <Card>
                <CardContent className="text-muted-foreground py-10 text-center text-sm">
                  {t("rules.empty")}
                </CardContent>
              </Card>
            )}
            {items.length > 0 && (
              <Card className="overflow-hidden py-0">
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
                      <tr>
                        <th className="w-12 px-4 py-2.5 font-medium">
                          {t("rules.columns.position")}
                        </th>
                        <th className="px-4 py-2.5 font-medium">
                          {t("rules.columns.name")}
                        </th>
                        <th className="px-4 py-2.5 font-medium">
                          {t("rules.columns.logic")}
                        </th>
                        <th className="px-4 py-2.5 font-medium">
                          {t("rules.columns.enabled")}
                        </th>
                        <th className="px-4 py-2.5 font-medium">
                          {t("rules.columns.updated")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {items.map((r) => (
                        <RuleRow key={r.uuid} rule={r} canWrite={canWrite} />
                      ))}
                    </tbody>
                  </table>
                </div>
              </Card>
            )}
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

function RuleRow({ rule: r, canWrite }: { rule: Rule; canWrite: boolean }) {
  const { t, locale } = useLocale();
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
    <tr className="hover:bg-muted/40 border-t">
      <td className="text-muted-foreground px-4 py-3 text-xs tabular-nums">
        {r.position}
      </td>
      <td className="px-4 py-3">
        <Link
          href={routes.cms.rules.detail(r.uuid)}
          className="font-medium hover:underline"
        >
          {r.name}
        </Link>
        <p className="text-muted-foreground text-xs">
          {triggerLabel(t, r.trigger_event)}
          {r.trigger_event === "scheduled" &&
            ` · ${t("rules.every_minutes", { count: r.schedule_minutes })}`}
        </p>
      </td>
      <td className="px-4 py-3">
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
      </td>
      <td className="px-4 py-3">
        <Switch
          checked={enabled}
          disabled={!canWrite || toggle.isPending}
          aria-label={t("rules.fields.enabled")}
          onCheckedChange={(v) => toggle.mutate(v)}
        />
      </td>
      <td className="text-muted-foreground px-4 py-3 text-xs">
        {datetime(r.updated_at, "dd.MM.yyyy HH:mm", locale)}
      </td>
    </tr>
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
  const [offset, setOffset] = useState(0);
  const log = useQuery({
    queryKey: [...ruleKeys.all(org?.slug), "executions", offset],
    queryFn: () => rulesService.executions(org?.slug ?? "", LOG_PAGE, offset),
    enabled: Boolean(org),
    placeholderData: (prev) => prev,
  });
  const byId = new Map(rules.map((r) => [r.id, r]));
  const items = log.data?.items ?? [];
  const total = log.data?.total ?? 0;

  if (log.isLoading) return <Loading label={t("common.loading")} />;
  if (log.isError)
    return (
      <ErrorState
        title={t("rules.error.title")}
        description={t("rules.error.description")}
        onRetry={() => void log.refetch()}
      />
    );
  if (items.length === 0)
    return (
      <Card>
        <CardContent className="text-muted-foreground py-10 text-center text-sm">
          {t("rules.log.empty")}
        </CardContent>
      </Card>
    );
  return (
    <div className="space-y-3">
      <Card className="overflow-hidden py-0">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
              <tr>
                <th className="px-4 py-2.5 font-medium">
                  {t("rules.log.when")}
                </th>
                <th className="px-4 py-2.5 font-medium">
                  {t("rules.log.rule")}
                </th>
                <th className="px-4 py-2.5 font-medium">
                  {t("rules.log.conversation")}
                </th>
                <th className="px-4 py-2.5 font-medium">
                  {t("rules.log.result")}
                </th>
                <th className="px-4 py-2.5 font-medium">
                  {t("rules.log.detail")}
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((e) => {
                const rule = byId.get(e.rule_id);
                return (
                  <tr key={e.uuid} className="border-t align-top">
                    <td className="text-muted-foreground px-4 py-3 text-xs whitespace-nowrap">
                      {datetime(e.created_at, "dd.MM HH:mm:ss", locale)}
                    </td>
                    <td className="px-4 py-3">
                      {rule ? (
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
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <Link
                        href={`${routes.cms.inbox}?c=${e.conversation_uuid}`}
                        className="font-mono text-xs hover:underline"
                      >
                        {e.conversation_uuid.slice(0, 8)}
                      </Link>
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant={TONE[e.status] ?? "secondary"}>
                        {t(`rules.log.status.${e.status}`)}
                      </Badge>
                    </td>
                    <td className="text-muted-foreground max-w-xs px-4 py-3 text-xs">
                      {e.status === "skipped"
                        ? t("rules.log.not_matched")
                        : e.status === "failed"
                          ? t("rules.log.failed_at", {
                              step: e.action_index + 1,
                              error: e.error || "—",
                            })
                          : e.status === "succeeded"
                            ? t("rules.log.ran", {
                                count: e.action_index,
                              })
                            : "—"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </Card>
      <div className="flex items-center justify-end gap-2 text-xs">
        <span className="text-muted-foreground tabular-nums">
          {t("rules.log.range", {
            from: offset + 1,
            to: offset + items.length,
            total,
          })}
        </span>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="size-8"
          aria-label={t("rules.log.previous")}
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - LOG_PAGE))}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="size-8"
          aria-label={t("rules.log.next")}
          disabled={offset + items.length >= total}
          onClick={() => setOffset(offset + LOG_PAGE)}
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  );
}
