"use client";

import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, Download } from "lucide-react";
import Link from "next/link";
import { Fragment, useMemo, useState } from "react";

import { AppChart, buildChartConfig } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  DateRangeFields,
  OptionalSelect,
} from "@/features/reports/components/report-filters";
import {
  defaultRange,
  formatSeconds,
  windowFromDays,
} from "@/features/reports/lib/format";
import {
  shortId,
  useMemberDirectory,
} from "@/features/tenant-members/hooks/use-member-directory";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { agentTimes, rangeValid } from "../lib/timesheets";
import {
  workforceService,
  type TimesheetFormat,
} from "../services/workforce.service";

const FORMATS: TimesheetFormat[] = ["csv", "xlsx", "json"];

/**
 * Per-agent active, handling and away time from activity heartbeats
 * (CF-107), with a per-conversation breakdown and CSV/XLSX/JSON export.
 * Needs tenant.schedules.manage.
 */
export function TimesheetsTab({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const directory = useMemberDirectory();
  const [range, setRange] = useState(() => defaultRange(7));
  const [agent, setAgent] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const valid = rangeValid(range.from, range.to);
  const span = useMemo(
    () => (valid ? windowFromDays(range.from, range.to) : null),
    [valid, range.from, range.to],
  );
  const params = span
    ? { since: span.from, until: span.to, agent: agent || undefined }
    : null;

  const query = useQuery({
    queryKey: ["tenant", slug, "workforce", "timesheets", params],
    queryFn: () => workforceService.timesheets(slug, params!),
    enabled: params !== null,
  });
  const agents = useMemo(() => agentTimes(query.data ?? []), [query.data]);

  const hours = (s: number) => Math.round((s / 3600) * 100) / 100;
  const chartData = useMemo(
    () =>
      agents.slice(0, 15).map((a) => ({
        label: directory.memberName(a.agent),
        active: hours(a.active),
        handling: hours(a.handling),
        away: hours(a.away),
      })),
    [agents, directory],
  );
  const config = useMemo(
    () =>
      buildChartConfig([
        { key: "active", label: t("workforce.state.active") },
        { key: "handling", label: t("workforce.state.handling") },
        { key: "away", label: t("workforce.state.away") },
      ]),
    [t],
  );

  const download = async (format: TimesheetFormat) => {
    if (!params) return;
    setExporting(true);
    try {
      const { blob, filename } = await workforceService.exportTimesheets(slug, {
        ...params,
        format,
      });
      const href = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = href;
      a.download = filename ?? `timesheets.${format}`;
      a.click();
      URL.revokeObjectURL(href);
    } catch {
      appToast.error(t("workforce.timesheets.export_failed"));
    } finally {
      setExporting(false);
    }
  };

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
            id="timesheet-agent"
            label={t("workforce.filters.agent")}
            value={agent}
            options={directory.members}
            anyLabel={t("workforce.filters.all_agents")}
            onChange={setAgent}
          />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                disabled={!params || exporting}
                className="sm:ml-auto"
              >
                <Download className="size-4" />
                {t("workforce.timesheets.export")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {FORMATS.map((f) => (
                <DropdownMenuItem key={f} onSelect={() => void download(f)}>
                  {f.toUpperCase()}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          {!valid && (
            <p className="text-destructive w-full text-xs">
              {t("workforce.timesheets.invalid_range")}
            </p>
          )}
        </CardContent>
      </Card>

      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError ? (
        <ErrorState
          title={t("workforce.error.title")}
          description={
            isApiError(query.error)
              ? query.error.message
              : t("workforce.error.description")
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
              series={["active", "handling", "away"]}
              stacked
              showLegend
              height={Math.max(160, chartData.length * 36)}
              title={t("workforce.timesheets.chart")}
              valueFormatter={(v) => formatSeconds(v * 3600, locale)}
            />
          )}
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full min-w-[36rem] text-sm">
              <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
                <tr>
                  <th scope="col" className="px-3 py-2 font-medium">
                    {t("workforce.filters.agent")}
                  </th>
                  <th scope="col" className="px-3 py-2 text-right font-medium">
                    {t("workforce.state.active")}
                  </th>
                  <th scope="col" className="px-3 py-2 text-right font-medium">
                    {t("workforce.state.handling")}
                  </th>
                  <th scope="col" className="px-3 py-2 text-right font-medium">
                    {t("workforce.timesheets.working")}
                  </th>
                  <th scope="col" className="px-3 py-2 text-right font-medium">
                    {t("workforce.state.away")}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {agents.length === 0 && (
                  <tr>
                    <td
                      colSpan={5}
                      className="text-muted-foreground px-3 py-8 text-center"
                    >
                      {t("workforce.timesheets.empty")}
                    </td>
                  </tr>
                )}
                {agents.map((a) => {
                  const expanded = open === a.agent;
                  const Icon = expanded ? ChevronDown : ChevronRight;
                  return (
                    <Fragment key={a.agent}>
                      <tr>
                        <td className="px-3 py-2 font-medium">
                          <button
                            type="button"
                            className="hover:text-primary inline-flex items-center gap-1 text-left disabled:cursor-default"
                            disabled={a.conversations.length === 0}
                            aria-expanded={expanded}
                            onClick={() => setOpen(expanded ? null : a.agent)}
                          >
                            <Icon
                              className={
                                a.conversations.length === 0
                                  ? "size-4 opacity-0"
                                  : "size-4"
                              }
                            />
                            <span className="max-w-[14rem] truncate">
                              {directory.memberName(a.agent)}
                            </span>
                          </button>
                        </td>
                        <td className="px-3 py-2 text-right tabular-nums">
                          {formatSeconds(a.active, locale)}
                        </td>
                        <td className="px-3 py-2 text-right tabular-nums">
                          {formatSeconds(a.handling, locale)}
                        </td>
                        <td className="px-3 py-2 text-right font-medium tabular-nums">
                          {formatSeconds(a.active + a.handling, locale)}
                        </td>
                        <td className="text-muted-foreground px-3 py-2 text-right tabular-nums">
                          {formatSeconds(a.away, locale)}
                        </td>
                      </tr>
                      {expanded &&
                        a.conversations.map((c, i) => (
                          <tr
                            key={`${a.agent}-${c.uuid ?? "deleted"}-${i}`}
                            className="bg-muted/30 text-xs"
                          >
                            <td className="py-1.5 pr-3 pl-10">
                              {c.uuid ? (
                                <Link
                                  href={`/inbox?c=${c.uuid}`}
                                  className="text-primary hover:underline"
                                >
                                  {t("workforce.timesheets.conversation", {
                                    id: shortId(c.uuid),
                                  })}
                                </Link>
                              ) : (
                                <span className="text-muted-foreground">
                                  {t("workforce.timesheets.deleted")}
                                </span>
                              )}
                            </td>
                            <td />
                            <td className="px-3 py-1.5 text-right tabular-nums">
                              {formatSeconds(c.seconds, locale)}
                            </td>
                            <td colSpan={2} />
                          </tr>
                        ))}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
          <p className="text-muted-foreground text-xs">
            {t("workforce.timesheets.hint")}
          </p>
        </>
      )}
    </div>
  );
}
