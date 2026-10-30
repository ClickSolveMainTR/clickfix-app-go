"use client";

import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import {
  AppChart,
  buildChartConfig,
  type ChartType,
} from "@/components/charts";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatMetric, toNumber } from "../lib/format";
import type {
  ReportChartType,
  ReportDimension,
  ReportMetric,
  ReportRow,
} from "../services/reports.service";

type Sort = { key: string; desc: boolean };

/** Display label of one `dimension` value (agent/team uuid, channel, day). */
export function useDimensionLabel(dimension: ReportDimension | null) {
  const { t } = useLocale();
  const directory = useMemberDirectory();
  return useCallback(
    (value: unknown) => {
      if (value === null || value === undefined || value === "")
        return t("reports.result.unassigned");
      const raw = String(value);
      switch (dimension) {
        case "agent":
          return directory.memberName(raw);
        case "team":
          return directory.teamName(raw);
        case "channel":
          return t(`sla.channel.${raw}`) === `sla.channel.${raw}`
            ? raw
            : t(`sla.channel.${raw}`);
        case "day":
          return raw;
        default:
          return t("reports.result.all");
      }
    },
    [dimension, directory, t],
  );
}

/** Sortable table of report rows; one column per metric. */
export function ReportTable({
  rows,
  dimension,
  metrics,
  className,
}: {
  rows: ReportRow[];
  dimension: ReportDimension | null;
  metrics: ReportMetric[];
  className?: string;
}) {
  const { t, locale } = useLocale();
  const label = useDimensionLabel(dimension);
  const [sort, setSort] = useState<Sort>({
    key: dimension === "day" ? "dimension" : (metrics[0] ?? "dimension"),
    desc: dimension !== "day",
  });

  const sorted = useMemo(() => {
    const list = [...rows];
    list.sort((a, b) => {
      let cmp: number;
      if (sort.key === "dimension") {
        cmp = label(a.dimension).localeCompare(label(b.dimension), locale);
      } else {
        const x = toNumber(a[sort.key]);
        const y = toNumber(b[sort.key]);
        if (x === null && y === null) cmp = 0;
        else if (x === null) return 1;
        else if (y === null) return -1;
        else cmp = x - y;
      }
      return sort.desc ? -cmp : cmp;
    });
    return list;
  }, [rows, sort, label, locale]);

  const header = (key: string, text: string, numeric: boolean) => {
    const active = sort.key === key;
    const Icon = !active ? ArrowUpDown : sort.desc ? ArrowDown : ArrowUp;
    return (
      <th
        key={key}
        scope="col"
        aria-sort={active ? (sort.desc ? "descending" : "ascending") : "none"}
        className={cn("px-3 py-2 font-medium", numeric && "text-right")}
      >
        <button
          type="button"
          className={cn(
            "hover:text-foreground inline-flex items-center gap-1",
            numeric && "flex-row-reverse",
          )}
          onClick={() =>
            setSort((s) =>
              s.key === key ? { key, desc: !s.desc } : { key, desc: numeric },
            )
          }
        >
          {text}
          <Icon className="size-3 shrink-0" />
        </button>
      </th>
    );
  };

  return (
    <div className={cn("overflow-x-auto rounded-md border", className)}>
      <table className="w-full min-w-[36rem] text-sm">
        <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
          <tr>
            {header(
              "dimension",
              dimension
                ? t(`reports.dimension.${dimension}`)
                : t("reports.dimension.none"),
              false,
            )}
            {metrics.map((m) => header(m, t(`reports.metric.${m}`), true))}
          </tr>
        </thead>
        <tbody className="divide-y">
          {sorted.length === 0 && (
            <tr>
              <td
                colSpan={metrics.length + 1}
                className="text-muted-foreground px-3 py-8 text-center"
              >
                {t("reports.result.empty")}
              </td>
            </tr>
          )}
          {sorted.map((row, i) => (
            <tr key={`${String(row.dimension)}-${i}`}>
              <td className="max-w-[16rem] truncate px-3 py-2 font-medium">
                {label(row.dimension)}
              </td>
              {metrics.map((m) => (
                <td key={m} className="px-3 py-2 text-right tabular-nums">
                  {formatMetric(m, row[m], locale)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

const CHART: Record<Exclude<ReportChartType, "table">, ChartType> = {
  bar: "bar",
  line: "line",
  pie: "donut",
};

/** One metric of the rows as a bar, line or pie chart. */
export function ReportChart({
  rows,
  dimension,
  metric,
  chartType,
  height = 260,
  title,
  horizontal,
}: {
  rows: ReportRow[];
  dimension: ReportDimension | null;
  metric: ReportMetric;
  chartType: Exclude<ReportChartType, "table">;
  height?: number;
  title?: string;
  horizontal?: boolean;
}) {
  const { t, locale } = useLocale();
  const label = useDimensionLabel(dimension);
  const pie = chartType === "pie";
  const data = useMemo(
    () =>
      // Bars and slices show the 12 largest; lines keep the row order.
      (chartType === "line"
        ? rows.slice(0, 400)
        : [...rows]
            .sort(
              (a, b) => (toNumber(b[metric]) ?? 0) - (toNumber(a[metric]) ?? 0),
            )
            .slice(0, 12)
      ).map((row, i) => ({
        // Pie slices are colored by a config entry per category, and
        // config keys become CSS variables: use a stable slot id.
        label: pie ? `s${i}` : label(row.dimension),
        name: label(row.dimension),
        [metric]: toNumber(row[metric]) ?? 0,
      })),
    [rows, metric, label, chartType, pie],
  );
  const config = useMemo(
    () =>
      buildChartConfig([
        { key: metric, label: t(`reports.metric.${metric}`) },
        ...(pie ? data.map((d) => ({ key: d.label, label: d.name })) : []),
      ]),
    [metric, t, pie, data],
  );
  const names = useMemo(
    () => new Map(data.map((d) => [d.label, d.name])),
    [data],
  );
  const type: ChartType =
    horizontal && chartType === "bar" ? "bar-horizontal" : CHART[chartType];
  return (
    <AppChart
      type={type}
      data={data}
      config={config}
      categoryKey="label"
      series={[metric]}
      height={height}
      title={title}
      showLegend={pie}
      categoryFormatter={pie ? (v) => names.get(v) ?? v : undefined}
      valueFormatter={(v) => formatMetric(metric, v, locale)}
      emptyTitle={t("reports.result.empty")}
    />
  );
}
