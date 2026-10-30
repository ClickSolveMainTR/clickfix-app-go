"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, ExternalLink, Lock } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { AppChart, buildChartConfig } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { formatScore } from "../lib/format";
import {
  coachingService,
  type CalibrationSession,
  type RubricCriterion,
} from "../services/coaching.service";

/** Criteria whose reviewers are this many points apart are flagged. */
const SPREAD_FLAG = 20;

type Draft = Record<string, { value: string; rationale: string }>;

/**
 * One calibration session. While open each reviewer submits their own
 * scores blind (peers stay hidden) and a manager closes it once two or
 * more reviewers scored; after closing it shows mean, variance and range
 * per criterion and every reviewer's scores.
 */
export function CalibrationDialog({
  slug,
  uuid,
  onOpenChange,
}: {
  slug: string;
  uuid: string;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const key = ["tenant", slug, "quality", "calibrations", uuid];
  const session = useQuery({
    queryKey: key,
    queryFn: () => coachingService.calibration(slug, uuid),
  });
  const data = session.data;

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={data?.name ?? t("coaching.calibrations.session")}
      description={
        data?.closed_at
          ? t("coaching.calibrations.closed_hint")
          : t("coaching.calibrations.open_hint")
      }
      className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"
    >
      {session.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : session.isError || !data ? (
        <ErrorState
          title={t("coaching.error.title")}
          description={
            isApiError(session.error)
              ? session.error.message
              : t("coaching.error.description")
          }
          onRetry={() => void session.refetch()}
        />
      ) : (
        <div className="grid gap-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={data.closed_at ? "secondary" : "success"}>
              {data.closed_at
                ? t("coaching.calibrations.status_closed")
                : t("coaching.calibrations.status_open")}
            </Badge>
            <Link
              href={`/inbox?c=${data.conversation_uuid}`}
              className="text-primary inline-flex items-center gap-1 hover:underline"
            >
              {t("coaching.calibrations.open_conversation")}
              <ExternalLink className="size-3.5" />
            </Link>
          </div>
          {data.closed_at ? (
            <CalibrationResults session={data} />
          ) : (
            <ScoreForm
              slug={slug}
              session={data}
              onClose={() => onOpenChange(false)}
            />
          )}
        </div>
      )}
    </AppDialog>
  );
}

function ScoreForm({
  slug,
  session,
  onClose,
}: {
  slug: string;
  session: CalibrationSession;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const rubric = session.rubric;
  const [draft, setDraft] = useState<Draft>(() =>
    Object.fromEntries(
      rubric.map((c) => [c.key ?? "", { value: "", rationale: "" }]),
    ),
  );
  const [submitted, setSubmitted] = useState(false);

  const valid = rubric.every((c) => {
    const d = draft[c.key ?? ""];
    const n = Number(d?.value);
    return (
      d &&
      d.value.trim() !== "" &&
      Number.isInteger(n) &&
      n >= 0 &&
      n <= 100 &&
      d.rationale.trim().length > 0 &&
      d.rationale.length <= 2000
    );
  });

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["tenant", slug, "quality", "calibrations"],
    });

  const score = useMutation({
    mutationFn: () =>
      coachingService.score(
        slug,
        session.uuid,
        rubric.map((c) => ({
          key: c.key ?? "",
          value: Number(draft[c.key ?? ""]!.value),
          rationale: draft[c.key ?? ""]!.rationale.trim(),
        })),
      ),
    onSuccess: (res) => {
      setSubmitted(true);
      appToast.success(
        t("coaching.calibrations.scored", {
          score: formatScore(res.score, locale),
        }),
      );
    },
  });
  const close = useMutation({
    mutationFn: () => coachingService.closeCalibration(slug, session.uuid),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("coaching.calibrations.closed"));
    },
  });

  const patch = (k: string, p: Partial<Draft[string]>) =>
    setDraft((d) => ({ ...d, [k]: { ...d[k]!, ...p } }));

  return (
    <>
      {submitted ? (
        <div className="flex items-center gap-2 rounded-md border border-emerald-600/30 bg-emerald-600/10 p-3 text-sm">
          <CheckCircle2 className="size-4 text-emerald-600" />
          {t("coaching.calibrations.submitted")}
        </div>
      ) : (
        <section className="grid gap-3">
          <h3 className="text-sm font-medium">
            {t("coaching.calibrations.your_scores")}
          </h3>
          {rubric.map((c) => (
            <CriterionInput
              key={c.key}
              criterion={c}
              value={draft[c.key ?? ""]?.value ?? ""}
              rationale={draft[c.key ?? ""]?.rationale ?? ""}
              onChange={(p) => patch(c.key ?? "", p)}
            />
          ))}
          <p className="text-muted-foreground text-xs">
            {t("coaching.calibrations.score_hint")}
          </p>
        </section>
      )}
      <FormActions className="flex-wrap">
        <Button
          variant="outline"
          disabled={close.isPending}
          onClick={() => close.mutate()}
        >
          <Lock className="size-4" />
          {t("coaching.calibrations.close")}
        </Button>
        {submitted ? (
          <Button onClick={onClose}>{t("common.close")}</Button>
        ) : (
          <Button
            disabled={!valid || score.isPending}
            onClick={() => score.mutate()}
          >
            {t("coaching.calibrations.submit")}
          </Button>
        )}
      </FormActions>
    </>
  );
}

function CriterionInput({
  criterion,
  value,
  rationale,
  onChange,
}: {
  criterion: RubricCriterion;
  value: string;
  rationale: string;
  onChange: (p: { value?: string; rationale?: string }) => void;
}) {
  const { t } = useLocale();
  const id = `criterion-${criterion.key}`;
  return (
    <div className="grid gap-2 rounded-md border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">
          {criterion.label || criterion.key}
        </span>
        <Badge variant="outline">
          {t("coaching.calibrations.weight", { weight: criterion.weight ?? 0 })}
        </Badge>
        {criterion.critical && (
          <Badge variant="danger">{t("coaching.calibrations.critical")}</Badge>
        )}
      </div>
      <div className="grid gap-3 sm:grid-cols-[7rem_1fr]">
        <div className="grid gap-1.5">
          <Label htmlFor={`${id}-value`} className="text-xs">
            {t("coaching.calibrations.value")}
          </Label>
          <Input
            id={`${id}-value`}
            type="number"
            min={0}
            max={100}
            value={value}
            onChange={(e) => onChange({ value: e.target.value })}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`${id}-rationale`} className="text-xs">
            {t("coaching.calibrations.rationale")}
          </Label>
          <Textarea
            id={`${id}-rationale`}
            rows={2}
            maxLength={2000}
            value={rationale}
            onChange={(e) => onChange({ rationale: e.target.value })}
          />
        </div>
      </div>
    </div>
  );
}

function CalibrationResults({ session }: { session: CalibrationSession }) {
  const { t, locale } = useLocale();
  const directory = useMemberDirectory();
  const summary = session.summary;
  const reviews = useMemo(() => session.reviews ?? [], [session.reviews]);
  const criteria = useMemo(
    () =>
      session.rubric.map((c) => ({
        key: c.key ?? "",
        label: c.label || c.key || "",
        critical: Boolean(c.critical),
        stats: session.criteria?.[c.key ?? ""],
      })),
    [session.rubric, session.criteria],
  );
  const chartData = useMemo(
    () =>
      criteria.map((c) => ({
        label: c.label,
        mean: Math.round((c.stats?.mean ?? 0) * 10) / 10,
        range: c.stats?.range ?? 0,
      })),
    [criteria],
  );
  const config = useMemo(
    () =>
      buildChartConfig([
        { key: "mean", label: t("coaching.calibrations.mean") },
        { key: "range", label: t("coaching.calibrations.range") },
      ]),
    [t],
  );
  const nf = (v: number) => formatScore(v, locale);

  return (
    <div className="grid gap-4">
      {summary && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <StatsCard
            title={t("coaching.calibrations.reviewers")}
            value={summary.count}
          />
          <StatsCard
            title={t("coaching.calibrations.mean_score")}
            value={nf(summary.mean)}
          />
          <StatsCard
            title={t("coaching.calibrations.variance")}
            value={nf(summary.variance)}
          />
          <StatsCard
            title={t("coaching.calibrations.range")}
            value={
              <span
                className={cn(
                  summary.range >= SPREAD_FLAG && "text-destructive",
                )}
              >
                {nf(summary.range)}
              </span>
            }
          />
        </div>
      )}

      <AppChart
        type="bar"
        data={chartData}
        config={config}
        series={["mean", "range"]}
        showLegend
        height={220}
        title={t("coaching.calibrations.by_criterion")}
        valueFormatter={nf}
      />

      <div className="overflow-x-auto rounded-md border">
        <table className="w-full min-w-[34rem] text-sm">
          <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
            <tr>
              <th scope="col" className="px-3 py-2 font-medium">
                {t("coaching.calibrations.criterion")}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-medium">
                {t("coaching.calibrations.mean")}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-medium">
                {t("coaching.calibrations.variance")}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-medium">
                {t("coaching.calibrations.range")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {criteria.map((c) => {
              const flagged = (c.stats?.range ?? 0) >= SPREAD_FLAG;
              return (
                <tr key={c.key} className={cn(flagged && "bg-destructive/5")}>
                  <td className="px-3 py-2">
                    <span className="inline-flex items-center gap-1.5">
                      {flagged && (
                        <AlertTriangle
                          className="text-destructive size-3.5"
                          aria-label={t("coaching.calibrations.spread_flag")}
                        />
                      )}
                      {c.label}
                      {c.critical && (
                        <Badge variant="danger" className="ml-1">
                          {t("coaching.calibrations.critical")}
                        </Badge>
                      )}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {c.stats ? nf(c.stats.mean) : "—"}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {c.stats ? nf(c.stats.variance) : "—"}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {c.stats ? nf(c.stats.range) : "—"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="overflow-x-auto rounded-md border">
        <table className="w-full min-w-[34rem] text-sm">
          <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
            <tr>
              <th scope="col" className="px-3 py-2 font-medium">
                {t("coaching.calibrations.reviewer")}
              </th>
              {criteria.map((c) => (
                <th
                  key={c.key}
                  scope="col"
                  className="max-w-[8rem] truncate px-3 py-2 text-right font-medium"
                >
                  {c.label}
                </th>
              ))}
              <th scope="col" className="px-3 py-2 text-right font-medium">
                {t("coaching.calibrations.total")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {reviews.map((r) => {
              const values = new Map(r.scores.map((s) => [s.key, s]));
              return (
                <tr key={r.reviewer_uuid}>
                  <td className="max-w-[12rem] truncate px-3 py-2 font-medium">
                    {directory.memberName(r.reviewer_uuid)}
                  </td>
                  {criteria.map((c) => {
                    const s = values.get(c.key);
                    return (
                      <td
                        key={c.key}
                        className="px-3 py-2 text-right tabular-nums"
                        title={s?.rationale}
                      >
                        {s ? s.value : "—"}
                      </td>
                    );
                  })}
                  <td className="px-3 py-2 text-right font-semibold tabular-nums">
                    {nf(r.score)}
                    {r.critical_failure && (
                      <Badge variant="danger" className="ml-2">
                        {t("coaching.calibrations.critical_failure")}
                      </Badge>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <p className="text-muted-foreground text-xs">
        {t("coaching.calibrations.results_hint", { spread: SPREAD_FLAG })}
      </p>
    </div>
  );
}
