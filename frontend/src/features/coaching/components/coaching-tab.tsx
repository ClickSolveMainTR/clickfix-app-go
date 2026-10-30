"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Flag, ListTodo, NotebookPen, Plus } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { OptionalSelect } from "@/features/reports/components/report-filters";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { formatDate } from "../lib/format";
import {
  coachingService,
  type CoachingInput,
  type CoachingItem,
  type CoachingKind,
} from "../services/coaching.service";
import { CoachingItemDialog } from "./coaching-item-dialog";

/** The API pages at 100; one agent's plan is expected to fit. */
const LIMIT = 100;

type NewItem = { kind: CoachingKind; parent: string };

/**
 * Coaching notes, goals and follow-up tasks per agent (CF-109). Goals and
 * tasks can be completed or reopened; notes are a plain log.
 */
export function CoachingTab({ slug }: { slug: string }) {
  const { t } = useLocale();
  const directory = useMemberDirectory();
  const queryClient = useQueryClient();
  const [agent, setAgent] = useState("");
  const [creating, setCreating] = useState<NewItem | null>(null);
  // Overdue badges compare against the time the tab was opened.
  const [now] = useState(() => Date.now());
  const key = ["tenant", slug, "coaching", agent];

  const list = useQuery({
    queryKey: key,
    queryFn: () =>
      coachingService.items(slug, { agent, limit: LIMIT, offset: 0 }),
    enabled: Boolean(agent),
  });
  const items = useMemo(() => list.data?.items ?? [], [list.data]);

  const grouped = useMemo(() => {
    const goals = items.filter((i) => i.kind === "goal");
    const tasks = new Map<string, CoachingItem[]>();
    for (const i of items) {
      if (i.kind !== "task" || !i.parent_uuid) continue;
      tasks.set(i.parent_uuid, [...(tasks.get(i.parent_uuid) ?? []), i]);
    }
    return {
      goals,
      tasks,
      notes: items.filter((i) => i.kind === "note"),
    };
  }, [items]);

  const create = useMutation({
    mutationFn: (body: CoachingInput) => coachingService.createItem(slug, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: key });
      setCreating(null);
      appToast.success(t("coaching.items.created"));
    },
  });
  const complete = useMutation({
    mutationFn: (p: { uuid: string; completed: boolean }) =>
      coachingService.completeItem(slug, p.uuid, p.completed),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: key }),
  });

  const toggle = (item: CoachingItem) => (
    <Checkbox
      checked={Boolean(item.completed_at)}
      disabled={complete.isPending}
      aria-label={t("coaching.items.complete")}
      onCheckedChange={(v) =>
        complete.mutate({ uuid: item.uuid, completed: v === true })
      }
      className="mt-0.5"
    />
  );

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 py-4">
          <OptionalSelect
            id="coaching-agent"
            label={t("coaching.filters.agent")}
            value={agent}
            options={directory.members}
            anyLabel={t("coaching.filters.pick_agent")}
            onChange={setAgent}
            className="w-full sm:w-64"
          />
          {agent && (
            <Button
              className="sm:ml-auto"
              onClick={() => setCreating({ kind: "note", parent: "" })}
            >
              <Plus className="size-4" />
              {t("coaching.items.new")}
            </Button>
          )}
        </CardContent>
      </Card>

      {!agent ? (
        <EmptyState
          icon={<NotebookPen className="size-6" />}
          title={t("coaching.items.pick_title")}
          description={t("coaching.items.pick")}
        />
      ) : list.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : list.isError ? (
        <ErrorState
          title={t("coaching.error.title")}
          description={
            isApiError(list.error)
              ? list.error.message
              : t("coaching.error.description")
          }
          onRetry={() => void list.refetch()}
        />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[3fr_2fr]">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Flag className="size-4" />
                {t("coaching.items.goals")}
              </CardTitle>
            </CardHeader>
            <CardContent className="grid gap-3">
              {grouped.goals.length === 0 && (
                <p className="text-muted-foreground text-sm">
                  {t("coaching.items.no_goals")}
                </p>
              )}
              {grouped.goals.map((g) => (
                <div key={g.uuid} className="grid gap-2 rounded-md border p-3">
                  <div className="flex items-start gap-2">
                    {toggle(g)}
                    <div className="grid min-w-0 flex-1 gap-1">
                      <p
                        className={cn(
                          "text-sm break-words whitespace-pre-wrap",
                          g.completed_at &&
                            "text-muted-foreground line-through",
                        )}
                      >
                        {g.body}
                      </p>
                      <ItemMeta
                        now={now}
                        item={g}
                        coach={directory.memberName(g.coach_uuid)}
                      />
                    </div>
                  </div>
                  <div className="ml-6 grid gap-2 border-l pl-3">
                    {(grouped.tasks.get(g.uuid) ?? []).map((task) => (
                      <div key={task.uuid} className="flex items-start gap-2">
                        {toggle(task)}
                        <div className="grid min-w-0 flex-1 gap-0.5">
                          <p
                            className={cn(
                              "text-sm break-words whitespace-pre-wrap",
                              task.completed_at &&
                                "text-muted-foreground line-through",
                            )}
                          >
                            {task.body}
                          </p>
                          <ItemMeta
                            now={now}
                            item={task}
                            coach={directory.memberName(task.coach_uuid)}
                          />
                        </div>
                      </div>
                    ))}
                    <Button
                      size="sm"
                      variant="ghost"
                      className="justify-start"
                      onClick={() =>
                        setCreating({ kind: "task", parent: g.uuid })
                      }
                    >
                      <ListTodo className="size-4" />
                      {t("coaching.items.add_task")}
                    </Button>
                  </div>
                </div>
              ))}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <NotebookPen className="size-4" />
                {t("coaching.items.notes")}
              </CardTitle>
            </CardHeader>
            <CardContent className="grid gap-3">
              {grouped.notes.length === 0 && (
                <p className="text-muted-foreground text-sm">
                  {t("coaching.items.no_notes")}
                </p>
              )}
              {grouped.notes.map((n) => (
                <div key={n.uuid} className="grid gap-1 rounded-md border p-3">
                  <p className="text-sm break-words whitespace-pre-wrap">
                    {n.body}
                  </p>
                  <ItemMeta
                    now={now}
                    item={n}
                    coach={directory.memberName(n.coach_uuid)}
                  />
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      )}
      {agent && (list.data?.total ?? 0) > LIMIT && (
        <p className="text-muted-foreground text-xs">
          {t("coaching.items.truncated", { count: LIMIT })}
        </p>
      )}

      {creating && agent && (
        <CoachingItemDialog
          agent={agent}
          goals={grouped.goals.filter((g) => !g.completed_at)}
          defaultKind={creating.kind}
          defaultParent={creating.parent}
          pending={create.isPending}
          onOpenChange={(open) => !open && setCreating(null)}
          onSubmit={(body) => create.mutate(body)}
        />
      )}
    </div>
  );
}

function ItemMeta({
  item,
  coach,
  now,
}: {
  item: CoachingItem;
  coach: string;
  now: number;
}) {
  const { t, locale } = useLocale();
  const overdue =
    !item.completed_at &&
    item.due_at !== null &&
    new Date(item.due_at).getTime() < now;
  return (
    <p className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      <span>
        {coach} · {formatDate(item.created_at, locale)}
      </span>
      {item.due_at && (
        <Badge variant={overdue ? "danger" : "outline"}>
          {overdue
            ? t("coaching.items.overdue", {
                date: formatDate(item.due_at, locale),
              })
            : t("coaching.items.due", {
                date: formatDate(item.due_at, locale),
              })}
        </Badge>
      )}
      {item.completed_at && (
        <Badge variant="success">
          {t("coaching.items.completed", {
            date: formatDate(item.completed_at, locale),
          })}
        </Badge>
      )}
    </p>
  );
}
