"use client";

import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { useLocale } from "@/providers/locale-provider";

import {
  browserTimezone,
  CHART_TYPES,
  DIMENSIONS,
  METRICS,
} from "../lib/format";
import type {
  CustomReport,
  CustomReportInput,
  ReportChartType,
  ReportDimension,
  ReportMeta,
  ReportMetric,
} from "../services/reports.service";
import { OptionalSelect, useChannelOptions } from "./report-filters";

const NONE = "__none__";

/**
 * Report builder: one optional dimension (the API allows at most one),
 * metrics, filters (channel, team, agent), chart type and the zone that
 * buckets days.
 */
export function ReportDialog({
  open,
  report,
  meta,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  report: CustomReport | null;
  meta?: ReportMeta;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: CustomReportInput) => void;
}) {
  const { t } = useLocale();
  const directory = useMemberDirectory();
  const channels = useChannelOptions();
  const def = report?.definition;
  const [name, setName] = useState(report?.name ?? "");
  const [dimension, setDimension] = useState<ReportDimension | typeof NONE>(
    def?.dimensions[0] ?? "agent",
  );
  const [metrics, setMetrics] = useState<ReportMetric[]>(
    def?.metrics ?? ["volume", "first_response_seconds", "csat_average"],
  );
  const [chartType, setChartType] = useState<ReportChartType>(
    def?.chart_type ?? "bar",
  );
  const [channel, setChannel] = useState(def?.filters.channel ?? "");
  const [team, setTeam] = useState(def?.filters.team ?? "");
  const [agent, setAgent] = useState(def?.filters.agent ?? "");
  const [timezone, setTimezone] = useState(def?.timezone ?? browserTimezone());

  const allMetrics = meta?.metrics ?? METRICS;
  const valid =
    name.trim().length > 0 &&
    name.trim().length <= 120 &&
    metrics.length > 0 &&
    timezone.trim().length > 0;

  const submit = () =>
    onSubmit({
      name: name.trim(),
      definition: {
        dimensions: dimension === NONE ? [] : [dimension],
        // Keep the allowlist order so columns stay stable.
        metrics: allMetrics.filter((m) => metrics.includes(m)),
        chart_type: chartType,
        timezone: timezone.trim(),
        filters: {
          ...(channel ? { channel } : {}),
          ...(team ? { team } : {}),
          ...(agent ? { agent } : {}),
        },
      },
    });

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t(report ? "reports.custom.edit" : "reports.custom.new")}
      description={t("reports.custom.form_hint")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="report-name">{t("reports.fields.name")}</Label>
          <Input
            id="report-name"
            value={name}
            maxLength={120}
            onChange={(e) => setName(e.target.value)}
          />
        </div>

        <div className="grid gap-4 sm:grid-cols-3">
          <div className="grid gap-1.5">
            <Label htmlFor="report-dimension">
              {t("reports.fields.dimension")}
            </Label>
            <Select
              value={dimension}
              onValueChange={(v) => setDimension(v as ReportDimension)}
            >
              <SelectTrigger id="report-dimension" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>
                  {t("reports.dimension.none")}
                </SelectItem>
                {(meta?.dimensions ?? DIMENSIONS).map((d) => (
                  <SelectItem key={d} value={d}>
                    {t(`reports.dimension.${d}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="report-chart">
              {t("reports.fields.chart_type")}
            </Label>
            <Select
              value={chartType}
              onValueChange={(v) => setChartType(v as ReportChartType)}
            >
              <SelectTrigger id="report-chart" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(meta?.chart_types ?? CHART_TYPES).map((c) => (
                  <SelectItem key={c} value={c}>
                    {t(`reports.chart.${c}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="report-tz">{t("reports.fields.timezone")}</Label>
            <Input
              id="report-tz"
              value={timezone}
              placeholder="Europe/Istanbul"
              onChange={(e) => setTimezone(e.target.value)}
            />
          </div>
        </div>

        <fieldset className="grid gap-2">
          <legend className="mb-1 text-sm font-medium">
            {t("reports.fields.metrics")}
          </legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {allMetrics.map((m) => (
              <label
                key={m}
                className="flex items-center gap-2 text-sm"
                htmlFor={`metric-${m}`}
              >
                <Checkbox
                  id={`metric-${m}`}
                  checked={metrics.includes(m)}
                  onCheckedChange={(on) =>
                    setMetrics((list) =>
                      on ? [...list, m] : list.filter((x) => x !== m),
                    )
                  }
                />
                {t(`reports.metric.${m}`)}
              </label>
            ))}
          </div>
        </fieldset>

        <fieldset className="grid gap-2">
          <legend className="mb-1 text-sm font-medium">
            {t("reports.fields.filters")}
          </legend>
          <div className="grid gap-3 sm:grid-cols-3">
            <OptionalSelect
              id="report-filter-channel"
              label={t("reports.filters.channel")}
              value={channel}
              options={channels}
              anyLabel={t("reports.filters.all_channels")}
              onChange={setChannel}
              className="w-full"
            />
            <OptionalSelect
              id="report-filter-team"
              label={t("reports.filters.team")}
              value={team}
              options={directory.teams}
              anyLabel={t("reports.filters.all_teams")}
              onChange={setTeam}
              className="w-full"
            />
            <OptionalSelect
              id="report-filter-agent"
              label={t("reports.filters.agent")}
              value={agent}
              options={directory.members}
              anyLabel={t("reports.filters.all_agents")}
              onChange={setAgent}
              className="w-full"
            />
          </div>
        </fieldset>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button disabled={pending || !valid} onClick={submit}>
          {t("common.save")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
