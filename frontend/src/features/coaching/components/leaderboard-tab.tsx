"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Medal } from "lucide-react";
import { useMemo, useState } from "react";

import { AppChart, buildChartConfig } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { coachingService } from "../services/coaching.service";

const PAGE = 50;
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
  const [offset, setOffset] = useState(0);
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
      offset,
    ],
    queryFn: () =>
      coachingService.leaderboard(slug, {
        since: span!.from,
        until: span!.to,
        team: team || undefined,
        limit: PAGE,
        offset,
      }),
    enabled: span !== null,
  });
  const items = useMemo(() => query.data?.items ?? [], [query.data]);
  const total = query.data?.total ?? 0;

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
      setOffset(0);
    };

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

      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError ? (
        <ErrorState
          title={t("coaching.error.title")}
          description={
            isApiError(query.error)
              ? query.error.message
              : t("coaching.error.description")
          }
          onRetry={() => void query.refetch()}
        />
      ) : (
        <>
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
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full min-w-[32rem] text-sm">
              <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
                <tr>
                  <th scope="col" className="w-16 px-3 py-2 font-medium">
                    {t("coaching.leaderboard.rank")}
                  </th>
                  <th scope="col" className="px-3 py-2 font-medium">
                    {t("coaching.leaderboard.agent")}
                  </th>
                  <th scope="col" className="px-3 py-2 font-medium">
                    {t("coaching.leaderboard.badges")}
                  </th>
                  <th scope="col" className="px-3 py-2 text-right font-medium">
                    {t("coaching.leaderboard.points")}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {items.length === 0 && (
                  <tr>
                    <td
                      colSpan={4}
                      className="text-muted-foreground px-3 py-8 text-center"
                    >
                      {t("coaching.leaderboard.empty")}
                    </td>
                  </tr>
                )}
                {items.map((s) => (
                  <tr key={s.agent_uuid}>
                    <td className="px-3 py-2 font-medium tabular-nums">
                      <span className="inline-flex items-center gap-1">
                        {s.rank <= 3 && s.points > 0 && (
                          <Medal
                            className={cn("size-4", MEDAL[s.rank - 1])}
                            aria-hidden
                          />
                        )}
                        {s.rank}
                      </span>
                    </td>
                    <td className="max-w-[14rem] truncate px-3 py-2">
                      {directory.memberName(s.agent_uuid)}
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex flex-wrap gap-1">
                        {s.badges.length === 0 && (
                          <span className="text-muted-foreground">—</span>
                        )}
                        {s.badges.map((b) => (
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
                    </td>
                    <td className="px-3 py-2 text-right font-semibold tabular-nums">
                      {formatScore(s.points, locale, 0)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {total > PAGE && (
            <div className="flex items-center justify-end gap-2 text-sm">
              <span className="text-muted-foreground">
                {t("coaching.pagination", {
                  from: offset + 1,
                  to: Math.min(offset + PAGE, total),
                  total,
                })}
              </span>
              <Button
                size="sm"
                variant="outline"
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - PAGE))}
              >
                {t("coaching.previous")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={offset + PAGE >= total}
                onClick={() => setOffset(offset + PAGE)}
              >
                {t("coaching.next")}
              </Button>
            </div>
          )}
          <p className="text-muted-foreground text-xs">
            {t("coaching.leaderboard.hint")}
          </p>
        </>
      )}
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
