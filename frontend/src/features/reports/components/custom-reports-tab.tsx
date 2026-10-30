"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileBarChart, Pencil, Play, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { defaultRange, windowFromDays } from "../lib/format";
import {
  reportsService,
  type CustomReport,
  type CustomReportInput,
  type ReportMetric,
} from "../services/reports.service";
import { useReportError } from "../lib/use-report-error";
import { DateRangeFields } from "./report-filters";
import { ReportDialog } from "./report-dialog";
import { ReportChart, ReportTable } from "./report-result";

/** Saved report definitions; pick one, choose a range and run it. */
export function CustomReportsTab({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useReportError();
  const key = ["tenant", slug, "reports", "custom"] as const;
  const reports = useQuery({
    queryKey: key,
    queryFn: () => reportsService.list(slug),
  });
  const meta = useQuery({
    queryKey: ["tenant", slug, "reports", "meta"],
    queryFn: () => reportsService.meta(slug),
    staleTime: Infinity,
  });
  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState<CustomReport | "new" | null>(null);
  const [deleting, setDeleting] = useState<CustomReport | null>(null);

  const save = useMutation({
    mutationFn: (body: CustomReportInput) =>
      editing && editing !== "new"
        ? reportsService.update(slug, editing.uuid, body)
        : reportsService.create(slug, body),
    onSuccess: async (saved) => {
      setEditing(null);
      setSelected(saved.uuid);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("reports.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => reportsService.remove(slug, uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: ["tenant", slug, "reports"] });
      appToast.success(t("reports.toast.deleted"));
    },
    onError,
  });

  if (reports.isLoading) return <Loading label={t("common.loading")} />;
  if (reports.isError)
    return (
      <ErrorState
        title={t("reports.error.title")}
        description={t("reports.error.description")}
        onRetry={() => void reports.refetch()}
      />
    );
  const items = reports.data ?? [];
  const current = items.find((r) => r.uuid === selected) ?? items[0] ?? null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {t("reports.custom.hint")}
        </p>
        {canWrite && (
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("reports.custom.new")}
          </Button>
        )}
      </div>

      {items.length === 0 ? (
        <EmptyState
          icon={<FileBarChart className="size-6" />}
          title={t("reports.custom.empty_title")}
          description={t("reports.custom.empty_description")}
        />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[18rem_1fr]">
          <ul className="divide-y self-start rounded-md border">
            {items.map((r) => (
              <li
                key={r.uuid}
                className={cn(
                  "flex items-center gap-2 p-2",
                  current?.uuid === r.uuid && "bg-muted/60",
                )}
              >
                <button
                  type="button"
                  className="min-w-0 flex-1 px-1 text-left"
                  onClick={() => setSelected(r.uuid)}
                >
                  <span className="block truncate text-sm font-medium">
                    {r.name}
                  </span>
                  <span className="text-muted-foreground block truncate text-xs">
                    {r.definition.dimensions[0]
                      ? t(`reports.dimension.${r.definition.dimensions[0]}`)
                      : t("reports.dimension.none")}{" "}
                    · {t(`reports.chart.${r.definition.chart_type}`)}
                  </span>
                </button>
                {canWrite && (
                  <>
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("reports.custom.edit")}
                      onClick={() => setEditing(r)}
                    >
                      <Pencil className="size-4" />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("reports.custom.delete")}
                      onClick={() => setDeleting(r)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </>
                )}
              </li>
            ))}
          </ul>
          {current && (
            <ReportRunner key={current.uuid} slug={slug} report={current} />
          )}
        </div>
      )}

      {editing && (
        <ReportDialog
          open
          report={editing === "new" ? null : editing}
          meta={meta.data}
          pending={save.isPending}
          onOpenChange={(o) => !o && setEditing(null)}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("reports.custom.delete_title", { name: deleting?.name ?? "" })}
        description={t("reports.custom.delete_description")}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

/** Evaluates a saved definition server-side for the chosen range. */
function ReportRunner({
  slug,
  report,
}: {
  slug: string;
  report: CustomReport;
}) {
  const { t } = useLocale();
  const def = report.definition;
  const dimension = def.dimensions[0] ?? null;
  const [range, setRange] = useState(defaultRange);
  const [ran, setRan] = useState(range);
  const [metric, setMetric] = useState<ReportMetric>(def.metrics[0]!);
  const valid = Boolean(range.from && range.to && range.from <= range.to);
  const result = useQuery({
    queryKey: [
      "tenant",
      slug,
      "reports",
      "run",
      report.uuid,
      report.updated_at,
      ran,
    ],
    queryFn: () =>
      reportsService.evaluate(
        slug,
        report.uuid,
        windowFromDays(ran.from, ran.to),
      ),
  });
  const rows = result.data?.rows ?? [];

  return (
    <Card className="min-w-0">
      <CardContent className="space-y-4 py-4">
        <div className="flex flex-wrap items-end gap-3">
          <DateRangeFields
            from={range.from}
            to={range.to}
            onChange={setRange}
          />
          <Button
            size="sm"
            disabled={!valid || result.isFetching}
            onClick={() => {
              setRan(range);
              if (ran === range) void result.refetch();
            }}
          >
            <Play className="size-4" />
            {t("reports.custom.run")}
          </Button>
          {def.chart_type !== "table" && def.metrics.length > 1 && (
            <div className="grid gap-1.5 sm:ml-auto">
              <Label htmlFor="report-chart-metric" className="text-xs">
                {t("reports.custom.chart_metric")}
              </Label>
              <Select
                value={metric}
                onValueChange={(v) => setMetric(v as ReportMetric)}
              >
                <SelectTrigger
                  id="report-chart-metric"
                  className="w-full sm:w-52"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {def.metrics.map((m) => (
                    <SelectItem key={m} value={m}>
                      {t(`reports.metric.${m}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>
        <div className="flex flex-wrap gap-1.5">
          {def.filters.channel && (
            <Badge variant="outline">
              {t("reports.filters.channel")}: {def.filters.channel}
            </Badge>
          )}
          {def.filters.team && (
            <Badge variant="outline">
              {t("reports.filters.team_filtered")}
            </Badge>
          )}
          {def.filters.agent && (
            <Badge variant="outline">
              {t("reports.filters.agent_filtered")}
            </Badge>
          )}
          <Badge variant="secondary">{def.timezone}</Badge>
        </div>
        {result.isLoading ? (
          <Loading label={t("common.loading")} />
        ) : result.isError ? (
          <ErrorState
            title={t("reports.error.title")}
            description={t("reports.error.description")}
            onRetry={() => void result.refetch()}
          />
        ) : (
          <>
            {def.chart_type !== "table" && rows.length > 0 && (
              <ReportChart
                rows={rows}
                dimension={dimension}
                metric={metric}
                chartType={def.chart_type}
              />
            )}
            <ReportTable
              rows={rows}
              dimension={dimension}
              metrics={def.metrics}
            />
          </>
        )}
      </CardContent>
    </Card>
  );
}
