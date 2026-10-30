"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Card, CardContent } from "@/components/ui/card";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { useLocale } from "@/providers/locale-provider";

import { defaultRange, METRICS, windowFromDays } from "../lib/format";
import { reportsService } from "../services/reports.service";
import {
  DateRangeFields,
  OptionalSelect,
  useChannelOptions,
} from "./report-filters";
import { ReportChart, ReportTable } from "./report-result";

/**
 * Per-agent volume, first response, resolution, CSAT, QA and SLA breaches
 * over a date range, filtered by team and channel.
 */
export function ScorecardsTab({ slug }: { slug: string }) {
  const { t } = useLocale();
  const directory = useMemberDirectory();
  const channels = useChannelOptions();
  const [range, setRange] = useState(defaultRange);
  const [team, setTeam] = useState("");
  const [channel, setChannel] = useState("");
  const valid = Boolean(range.from && range.to && range.from <= range.to);

  const query = useQuery({
    queryKey: ["tenant", slug, "reports", "scorecards", range, team, channel],
    queryFn: () =>
      reportsService.scorecards(slug, {
        ...windowFromDays(range.from, range.to),
        team: team || undefined,
        channel: channel || undefined,
      }),
    enabled: valid,
  });
  const rows = query.data?.rows ?? [];

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 py-4">
          <DateRangeFields
            from={range.from}
            to={range.to}
            onChange={setRange}
          />
          <OptionalSelect
            id="scorecard-team"
            label={t("reports.filters.team")}
            value={team}
            options={directory.teams}
            anyLabel={t("reports.filters.all_teams")}
            onChange={setTeam}
          />
          <OptionalSelect
            id="scorecard-channel"
            label={t("reports.filters.channel")}
            value={channel}
            options={channels}
            anyLabel={t("reports.filters.all_channels")}
            onChange={setChannel}
          />
          {!valid && (
            <p className="text-destructive text-xs">
              {t("reports.filters.invalid_range")}
            </p>
          )}
        </CardContent>
      </Card>

      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError ? (
        <ErrorState
          title={t("reports.error.title")}
          description={t("reports.error.description")}
          onRetry={() => void query.refetch()}
        />
      ) : (
        <>
          {rows.length > 0 && (
            <div className="grid gap-4 lg:grid-cols-2">
              <ReportChart
                rows={rows}
                dimension="agent"
                metric="volume"
                chartType="bar"
                horizontal
                height={220}
                title={t("reports.metric.volume")}
              />
              <ReportChart
                rows={rows}
                dimension="agent"
                metric="first_response_seconds"
                chartType="bar"
                horizontal
                height={220}
                title={t("reports.metric.first_response_seconds")}
              />
            </div>
          )}
          <ReportTable rows={rows} dimension="agent" metrics={METRICS} />
          <p className="text-muted-foreground text-xs">
            {t("reports.scorecards.hint")}
          </p>
        </>
      )}
    </div>
  );
}
