"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Download } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { AppChart, buildChartConfig } from "@/components/charts";
import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
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

import { agentTimes, rangeValid, type AgentTime } from "../lib/timesheets";
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

  const columns = useMemo(
    () =>
      [
        createColumn<AgentTime>({
          id: "agent",
          accessorFn: (a) => directory.memberName(a.agent),
          labelKey: "workforce.filters.agent",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ getValue }) => (
            <span className="block max-w-[14rem] truncate font-medium">
              {getValue() as string}
            </span>
          ),
        }),
        createColumn<AgentTime>({
          accessorKey: "active",
          labelKey: "workforce.state.active",
          searchable: false,
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right tabular-nums",
          },
          cell: ({ row }) => formatSeconds(row.original.active, locale),
        }),
        createColumn<AgentTime>({
          accessorKey: "handling",
          labelKey: "workforce.state.handling",
          searchable: false,
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right tabular-nums",
          },
          cell: ({ row }) => formatSeconds(row.original.handling, locale),
        }),
        createColumn<AgentTime>({
          id: "working",
          accessorFn: (a) => a.active + a.handling,
          labelKey: "workforce.timesheets.working",
          searchable: false,
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right font-medium tabular-nums",
          },
          cell: ({ getValue }) => formatSeconds(getValue() as number, locale),
        }),
        createColumn<AgentTime>({
          accessorKey: "away",
          labelKey: "workforce.state.away",
          searchable: false,
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-muted-foreground text-right tabular-nums",
          },
          cell: ({ row }) => formatSeconds(row.original.away, locale),
        }),
      ] as ColumnDef<AgentTime, unknown>[],
    [directory, locale],
  );

  const renderConversations = (a: AgentTime) => (
    <ul className="space-y-1 ps-8 text-xs">
      {a.conversations.map((c, i) => (
        <li
          key={`${a.agent}-${c.uuid ?? "deleted"}-${i}`}
          className="flex items-center justify-between gap-3"
        >
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
          <span className="tabular-nums">
            {formatSeconds(c.seconds, locale)}
          </span>
        </li>
      ))}
    </ul>
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
      <EntityTable
        columns={columns}
        data={agents}
        getRowId={(a) => a.agent}
        isLoading={query.isLoading}
        isError={query.isError}
        errorTitle={t("workforce.error.title")}
        errorDescription={
          isApiError(query.error)
            ? query.error.message
            : t("workforce.error.description")
        }
        onRetry={() => void query.refetch()}
        emptyTitle={t("workforce.timesheets.empty")}
        emptyDescription=""
        renderSubRow={renderConversations}
        getRowCanExpand={(a) => a.conversations.length > 0}
        manual={{ filtering: false, sorting: false, pagination: false }}
        initialState={{
          pagination: { pageIndex: 0, pageSize: 20 },
          sorting: [{ id: "working", desc: true }],
        }}
        features={{
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
      <p className="text-muted-foreground text-xs">
        {t("workforce.timesheets.hint")}
      </p>
    </div>
  );
}
