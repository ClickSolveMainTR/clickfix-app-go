"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Award, Pencil, Plus } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { isApiError } from "@/lib/api";
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
  const list = rules.data ?? [];

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

  const toggle = (rule: GameRule, enabled: boolean) =>
    save.mutate({
      name: rule.name,
      metric: rule.metric,
      threshold: rule.threshold,
      points: rule.points,
      min_samples: rule.min_samples,
      enabled,
    });

  if (rules.isLoading) return <Loading label={t("common.loading")} />;
  if (rules.isError)
    return (
      <ErrorState
        title={t("coaching.error.title")}
        description={
          isApiError(rules.error)
            ? rules.error.message
            : t("coaching.error.description")
        }
        onRetry={() => void rules.refetch()}
      />
    );

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

      {list.length === 0 ? (
        <EmptyState
          icon={<Award className="size-6" />}
          title={t("coaching.rules.empty_title")}
          description={t("coaching.rules.empty")}
        />
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full min-w-[40rem] text-sm">
            <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
              <tr>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t("coaching.rules.name")}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t("coaching.rules.metric")}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t("coaching.rules.threshold")}
                </th>
                <th scope="col" className="px-3 py-2 text-right font-medium">
                  {t("coaching.rules.points")}
                </th>
                <th scope="col" className="px-3 py-2 text-right font-medium">
                  {t("coaching.rules.min_samples")}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t("coaching.rules.enabled")}
                </th>
                {canWrite && <th scope="col" className="w-12" />}
              </tr>
            </thead>
            <tbody className="divide-y">
              {list.map((r) => (
                <tr key={r.uuid} className={r.enabled ? "" : "opacity-60"}>
                  <td className="max-w-[14rem] truncate px-3 py-2 font-medium">
                    {r.name}
                  </td>
                  <td className="px-3 py-2">
                    {t(`coaching.metric.${r.metric}`)}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap tabular-nums">
                    {thresholdLabel(r.metric, r.threshold, locale)}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {formatScore(r.points, locale, 0)}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {formatScore(r.min_samples, locale, 0)}
                  </td>
                  <td className="px-3 py-2">
                    <Switch
                      checked={r.enabled}
                      disabled={!canWrite || save.isPending}
                      aria-label={t("coaching.rules.enabled")}
                      onCheckedChange={(v) => toggle(r, v)}
                    />
                  </td>
                  {canWrite && (
                    <td className="px-2 py-2">
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={t("coaching.rules.edit")}
                        onClick={() => setEditing(r)}
                      >
                        <Pencil className="size-4" />
                      </Button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
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
