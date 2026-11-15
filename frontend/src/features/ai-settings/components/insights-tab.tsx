"use client";

import { useQuery } from "@tanstack/react-query";
import { CircleSlash } from "lucide-react";
import { useState, type ReactNode } from "react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import { formatLatency, formatMicros, formatNumber } from "../lib/format";
import {
  aiInsightsService,
  type AIKnowledgeEvaluationPoint,
  type AIKnowledgePanel,
} from "../services/ai-insights.service";
import { QueryState, Stat } from "./parts";
import type { AISettingsTabProps } from "./tab-registry";

const PERIODS = [7, 30, 90] as const;
const ALL = "all";

type Availability = { available: boolean; reason?: string };

export function percent(value: number | null | undefined, locale: string) {
  if (value === null || value === undefined) return "—";
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "percent",
    maximumFractionDigits: 1,
  }).format(value);
}

/** Card whose body is replaced by "no data" when the source is missing. */
function Section({
  title,
  state,
  children,
}: {
  title: string;
  state: Availability;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <Card className="min-w-0">
      <CardHeader className="pb-3">
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {state.available ? (
          children
        ) : (
          <div className="text-muted-foreground flex items-start gap-2 text-sm">
            <CircleSlash className="mt-0.5 size-4 shrink-0" />
            <div>
              <p className="text-foreground font-medium">
                {t("ai_settings.insights.no_data")}
              </p>
              <p className="text-xs">
                {t(
                  `ai_settings.insights.reason.${state.reason ?? "source_not_connected"}`,
                )}
              </p>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/** Label, value and a single-hue proportion bar (share of the largest row). */
function ShareRow({
  label,
  value,
  share,
}: {
  label: string;
  value: ReactNode;
  share: number;
}) {
  return (
    <li className="space-y-1">
      <div className="flex items-baseline justify-between gap-3 text-sm">
        <span className="min-w-0 truncate">{label}</span>
        <span className="text-muted-foreground shrink-0 tabular-nums">
          {value}
        </span>
      </div>
      <div className="bg-muted h-1.5 overflow-hidden rounded-full">
        <div
          className="bg-primary h-full rounded-full"
          style={{ width: `${Math.max(0, Math.min(1, share)) * 100}%` }}
        />
      </div>
    </li>
  );
}

function EvaluationSummary({
  latest,
  previous,
}: {
  latest: AIKnowledgeEvaluationPoint | null | undefined;
  previous: AIKnowledgeEvaluationPoint | null | undefined;
}) {
  const { t, locale } = useLocale();
  if (!latest)
    return (
      <p className="text-muted-foreground text-sm">
        {t("ai_settings.insights.evaluation_none")}
      </p>
    );
  const rows: {
    key: string;
    value: (p: AIKnowledgeEvaluationPoint) => number | null | undefined;
    format: (v: number) => string;
  }[] = [
    {
      key: "answer_accuracy",
      value: (p) => p.answer_accuracy,
      format: (v) => percent(v, locale),
    },
    {
      key: "source_hit_rate",
      value: (p) => p.source_hit_rate,
      format: (v) => percent(v, locale),
    },
    {
      key: "cache_hit_rate",
      value: (p) => p.cache_hit_rate,
      format: (v) => percent(v, locale),
    },
    {
      key: "avg_latency_ms",
      value: (p) => p.avg_latency_ms,
      format: (v) => formatLatency(Math.round(v), locale),
    },
    {
      key: "cost_per_case_micros",
      value: (p) => p.cost_per_case_micros,
      format: (v) => formatMicros(v, locale),
    },
  ];
  return (
    <div className="space-y-3">
      <p className="text-muted-foreground text-xs">
        {t("ai_settings.insights.evaluation_latest", {
          date: datetime(latest.created_at, undefined, locale),
          trigger: t(`ai_settings.evaluation.trigger.${latest.trigger}`),
        })}
      </p>
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {rows.map((row) => {
          const now = row.value(latest);
          const before = previous ? row.value(previous) : null;
          return (
            <Stat
              key={row.key}
              label={t(`ai_settings.evaluation.metric.${row.key}`)}
              value={
                <>
                  {now === null || now === undefined ? "—" : row.format(now)}
                  {before !== null && before !== undefined && (
                    <span className="text-muted-foreground ms-1 text-xs font-normal">
                      ({t("ai_settings.insights.before")}: {row.format(before)})
                    </span>
                  )}
                </>
              }
            />
          );
        })}
      </dl>
    </div>
  );
}

function Panel({ data }: { data: AIKnowledgePanel }) {
  const { t, locale } = useLocale();
  const n = (v: number) => formatNumber(v, locale);
  const topGap = Math.max(
    1,
    ...data.knowledge_gaps.topics.map((x) => x.conversations),
  );
  const topCategory = Math.max(
    1,
    ...data.categories.categories.map((x) => x.conversations),
  );
  const topBucket = Math.max(
    1,
    ...data.categories.confidence_buckets.map((x) => x.count),
  );
  const pipeline = data.suggestions;
  const pipelineTotal = Math.max(
    1,
    pipeline.pending + pipeline.approved + pipeline.rejected,
  );
  return (
    <div className="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
      <Section title={t("ai_settings.insights.cache")} state={data.cache}>
        <dl className="grid grid-cols-3 gap-2">
          <Stat
            label={t("ai_settings.insights.hit_rate")}
            value={percent(data.cache.hit_rate, locale)}
          />
          <Stat
            label={t("ai_settings.insights.hits")}
            value={n(data.cache.hits)}
          />
          <Stat
            label={t("ai_settings.insights.lookups")}
            value={n(data.cache.lookups)}
          />
        </dl>
      </Section>
      <Section title={t("ai_settings.insights.usage")} state={data.ai_usage}>
        <dl className="mb-3 grid grid-cols-3 gap-2">
          <Stat
            label={t("ai_settings.insights.calls")}
            value={n(data.ai_usage.calls)}
          />
          <Stat
            label={t("ai_settings.insights.failed")}
            value={n(data.ai_usage.failed)}
          />
          <Stat
            label={t("ai_settings.insights.cost")}
            value={formatMicros(data.ai_usage.cost_micros, locale)}
          />
        </dl>
        <ul className="space-y-1 text-sm">
          {data.ai_usage.by_task.map((task) => (
            <li key={task.task} className="flex justify-between gap-3">
              <span>{task.task}</span>
              <span className="text-muted-foreground tabular-nums">
                {n(task.calls)} · {formatMicros(task.cost_micros, locale)}
              </span>
            </li>
          ))}
        </ul>
      </Section>
      <Section title={t("ai_settings.insights.handoff")} state={data.handoff}>
        <dl className="grid grid-cols-3 gap-2">
          <Stat
            label={t("ai_settings.insights.handoff_rate")}
            value={percent(data.handoff.rate, locale)}
          />
          <Stat
            label={t("ai_settings.insights.handoffs")}
            value={n(data.handoff.handoffs)}
          />
          <Stat
            label={t("ai_settings.insights.conversations")}
            value={n(data.handoff.conversations)}
          />
        </dl>
      </Section>
      <Section
        title={t("ai_settings.insights.gaps")}
        state={data.knowledge_gaps}
      >
        {data.knowledge_gaps.topics.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("ai_settings.insights.gaps_empty")}
          </p>
        ) : (
          <ul className="space-y-2">
            {data.knowledge_gaps.topics.map((topic) => (
              <ShareRow
                key={topic.topic}
                label={topic.topic}
                value={n(topic.conversations)}
                share={topic.conversations / topGap}
              />
            ))}
          </ul>
        )}
      </Section>
      <Section
        title={t("ai_settings.insights.suggestions")}
        state={data.suggestions}
      >
        <ul className="space-y-2">
          {(["pending", "approved", "rejected"] as const).map((status) => (
            <ShareRow
              key={status}
              label={t(`ai_settings.insights.suggestion.${status}`)}
              value={n(pipeline[status])}
              share={pipeline[status] / pipelineTotal}
            />
          ))}
        </ul>
      </Section>
      <Section
        title={t("ai_settings.insights.categories")}
        state={data.categories}
      >
        <dl className="mb-3 grid grid-cols-3 gap-2">
          <Stat
            label={t("ai_settings.insights.classified")}
            value={n(data.categories.classified)}
          />
          <Stat
            label={t("ai_settings.insights.avg_confidence")}
            value={percent(data.categories.avg_confidence, locale)}
          />
          <Stat
            label={t("ai_settings.insights.low_confidence")}
            value={n(data.categories.low_confidence)}
          />
        </dl>
        <ul className="space-y-2">
          {data.categories.categories.slice(0, 8).map((c) => (
            <ShareRow
              key={c.category}
              label={c.category}
              value={`${n(c.conversations)} · ${percent(c.avg_confidence, locale)}`}
              share={c.conversations / topCategory}
            />
          ))}
        </ul>
        {data.categories.classified > 0 && (
          <div className="mt-4 space-y-2">
            <p className="text-muted-foreground text-xs">
              {t("ai_settings.insights.confidence_distribution")}
            </p>
            <ul className="space-y-2">
              {data.categories.confidence_buckets.map((b) => (
                <ShareRow
                  key={b.from}
                  label={`${percent(b.from, locale)} – ${percent(b.to, locale)}`}
                  value={n(b.count)}
                  share={b.count / topBucket}
                />
              ))}
            </ul>
          </div>
        )}
      </Section>
      <div className="lg:col-span-2 xl:col-span-3">
        <Section
          title={t("ai_settings.insights.evaluation")}
          state={data.evaluation}
        >
          <EvaluationSummary
            latest={data.evaluation.latest}
            previous={data.evaluation.previous}
          />
        </Section>
      </div>
    </div>
  );
}

/**
 * "Bilgi paneli" tab (CF-220): cache, AI calls and cost, knowledge gaps,
 * suggestion pipeline, categories and confidence, handoff rate and the
 * latest evaluation run, for the organization or one department. A
 * section whose source has not landed yet says "no data" instead of 0.
 */
export function InsightsTab({ slug }: AISettingsTabProps) {
  const { t } = useLocale();
  const [days, setDays] = useState<(typeof PERIODS)[number]>(30);
  const [department, setDepartment] = useState(ALL);
  const query = useQuery({
    queryKey: ["tenant", slug, "ai", "insights", days, department],
    queryFn: () =>
      aiInsightsService.panel(slug, {
        from: new Date(Date.now() - days * 86_400_000).toISOString(),
        department_uuid: department === ALL ? undefined : department,
      }),
  });
  const departments = query.data?.departments ?? [];
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground me-auto text-sm">
          {t("ai_settings.insights.description")}
        </p>
        {departments.length > 0 && (
          <Select value={department} onValueChange={setDepartment}>
            <SelectTrigger
              className="w-48"
              aria-label={t("ai_settings.insights.department")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>
                {t("ai_settings.insights.organization")}
              </SelectItem>
              {departments.map((d) => (
                <SelectItem key={d.uuid} value={d.uuid}>
                  {d.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        <Select
          value={String(days)}
          onValueChange={(v) => setDays(Number(v) as (typeof PERIODS)[number])}
        >
          <SelectTrigger
            className="w-40"
            aria-label={t("ai_settings.insights.period")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PERIODS.map((p) => (
              <SelectItem key={p} value={String(p)}>
                {t("ai_settings.insights.last_days", { count: p })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {departments.length === 0 && query.data && (
        <p className="text-muted-foreground text-xs">
          {t("ai_settings.insights.departments_pending")}
        </p>
      )}
      {query.data ? (
        <Panel data={query.data} />
      ) : (
        <QueryState
          loading={query.isLoading}
          error={query.isError}
          onRetry={() => void query.refetch()}
        />
      )}
    </div>
  );
}
