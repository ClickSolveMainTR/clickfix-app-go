"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { StatusChip } from "@/components/common/status-chip";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { Button } from "@/components/ui/button";
import { cn, relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { useOncallError } from "../lib/use-oncall-error";
import {
  oncallService,
  type UptimeMonitor,
  type UptimeMonitorInput,
  type UptimeResult,
} from "../services/oncall.service";
import { intervalLabel, MonitorDialog } from "./monitor-dialog";

export function MonitorsTab({
  slug,
  canWrite,
  canManageComponents,
}: {
  slug: string;
  canWrite: boolean;
  canManageComponents: boolean;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useOncallError();
  const key = ["tenant", slug, "oncall", "monitors"] as const;
  const monitors = useQuery({
    queryKey: key,
    queryFn: () => oncallService.monitors(slug),
    refetchInterval: 60_000,
  });
  const components = useQuery({
    queryKey: ["tenant", slug, "status", "components"],
    queryFn: () => oncallService.components(slug),
  });
  const policies = useQuery({
    queryKey: ["tenant", slug, "oncall", "policies"],
    queryFn: () => oncallService.policies(slug),
  });
  const [editing, setEditing] = useState<UptimeMonitor | "new" | null>(null);
  const [deleting, setDeleting] = useState<UptimeMonitor | null>(null);

  const save = useMutation({
    mutationFn: (body: UptimeMonitorInput) =>
      editing && editing !== "new"
        ? oncallService.updateMonitor(slug, editing.uuid, body)
        : oncallService.createMonitor(slug, body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("oncall.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => oncallService.removeMonitor(slug, uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("oncall.toast.deleted"));
    },
    onError,
  });

  const componentName = useMemo(() => {
    const names = new Map((components.data ?? []).map((c) => [c.uuid, c.name]));
    return (uuid: string) => names.get(uuid) ?? "—";
  }, [components.data]);

  const columns = useMemo(
    () =>
      [
        createColumn<UptimeMonitor>({
          id: "monitor",
          // Search matches the name and the address.
          accessorFn: (m) => `${m.name} ${m.url}`,
          sortingFn: (a, b) =>
            a.original.name.localeCompare(b.original.name, locale),
          labelKey: "oncall.monitors.columns.monitor",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const m = row.original;
            return (
              <div className="min-w-0 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <MonitorStatus monitor={m} />
                  <span className="font-medium">{m.name}</span>
                </div>
                <a
                  href={m.url}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="text-muted-foreground block truncate font-mono text-xs hover:underline"
                >
                  {m.url}
                </a>
                <p className="text-muted-foreground text-xs whitespace-normal">
                  {t("oncall.monitors.every", {
                    interval: intervalLabel(m.interval_seconds, t),
                  })}{" "}
                  ·{" "}
                  {t("oncall.monitors.expects", { status: m.expected_status })}
                  {m.keyword && ` · "${m.keyword}"`} ·{" "}
                  {componentName(m.component_uuid)}
                  {m.last_checked_at &&
                    ` · ${t("oncall.monitors.checked", {
                      time: relativeDatetime(m.last_checked_at, locale),
                    })}`}
                </p>
              </div>
            );
          },
        }),
        createColumn<UptimeMonitor>({
          id: "checks",
          labelKey: "oncall.monitors.columns.checks",
          enableSorting: false,
          cell: ({ row }) => <Sparkline slug={slug} monitor={row.original} />,
        }),
        ...(canWrite
          ? [
              createColumn<UptimeMonitor>({
                id: "actions",
                labelKey: "common.actions",
                enableHiding: false,
                enableSorting: false,
                meta: {
                  headerClassName: "text-end",
                  cellClassName: "text-end",
                },
                cell: ({ row }) => (
                  <div className="flex justify-end gap-1">
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("oncall.monitors.edit")}
                      onClick={() => setEditing(row.original)}
                    >
                      <Pencil className="size-4" />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("oncall.monitors.delete")}
                      onClick={() => setDeleting(row.original)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<UptimeMonitor, unknown>[],
    [canWrite, componentName, locale, slug, t],
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {t("oncall.monitors.hint")}
        </p>
        {canWrite && (
          <Button
            size="sm"
            disabled={components.isLoading}
            onClick={() => setEditing("new")}
          >
            <Plus className="size-4" />
            {t("oncall.monitors.new")}
          </Button>
        )}
      </div>
      <EntityTable
        columns={columns}
        data={monitors.data ?? []}
        getRowId={(m) => m.uuid}
        isLoading={monitors.isLoading}
        isError={monitors.isError}
        errorTitle={t("oncall.error.title")}
        errorDescription={t("oncall.error.description")}
        onRetry={() => void monitors.refetch()}
        emptyTitle={t("oncall.monitors.empty_title")}
        emptyDescription={t("oncall.monitors.empty_description")}
        // One unpaged list (up to 100): search, sort and pages in the browser,
        // so only the visible rows load their check history.
        manual={{ filtering: false, sorting: false, pagination: false }}
        initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
        features={{
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
      {editing && (
        <MonitorDialog
          slug={slug}
          monitor={editing === "new" ? null : editing}
          components={components.data ?? []}
          policies={policies.data ?? []}
          pending={save.isPending}
          canManageComponents={canManageComponents}
          onOpenChange={(o) => !o && setEditing(null)}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("oncall.monitors.delete_title", {
          name: deleting?.name ?? "",
        })}
        description={t("oncall.monitors.delete_description")}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

function MonitorStatus({ monitor }: { monitor: UptimeMonitor }) {
  const { t } = useLocale();
  if (!monitor.enabled)
    return <StatusChip label={t("oncall.monitors.status.paused")} />;
  if (monitor.last_up === null)
    return <StatusChip label={t("oncall.monitors.status.pending")} />;
  return monitor.last_up ? (
    <StatusChip label={t("oncall.monitors.status.up")} tone="success" />
  ) : (
    <StatusChip
      label={
        monitor.last_status
          ? t("oncall.monitors.status.down_code", {
              code: monitor.last_status,
            })
          : t("oncall.monitors.status.down")
      }
      tone="danger"
    />
  );
}

const SPARK_W = 120;
const SPARK_H = 28;

/** Latency of the last 30 checks (oldest left); failed checks in red. */
function Sparkline({
  slug,
  monitor,
}: {
  slug: string;
  monitor: UptimeMonitor;
}) {
  const { t, locale } = useLocale();
  const results = useQuery({
    queryKey: [
      "tenant",
      slug,
      "oncall",
      "monitors",
      monitor.uuid,
      "results",
      monitor.last_checked_at,
    ],
    queryFn: () => oncallService.results(slug, monitor.uuid, 30),
  });
  const points: UptimeResult[] = [...(results.data ?? [])].reverse();
  if (points.length === 0)
    return (
      <span className="text-muted-foreground w-[120px] text-xs">
        {results.isLoading ? "…" : t("oncall.monitors.no_results")}
      </span>
    );
  const up = points.filter((p) => p.up).length;
  const max = Math.max(...points.map((p) => p.latency_ms), 1);
  const step = points.length > 1 ? SPARK_W / (points.length - 1) : 0;
  const y = (ms: number) => SPARK_H - 2 - (ms / max) * (SPARK_H - 4);
  const path = points
    .map(
      (p, i) =>
        `${i ? "L" : "M"}${(i * step).toFixed(1)},${y(p.latency_ms).toFixed(1)}`,
    )
    .join(" ");
  const last = points[points.length - 1]!;
  const summary = t("oncall.monitors.sparkline", {
    up,
    total: points.length,
    latency: last.latency_ms,
  });
  return (
    <div className="flex items-center gap-2">
      <svg
        width={SPARK_W}
        height={SPARK_H}
        viewBox={`0 0 ${SPARK_W} ${SPARK_H}`}
        role="img"
        aria-label={summary}
        className="text-primary shrink-0 overflow-visible"
      >
        <title>{summary}</title>
        <path d={path} fill="none" stroke="currentColor" strokeWidth={1.5} />
        {points.map((p, i) =>
          p.up ? null : (
            <circle
              key={p.uuid}
              cx={i * step}
              cy={y(p.latency_ms)}
              r={2.5}
              className="fill-destructive"
            />
          ),
        )}
      </svg>
      <span
        className={cn(
          "w-12 text-right text-xs tabular-nums",
          up === points.length ? "text-muted-foreground" : "text-destructive",
        )}
      >
        {new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
          style: "percent",
          maximumFractionDigits: 1,
        }).format(up / points.length)}
      </span>
    </div>
  );
}
