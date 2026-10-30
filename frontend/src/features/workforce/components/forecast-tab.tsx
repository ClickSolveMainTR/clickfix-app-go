"use client";

import { useQuery } from "@tanstack/react-query";
import { format } from "date-fns";
import { AlertTriangle, Info } from "lucide-react";
import { useMemo, useState } from "react";

import { AppChart, buildChartConfig } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  OptionalSelect,
  useChannelOptions,
} from "@/features/reports/components/report-filters";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  bucketLabel,
  forecastRows,
  forecastTotals,
  forecastWindow,
  formatNumber,
  HORIZONS,
  type ForecastRow,
} from "../lib/forecast";
import {
  workforceService,
  type ForecastInput,
} from "../services/workforce.service";

type Granularity = "hour" | "day";

type Filters = {
  day: string;
  days: number;
  granularity: Granularity;
  channel: string;
  team: string;
  /** Model parameters as typed (validated on apply). */
  handle: string;
  wait: string;
  target: string;
  alpha: string;
};

const DEFAULTS: Omit<Filters, "day"> = {
  days: 7,
  granularity: "hour",
  channel: "",
  team: "",
  handle: "300",
  wait: "30",
  target: "80",
  alpha: "0.3",
};

function inRange(raw: string, min: number, max: number, open = false) {
  const n = Number(raw);
  if (raw.trim() === "" || !Number.isFinite(n)) return false;
  return open ? n > min && n < max : n >= min && n <= max;
}

/**
 * Volume forecast and staffing (CF-107): weekday × hour EWMA demand, Erlang C
 * required agents and scheduled shift coverage, with the hours (or days)
 * that are short of staff highlighted.
 */
export function ForecastTab({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const directory = useMemberDirectory();
  const channels = useChannelOptions();
  const [draft, setDraft] = useState<Filters>(() => ({
    day: format(new Date(), "yyyy-MM-dd"),
    ...DEFAULTS,
  }));
  const [applied, setApplied] = useState<Filters>(draft);
  const [onlyGaps, setOnlyGaps] = useState(false);

  const paramsValid =
    inRange(draft.handle, 1, 86400) &&
    inRange(draft.wait, 1, 86400) &&
    inRange(draft.target, 0, 100, true) &&
    inRange(draft.alpha, 0, 1) &&
    Number(draft.alpha) > 0;
  const draftWindow = forecastWindow(draft.day, draft.days);
  const canApply = paramsValid && draftWindow.valid;

  const body = useMemo<ForecastInput | null>(() => {
    const w = forecastWindow(applied.day, applied.days);
    if (!w.valid) return null;
    return {
      since: w.since,
      until: w.until,
      granularity: applied.granularity,
      channel: applied.channel || undefined,
      team_uuid: applied.team || undefined,
      handle_seconds: Number(applied.handle),
      sla_seconds: Number(applied.wait),
      target_sla: Number(applied.target) / 100,
      alpha: Number(applied.alpha),
    };
  }, [applied]);

  const query = useQuery({
    queryKey: ["tenant", slug, "workforce", "forecast", body],
    queryFn: () => workforceService.forecast(slug, body!),
    enabled: body !== null,
  });

  const granularity = applied.granularity;
  const rows = useMemo(
    () => (query.data ? forecastRows(query.data, granularity) : []),
    [query.data, granularity],
  );
  const totals = useMemo(() => forecastTotals(rows), [rows]);
  const timezone = query.data?.timezone ?? "UTC";
  const unit =
    granularity === "day"
      ? t("workforce.forecast.unit_agent_hours")
      : t("workforce.forecast.unit_agents");

  const set = <K extends keyof Filters>(key: K, value: Filters[K]) =>
    setDraft((d) => ({ ...d, [key]: value }));

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="grid gap-4 py-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="grid gap-1.5">
              <Label className="text-xs">{t("workforce.forecast.start")}</Label>
              <DatePicker
                value={draft.day}
                onChange={(v) => set("day", v)}
                className="w-full sm:w-40"
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="forecast-days" className="text-xs">
                {t("workforce.forecast.horizon")}
              </Label>
              <Select
                value={String(draft.days)}
                onValueChange={(v) => set("days", Number(v))}
              >
                <SelectTrigger id="forecast-days" className="w-full sm:w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {HORIZONS.map((d) => (
                    <SelectItem key={d} value={String(d)}>
                      {t("workforce.forecast.days", { count: d })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="forecast-granularity" className="text-xs">
                {t("workforce.forecast.granularity")}
              </Label>
              <Select
                value={draft.granularity}
                onValueChange={(v) => set("granularity", v as Granularity)}
              >
                <SelectTrigger
                  id="forecast-granularity"
                  className="w-full sm:w-32"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="hour">
                    {t("workforce.forecast.hourly")}
                  </SelectItem>
                  <SelectItem value="day">
                    {t("workforce.forecast.daily")}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            <OptionalSelect
              id="forecast-channel"
              label={t("workforce.filters.channel")}
              value={draft.channel}
              options={channels}
              anyLabel={t("workforce.filters.all_channels")}
              onChange={(v) => set("channel", v)}
            />
            <OptionalSelect
              id="forecast-team"
              label={t("workforce.filters.team")}
              value={draft.team}
              options={directory.teams}
              anyLabel={t("workforce.filters.all_teams")}
              onChange={(v) => set("team", v)}
            />
          </div>
          <div className="flex flex-wrap items-end gap-3">
            {(
              [
                ["handle", "workforce.forecast.handle_seconds", 1, 86400, 1],
                ["wait", "workforce.forecast.sla_seconds", 1, 86400, 1],
                ["target", "workforce.forecast.target_sla", 1, 99, 1],
                ["alpha", "workforce.forecast.alpha", 0.05, 1, 0.05],
              ] as const
            ).map(([key, label, min, max, step]) => (
              <div key={key} className="grid gap-1.5">
                <Label htmlFor={`forecast-${key}`} className="text-xs">
                  {t(label)}
                </Label>
                <Input
                  id={`forecast-${key}`}
                  type="number"
                  inputMode="decimal"
                  min={min}
                  max={max}
                  step={step}
                  value={draft[key]}
                  onChange={(e) => set(key, e.target.value)}
                  className="w-full sm:w-32"
                />
              </div>
            ))}
            <Button disabled={!canApply} onClick={() => setApplied(draft)}>
              {t("workforce.forecast.apply")}
            </Button>
          </div>
          {!draftWindow.valid ? (
            <p className="text-destructive text-xs">
              {t("workforce.forecast.invalid_window")}
            </p>
          ) : !paramsValid ? (
            <p className="text-destructive text-xs">
              {t("workforce.forecast.invalid_params")}
            </p>
          ) : (
            <p className="text-muted-foreground text-xs">
              {t("workforce.forecast.params_hint")}
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
      ) : query.data ? (
        <>
          {query.data.history_conversations === 0 && (
            <Alert>
              <Info className="size-4" />
              <AlertTitle>
                {t("workforce.forecast.no_history_title")}
              </AlertTitle>
              <AlertDescription>
                {t("workforce.forecast.no_history")}
              </AlertDescription>
            </Alert>
          )}
          {totals.capacityLimited && (
            <Alert variant="destructive">
              <AlertTriangle className="size-4" />
              <AlertTitle>{t("workforce.forecast.capacity_title")}</AlertTitle>
              <AlertDescription>
                {t("workforce.forecast.capacity_limited")}
              </AlertDescription>
            </Alert>
          )}

          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatsCard
              title={t("workforce.forecast.total_volume")}
              value={formatNumber(totals.volume, locale, 0)}
              hint={t("workforce.forecast.history", {
                count: query.data.history_conversations,
              })}
            />
            <StatsCard
              title={
                granularity === "day"
                  ? t("workforce.forecast.peak_required_day")
                  : t("workforce.forecast.peak_required")
              }
              value={formatNumber(totals.peakRequired, locale, 0)}
              hint={unit}
            />
            <StatsCard
              title={
                granularity === "day"
                  ? t("workforce.forecast.gap_days")
                  : t("workforce.forecast.gap_hours")
              }
              value={
                <span
                  className={cn(totals.gapBuckets > 0 && "text-destructive")}
                >
                  {formatNumber(totals.gapBuckets, locale, 0)}
                  <span className="text-muted-foreground text-sm font-normal">
                    {" "}
                    / {rows.length}
                  </span>
                </span>
              }
            />
            <StatsCard
              title={t("workforce.forecast.total_gap")}
              value={
                <span className={cn(totals.gap > 0 && "text-destructive")}>
                  {formatNumber(totals.gap, locale, 0)}
                </span>
              }
              hint={t("workforce.forecast.unit_agent_hours")}
            />
          </div>

          <ForecastCharts
            rows={rows}
            granularity={granularity}
            timezone={timezone}
          />

          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-muted-foreground text-xs">
              {t("workforce.forecast.timezone", { zone: timezone })}
            </p>
            <div className="flex items-center gap-2">
              <Switch
                id="forecast-only-gaps"
                checked={onlyGaps}
                onCheckedChange={setOnlyGaps}
              />
              <Label htmlFor="forecast-only-gaps" className="text-sm">
                {t("workforce.forecast.only_gaps")}
              </Label>
            </div>
          </div>
          <ForecastTable
            rows={onlyGaps ? rows.filter((r) => r.gap > 0) : rows}
            granularity={granularity}
            timezone={timezone}
          />
          <p className="text-muted-foreground text-xs">
            {t("workforce.forecast.model_hint")}
          </p>
        </>
      ) : null}
    </div>
  );
}

function ForecastCharts({
  rows,
  granularity,
  timezone,
}: {
  rows: ForecastRow[];
  granularity: Granularity;
  timezone: string;
}) {
  const { t, locale } = useLocale();
  const data = useMemo(
    () =>
      rows.map((r) => ({
        label: bucketLabel(r, granularity, timezone, locale, true),
        volume: Math.round(r.volume * 10) / 10,
        required: r.required,
        // Stacked: scheduled coverage up to the requirement, then the gap.
        covered: Math.min(r.scheduled, r.required),
        surplus: Math.max(r.scheduled - r.required, 0),
        gap: r.gap,
      })),
    [rows, granularity, timezone, locale],
  );
  const volumeConfig = useMemo(
    () =>
      buildChartConfig([
        { key: "volume", label: t("workforce.forecast.volume") },
      ]),
    [t],
  );
  const staffConfig = useMemo(
    () =>
      buildChartConfig([
        { key: "covered", label: t("workforce.forecast.covered") },
        {
          key: "surplus",
          label: t("workforce.forecast.surplus"),
          color: "var(--chart-3)",
        },
        {
          key: "gap",
          label: t("workforce.forecast.gap"),
          color: "var(--destructive)",
        },
        {
          key: "required",
          label: t("workforce.forecast.required"),
          color: "var(--chart-1)",
        },
      ]),
    [t],
  );
  const nf = (v: number) => formatNumber(v, locale);

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <AppChart
        type="area"
        data={data}
        config={volumeConfig}
        series={["volume"]}
        curved
        height={240}
        title={t("workforce.forecast.volume_chart")}
        description={
          granularity === "day"
            ? t("workforce.forecast.volume_chart_daily")
            : t("workforce.forecast.volume_chart_hourly")
        }
        valueFormatter={nf}
        emptyTitle={t("workforce.forecast.empty")}
      />
      <AppChart
        type="composed"
        data={data}
        config={staffConfig}
        series={[
          { key: "covered", type: "bar", stackId: "staff" },
          { key: "surplus", type: "bar", stackId: "staff" },
          { key: "gap", type: "bar", stackId: "staff" },
          { key: "required", type: "line" },
        ]}
        height={240}
        showLegend
        title={t("workforce.forecast.staffing_chart")}
        description={t("workforce.forecast.staffing_chart_hint")}
        valueFormatter={nf}
        emptyTitle={t("workforce.forecast.empty")}
      />
    </div>
  );
}

function ForecastTable({
  rows,
  granularity,
  timezone,
}: {
  rows: ForecastRow[];
  granularity: Granularity;
  timezone: string;
}) {
  const { t, locale } = useLocale();
  return (
    <div className="max-h-[32rem] overflow-auto rounded-md border">
      <table className="w-full min-w-[34rem] text-sm">
        <thead className="bg-muted/50 text-muted-foreground sticky top-0 text-left text-xs backdrop-blur">
          <tr>
            <th scope="col" className="px-3 py-2 font-medium">
              {granularity === "day"
                ? t("workforce.forecast.col_day")
                : t("workforce.forecast.col_hour")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-medium">
              {t("workforce.forecast.volume")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-medium">
              {t("workforce.forecast.required")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-medium">
              {t("workforce.forecast.scheduled")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-medium">
              {t("workforce.forecast.gap")}
            </th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {rows.length === 0 && (
            <tr>
              <td
                colSpan={5}
                className="text-muted-foreground px-3 py-8 text-center"
              >
                {t("workforce.forecast.no_gaps")}
              </td>
            </tr>
          )}
          {rows.map((r) => (
            <tr key={r.key} className={cn(r.gap > 0 && "bg-destructive/5")}>
              <td className="px-3 py-2 whitespace-nowrap">
                {bucketLabel(r, granularity, timezone, locale, true)}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {formatNumber(r.volume, locale)}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {r.capacityLimited ? "≥ " : ""}
                {formatNumber(r.required, locale, 0)}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {formatNumber(r.scheduled, locale, 0)}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {r.gap > 0 ? (
                  <Badge variant="danger">
                    −{formatNumber(r.gap, locale, 0)}
                  </Badge>
                ) : (
                  <span className="text-muted-foreground">0</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
