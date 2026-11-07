"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertOctagon,
  BookOpen,
  Cpu,
  FileInput,
  FileOutput,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge, type BadgeProps } from "@/components/ui/badge";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { datetime } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatLatency, formatMicros, formatNumber } from "../lib/format";
import type { AIRun, Page } from "../services/ai-settings.service";
import { QueryState, Stat } from "./parts";

const STATUS_VARIANT: Record<AIRun["status"], BadgeProps["variant"]> = {
  completed: "success",
  running: "secondary",
  failed: "danger",
  blocked: "warning",
  unknown: "outline",
};

export function RunStatusBadge({ status }: { status: AIRun["status"] }) {
  const { t } = useLocale();
  return (
    <Badge variant={STATUS_VARIANT[status] ?? "outline"}>
      {t(`ai_settings.runs.status.${status}`)}
    </Badge>
  );
}

type RunLogProps = {
  /** React-query key prefix; page offset / run uuid are appended. */
  queryKey: readonly unknown[];
  fetchPage: (offset: number, limit: number) => Promise<Page<AIRun>>;
  fetchRun: (uuid: string) => Promise<AIRun>;
  /** Controlled selection (e.g. `?run=` in the URL). */
  selected?: string | null;
  onSelect?: (uuid: string | null) => void;
};

/**
 * Masked AI run log with a detail sheet (CF-69). Used by the organization
 * settings ("Çalışma kaydı") and by the platform organization page, which
 * passes its own fetchers.
 */
export function RunLog({
  queryKey,
  fetchPage,
  fetchRun,
  selected: controlled,
  onSelect,
}: RunLogProps) {
  const { t, locale } = useLocale();
  const listState = useServerListState({ initialPageSize: 20 });
  const { limit, offset } = listState.params;
  const [local, setLocal] = useState<string | null>(null);
  const selected = controlled !== undefined ? controlled : local;
  const select = (uuid: string | null) => {
    setLocal(uuid);
    onSelect?.(uuid);
  };
  const page = useQuery({
    queryKey: [...queryKey, "page", limit, offset],
    queryFn: () => fetchPage(offset, limit),
    placeholderData: keepPreviousData,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<AIRun>({
          accessorKey: "created_at",
          labelKey: "ai_settings.runs.created",
          enableHiding: false,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {datetime(row.original.created_at, "dd.MM.yyyy HH:mm:ss", locale)}
            </span>
          ),
        }),
        createColumn<AIRun>({
          accessorKey: "task",
          labelKey: "ai_settings.runs.task",
          gridPrimary: true,
          cell: ({ row }) =>
            t(`ai_settings.runs.task_name.${row.original.task}`),
        }),
        createColumn<AIRun>({
          accessorKey: "status",
          labelKey: "ai_settings.runs.status_label",
          cell: ({ row }) => <RunStatusBadge status={row.original.status} />,
        }),
        createColumn<AIRun>({
          accessorKey: "model",
          labelKey: "ai_settings.runs.model",
          cell: ({ row }) => (
            <span className="text-muted-foreground block max-w-[12rem] truncate">
              {row.original.model || "-"}
            </span>
          ),
        }),
        createColumn<AIRun>({
          id: "tokens",
          accessorFn: (row) => row.input_tokens + row.output_tokens,
          labelKey: "ai_settings.runs.tokens",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) =>
            formatNumber(
              row.original.input_tokens + row.original.output_tokens,
              locale,
            ),
        }),
        createColumn<AIRun>({
          accessorKey: "cost_micros",
          labelKey: "ai_settings.runs.cost",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) => formatMicros(row.original.cost_micros, locale),
        }),
        createColumn<AIRun>({
          accessorKey: "latency_ms",
          labelKey: "ai_settings.runs.latency",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) => formatLatency(row.original.latency_ms, locale),
        }),
      ] as ColumnDef<AIRun, unknown>[],
    [locale, t],
  );

  return (
    <div className="space-y-3">
      <EntityTable
        columns={columns}
        data={page.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => select(row.uuid)}
        isLoading={page.isLoading}
        isError={page.isError}
        onRetry={() => void page.refetch()}
        emptyTitle={t("ai_settings.runs.empty_title")}
        emptyDescription={t("ai_settings.runs.empty")}
        total={page.data?.total ?? 0}
        state={listState.tableState}
        features={{
          // The endpoint takes `limit` and `offset` only (it rejects `sort`
          // and `q`).
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
        }}
      />
      <Sheet
        open={Boolean(selected)}
        onOpenChange={(open) => {
          if (!open) select(null);
        }}
      >
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
          {selected && (
            <RunDetail
              queryKey={[...queryKey, "run", selected]}
              fetchRun={() => fetchRun(selected)}
            />
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}

type StepTone = "ok" | "warn" | "error" | "muted";

function Step({
  icon: Icon,
  title,
  meta,
  tone = "ok",
  children,
}: {
  icon: LucideIcon;
  title: string;
  meta?: ReactNode;
  tone?: StepTone;
  children?: ReactNode;
}) {
  return (
    <li className="relative ps-10">
      <span
        aria-hidden
        className={cn(
          "bg-background absolute start-0 top-0 flex size-7 items-center justify-center rounded-full border",
          tone === "ok" && "border-foreground/40",
          tone === "warn" &&
            "border-amber-500 text-amber-600 dark:text-amber-400",
          tone === "error" && "border-destructive text-destructive",
          tone === "muted" && "text-muted-foreground",
        )}
      >
        <Icon className="size-3.5" />
      </span>
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <p className="text-sm font-medium">{title}</p>
        {meta ? (
          <span className="text-muted-foreground text-xs">{meta}</span>
        ) : null}
      </div>
      {children ? <div className="mt-2 space-y-2">{children}</div> : null}
    </li>
  );
}

function TextBlock({ text, empty }: { text: string; empty: string }) {
  return (
    <pre className="bg-muted/40 max-h-64 overflow-auto rounded-md border p-3 font-mono text-xs break-words whitespace-pre-wrap">
      {text || empty}
    </pre>
  );
}

/** One run as a vertical step timeline ("akış"). */
function RunDetail({
  queryKey,
  fetchRun,
}: {
  queryKey: readonly unknown[];
  fetchRun: () => Promise<AIRun>;
}) {
  const { t, locale } = useLocale();
  const query = useQuery({ queryKey, queryFn: fetchRun });
  const run = query.data;
  if (!run) {
    return (
      <>
        <SheetHeader>
          <SheetTitle>{t("ai_settings.runs.detail_title")}</SheetTitle>
        </SheetHeader>
        <QueryState
          loading={query.isLoading}
          error={query.isError}
          onRetry={() => void query.refetch()}
        />
      </>
    );
  }
  const failed = run.status === "failed" || run.status === "blocked";
  return (
    <div className="space-y-6">
      <SheetHeader className="space-y-2 pe-8">
        <SheetTitle className="flex flex-wrap items-center gap-2">
          {t(`ai_settings.runs.task_name.${run.task}`)}
          <RunStatusBadge status={run.status} />
        </SheetTitle>
        <SheetDescription>
          {datetime(run.created_at, "dd.MM.yyyy HH:mm:ss", locale)}
          {" · "}
          <code className="text-xs">{run.uuid}</code>
        </SheetDescription>
      </SheetHeader>

      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        <Stat
          label={t("ai_settings.runs.input_tokens")}
          value={formatNumber(run.input_tokens, locale)}
        />
        <Stat
          label={t("ai_settings.runs.output_tokens")}
          value={formatNumber(run.output_tokens, locale)}
        />
        <Stat
          label={t("ai_settings.runs.latency")}
          value={formatLatency(run.latency_ms, locale)}
        />
        <Stat
          label={t("ai_settings.runs.cost")}
          value={formatMicros(run.cost_micros, locale)}
        />
        <Stat
          label={t("ai_settings.runs.reserved_cost")}
          value={formatMicros(run.reserved_cost_micros, locale)}
        />
        <Stat
          label={t("ai_settings.runs.credits")}
          value={`${formatNumber(run.charged_credits, locale)} / ${formatNumber(run.reserved_credits, locale)}`}
        />
      </dl>

      <section className="space-y-3">
        <h3 className="text-sm font-semibold">{t("ai_settings.runs.flow")}</h3>
        <ol className="before:bg-border relative space-y-5 before:absolute before:start-3.5 before:top-2 before:bottom-2 before:w-px">
          <Step
            icon={FileInput}
            title={t("ai_settings.runs.step_input")}
            meta={t("ai_settings.runs.masked")}
          >
            <TextBlock text={run.input} empty={t("ai_settings.runs.no_text")} />
          </Step>
          <Step
            icon={BookOpen}
            tone={run.retrieved_chunks.length ? "ok" : "muted"}
            title={t("ai_settings.runs.step_retrieval")}
            meta={t("ai_settings.runs.chunk_count", {
              count: run.retrieved_chunks.length,
            })}
          >
            {run.retrieved_chunks.length > 0 && (
              <ul className="space-y-2">
                {run.retrieved_chunks.map((chunk) => (
                  <li
                    key={chunk.chunk_uuid}
                    className="rounded-md border p-2 text-xs"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="min-w-0 truncate font-medium">
                        {chunk.title || chunk.slug}
                      </span>
                      <Badge variant="outline" className="shrink-0">
                        {t("ai_settings.runs.score", {
                          score: chunk.score.toFixed(3),
                        })}
                      </Badge>
                    </div>
                    <p className="text-muted-foreground mt-1 line-clamp-3">
                      {chunk.content}
                    </p>
                    <p className="text-muted-foreground mt-1">
                      {chunk.slug} · v{chunk.version} · {chunk.locale}
                    </p>
                  </li>
                ))}
              </ul>
            )}
          </Step>
          <Step
            icon={Cpu}
            tone={failed && !run.output ? "error" : "ok"}
            title={t("ai_settings.runs.step_model")}
            meta={[run.provider, run.model].filter(Boolean).join(" · ") || "-"}
          >
            <p className="text-muted-foreground text-xs">
              {t("ai_settings.runs.model_summary", {
                input: formatNumber(run.input_tokens, locale),
                output: formatNumber(run.output_tokens, locale),
                latency: formatLatency(run.latency_ms, locale),
              })}
            </p>
          </Step>
          <Step
            icon={Wrench}
            tone={run.tool_calls.length ? "ok" : "muted"}
            title={t("ai_settings.runs.step_tools")}
            meta={t("ai_settings.runs.tool_count", {
              count: run.tool_calls.length,
            })}
          >
            {run.tool_calls.length > 0 && (
              <ul className="space-y-2">
                {run.tool_calls.map((call, index) => (
                  <li key={call.id || index} className="rounded-md border p-2">
                    <p className="font-mono text-xs font-medium">{call.name}</p>
                    <pre className="text-muted-foreground mt-1 overflow-auto text-xs break-words whitespace-pre-wrap">
                      {JSON.stringify(call.input, null, 2)}
                    </pre>
                  </li>
                ))}
              </ul>
            )}
          </Step>
          {run.error_code ? (
            <Step
              icon={AlertOctagon}
              tone="error"
              title={t("ai_settings.runs.step_error")}
              meta={<code>{run.error_code}</code>}
            >
              <p className="text-muted-foreground text-xs">
                {t(`ai_settings.runs.error_hint.${run.status}`)}
              </p>
            </Step>
          ) : null}
          <Step
            icon={FileOutput}
            tone={run.output ? "ok" : "muted"}
            title={t("ai_settings.runs.step_output")}
            meta={
              run.finished_at
                ? datetime(run.finished_at, "HH:mm:ss", locale)
                : t("ai_settings.runs.not_finished")
            }
          >
            <TextBlock
              text={run.output}
              empty={t("ai_settings.runs.no_text")}
            />
          </Step>
        </ol>
      </section>
    </div>
  );
}
