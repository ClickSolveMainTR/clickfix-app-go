"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Medal } from "lucide-react";
import { useMemo, useState } from "react";

import { AppChart, buildChartConfig } from "@/components/charts";
import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  DateRangeFields,
  OptionalSelect,
} from "@/features/reports/components/report-filters";
import { defaultRange, windowFromDays } from "@/features/reports/lib/format";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { formatScore } from "../lib/format";
import { coachingService, type Standing } from "../services/coaching.service";

const MEDAL = ["text-amber-500", "text-slate-400", "text-orange-700"];

/**
 * Team leaderboard (CF-109): points and badges earned in a window from the
 * enabled rules, optionally for one team, plus the viewer's own opt-out.
 */
export function LeaderboardTab({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const directory = useMemberDirectory();
  const [range, setRange] = useState(() => defaultRange(30));
  const [team, setTeam] = useState("");
  const listState = useServerListState({ initialPageSize: 50 });
  const { limit, offset } = listState.params;
  const valid = Boolean(range.from && range.to && range.from <= range.to);
  const span = valid ? windowFromDays(range.from, range.to) : null;

  const query = useQuery({
    queryKey: [
      "tenant",
      slug,
      "gamification",
      "leaderboard",
      span,
      team,
      limit,
      offset,
    ],
    queryFn: () =>
      coachingService.leaderboard(slug, {
        since: span!.from,
        until: span!.to,
        team: team || undefined,
        limit,
        offset,
      }),
    enabled: span !== null,
    placeholderData: keepPreviousData,
  });
  const items = useMemo(() => query.data?.items ?? [], [query.data]);

  const chartData = useMemo(
    () =>
      items
        .filter((s) => s.points > 0)
        .slice(0, 10)
        .map((s) => ({
          label: directory.memberName(s.agent_uuid),
          points: s.points,
        })),
    [items, directory],
  );
  const config = useMemo(
    () =>
      buildChartConfig([
        { key: "points", label: t("coaching.leaderboard.points") },
      ]),
    [t],
  );

  const resetPage =
    <T,>(set: (v: T) => void) =>
    (v: T) => {
      set(v);
      listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
    };

  const columns = useMemo(
    () =>
      [
        createColumn<Standing>({
          accessorKey: "rank",
          labelKey: "coaching.leaderboard.rank",
          enableHiding: false,
          meta: { headerClassName: "w-16", cellClassName: "w-16" },
          cell: ({ row }) => {
            const s = row.original;
            return (
              <span className="inline-flex items-center gap-1 font-medium tabular-nums">
                {s.rank <= 3 && s.points > 0 && (
                  <Medal
                    className={cn("size-4", MEDAL[s.rank - 1])}
                    aria-hidden
                  />
                )}
                {s.rank}
              </span>
            );
          },
        }),
        createColumn<Standing>({
          id: "agent",
          accessorFn: (s) => directory.memberName(s.agent_uuid),
          labelKey: "coaching.leaderboard.agent",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ getValue }) => (
            <span className="block max-w-[14rem] truncate">
              {getValue() as string}
            </span>
          ),
        }),
        createColumn<Standing>({
          id: "badges",
          labelKey: "coaching.leaderboard.badges",
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.badges.length === 0 && (
                <span className="text-muted-foreground">—</span>
              )}
              {row.original.badges.map((b) => (
                <Badge
                  key={b.uuid}
                  variant="secondary"
                  title={t("coaching.leaderboard.badge_points", {
                    points: b.points,
                  })}
                >
                  {b.name}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<Standing>({
          accessorKey: "points",
          labelKey: "coaching.leaderboard.points",
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right font-semibold tabular-nums",
          },
          cell: ({ row }) => formatScore(row.original.points, locale, 0),
        }),
      ] as ColumnDef<Standing, unknown>[],
    [directory, locale, t],
  );

  return (
    <div className="space-y-4">
      <OptOutCard slug={slug} />
      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 py-4">
          <DateRangeFields
            from={range.from}
            to={range.to}
            onChange={resetPage(setRange)}
          />
          <OptionalSelect
            id="leaderboard-team"
            label={t("coaching.filters.team")}
            value={team}
            options={directory.teams}
            anyLabel={t("coaching.filters.all_teams")}
            onChange={resetPage(setTeam)}
          />
          {!valid && (
            <p className="text-destructive w-full text-xs">
              {t("coaching.filters.invalid_range")}
            </p>
          )}
        </CardContent>
      </Card>

      {chartData.length > 0 && (
        <AppChart
          type="bar-horizontal"
          data={chartData}
          config={config}
          series={["points"]}
          height={Math.max(160, chartData.length * 32)}
          title={t("coaching.leaderboard.top")}
          valueFormatter={(v) => formatScore(v, locale, 0)}
        />
      )}
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(s) => s.agent_uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorTitle={t("coaching.error.title")}
        errorDescription={
          isApiError(query.error)
            ? query.error.message
            : t("coaching.error.description")
        }
        onRetry={() => void query.refetch()}
        emptyTitle={t("coaching.leaderboard.empty")}
        emptyDescription=""
        total={query.data?.total ?? 0}
        state={listState.tableState}
        features={{
          // Rank order is the content; the endpoint takes the window, team,
          // `limit` and `offset` only.
          toolbar: false,
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
      <p className="text-muted-foreground text-xs">
        {t("coaching.leaderboard.hint")}
      </p>
    </div>
  );
}

/** The viewer's own leaderboard visibility; nobody can change another's. */
function OptOutCard({ slug }: { slug: string }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const key = ["tenant", slug, "gamification", "preference"];
  const pref = useQuery({
    queryKey: key,
    queryFn: () => coachingService.preference(slug),
  });
  const save = useMutation({
    mutationFn: (optedOut: boolean) =>
      coachingService.setPreference(slug, optedOut),
    onSuccess: (next) => {
      queryClient.setQueryData(key, next);
      void queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "gamification", "leaderboard"],
      });
      appToast.success(
        next.opted_out
          ? t("coaching.optout.hidden")
          : t("coaching.optout.visible"),
      );
    },
  });
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center justify-between gap-3 py-4">
        <div className="grid gap-0.5">
          <Label htmlFor="gamification-optout">
            {t("coaching.optout.label")}
          </Label>
          <p className="text-muted-foreground text-xs">
            {t("coaching.optout.hint")}
          </p>
        </div>
        <Switch
          id="gamification-optout"
          checked={pref.data?.opted_out ?? false}
          disabled={pref.isLoading || pref.isError || save.isPending}
          onCheckedChange={(v) => save.mutate(v)}
        />
      </CardContent>
    </Card>
  );
}
