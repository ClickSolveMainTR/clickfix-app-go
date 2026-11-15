"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Play, Plus } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import { Textarea } from "@/components/ui/textarea";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { aiErrorMessage } from "../lib/errors";
import { formatLatency, formatMicros } from "../lib/format";
import {
  aiInsightsService,
  type AIEvaluation,
  type AIEvaluationCase,
  type AIEvaluationCaseInput,
} from "../services/ai-insights.service";
import { percent } from "./insights-tab";
import { QueryState, SettingRow } from "./parts";
import type { AISettingsTabProps } from "./tab-registry";

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

type Draft = {
  name: string;
  question: string;
  category: string;
  locale: "tr" | "en";
  expectedAnswer: string;
  sources: string;
  handoff: "any" | "yes" | "no";
  active: boolean;
};

const emptyDraft: Draft = {
  name: "",
  question: "",
  category: "",
  locale: "tr",
  expectedAnswer: "",
  sources: "",
  handoff: "any",
  active: true,
};

function fromCase(c: AIEvaluationCase): Draft {
  const handoff = c.expected?.handoff;
  return {
    name: c.name,
    question: c.transcript.replace(/^Customer: /, ""),
    category: c.category ?? "",
    locale: c.locale,
    expectedAnswer: c.expected_answer,
    sources: c.expected_article_uuids.join("\n"),
    handoff: handoff === true ? "yes" : handoff === false ? "no" : "any",
    active: c.active,
  };
}

function toInput(d: Draft, base?: AIEvaluationCase): AIEvaluationCaseInput {
  const expected = { ...(base?.expected ?? {}) } as Record<string, unknown>;
  if (d.handoff === "any") delete expected.handoff;
  else expected.handoff = d.handoff === "yes";
  return {
    name: d.name.trim(),
    transcript: "",
    question: d.question.trim(),
    category: d.category.trim(),
    channel: base?.channel ?? "web",
    locale: d.locale,
    expected: expected as AIEvaluationCaseInput["expected"],
    expected_answer: d.expectedAnswer.trim(),
    expected_article_uuids: d.sources
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean),
    department_uuid: base?.department_uuid ?? undefined,
    active: d.active,
  };
}

function CaseDialog({
  slug,
  editing,
  open,
  onClose,
}: {
  slug: string;
  editing: AIEvaluationCase | null;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [draft, setDraft] = useState<Draft>(
    editing ? fromCase(editing) : emptyDraft,
  );
  const [saving, setSaving] = useState(false);
  const set = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft((prev) => ({ ...prev, [key]: value }));
  const input = toInput(draft, editing ?? undefined);
  const badSource = (input.expected_article_uuids ?? []).some(
    (s) => !UUID_RE.test(s),
  );
  const hasExpectation =
    input.expected_answer !== "" ||
    (input.expected_article_uuids ?? []).length > 0 ||
    draft.handoff !== "any" ||
    Object.keys(input.expected ?? {}).length > 0;
  const invalid =
    !input.name || !input.question || badSource || !hasExpectation;

  const save = async () => {
    setSaving(true);
    try {
      if (editing)
        await aiInsightsService.updateCase(slug, editing.uuid, input);
      else await aiInsightsService.createCase(slug, input);
      await qc.invalidateQueries({
        queryKey: ["tenant", slug, "ai", "evaluation-cases"],
      });
      appToast.success(t("ai_settings.evaluation.case_saved"));
      onClose();
    } catch (error) {
      appToast.error(aiErrorMessage(error, t));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {editing
              ? t("ai_settings.evaluation.edit_case")
              : t("ai_settings.evaluation.add_case")}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <SettingRow
            label={t("ai_settings.evaluation.name")}
            htmlFor="ev-name"
          >
            <Input
              id="ev-name"
              value={draft.name}
              maxLength={200}
              onChange={(e) => set("name", e.target.value)}
            />
          </SettingRow>
          <SettingRow
            label={t("ai_settings.evaluation.question")}
            htmlFor="ev-question"
          >
            <Textarea
              id="ev-question"
              value={draft.question}
              maxLength={17990}
              rows={3}
              onChange={(e) => set("question", e.target.value)}
            />
          </SettingRow>
          <SettingRow
            label={t("ai_settings.evaluation.expected_answer")}
            hint={t("ai_settings.evaluation.expected_answer_hint")}
            htmlFor="ev-answer"
          >
            <Textarea
              id="ev-answer"
              value={draft.expectedAnswer}
              maxLength={6000}
              rows={3}
              onChange={(e) => set("expectedAnswer", e.target.value)}
            />
          </SettingRow>
          <SettingRow
            label={t("ai_settings.evaluation.sources")}
            hint={t("ai_settings.evaluation.sources_hint")}
            htmlFor="ev-sources"
          >
            <Textarea
              id="ev-sources"
              value={draft.sources}
              rows={2}
              aria-invalid={badSource}
              onChange={(e) => set("sources", e.target.value)}
            />
          </SettingRow>
          <div className="grid gap-4 sm:grid-cols-3">
            <SettingRow
              label={t("ai_settings.evaluation.category")}
              htmlFor="ev-category"
            >
              <Input
                id="ev-category"
                value={draft.category}
                maxLength={100}
                onChange={(e) => set("category", e.target.value)}
              />
            </SettingRow>
            <SettingRow label={t("ai_settings.evaluation.locale")}>
              <Select
                value={draft.locale}
                onValueChange={(v) => set("locale", v as Draft["locale"])}
              >
                <SelectTrigger aria-label={t("ai_settings.evaluation.locale")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="tr">Türkçe</SelectItem>
                  <SelectItem value="en">English</SelectItem>
                </SelectContent>
              </Select>
            </SettingRow>
            <SettingRow label={t("ai_settings.evaluation.handoff")}>
              <Select
                value={draft.handoff}
                onValueChange={(v) => set("handoff", v as Draft["handoff"])}
              >
                <SelectTrigger aria-label={t("ai_settings.evaluation.handoff")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(["any", "yes", "no"] as const).map((v) => (
                    <SelectItem key={v} value={v}>
                      {t(`ai_settings.evaluation.handoff_${v}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </SettingRow>
          </div>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="ev-active">
              {t("ai_settings.evaluation.active")}
            </Label>
            <Switch
              id="ev-active"
              checked={draft.active}
              onCheckedChange={(v) => set("active", v)}
            />
          </div>
          {!hasExpectation && (
            <p className="text-muted-foreground text-xs">
              {t("ai_settings.evaluation.need_expectation")}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button disabled={invalid || saving} onClick={() => void save()}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function metric(run: AIEvaluation, key: keyof AIEvaluation, locale: string) {
  const v = run[key] as number | null | undefined;
  if (v === null || v === undefined) return "—";
  if (key === "avg_latency_ms")
    return formatLatency(Math.round(v), locale as "tr");
  if (key === "cost_per_case_micros") return formatMicros(v, locale as "tr");
  return percent(v, locale);
}

const METRICS = [
  "answer_accuracy",
  "source_hit_rate",
  "cache_hit_rate",
  "avg_latency_ms",
  "cost_per_case_micros",
] as const;

/**
 * "Ölçüm" tab (CF-220): the question set (question, expected answer or
 * sources, category), manual and automatic runs, and the run history with
 * the before/after metrics. Running spends AI credits for every case.
 */
export function EvaluationTab({ slug, canManage }: AISettingsTabProps) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const [dialog, setDialog] = useState<{
    editing: AIEvaluationCase | null;
  } | null>(null);
  const [running, setRunning] = useState(false);
  const cases = useQuery({
    queryKey: ["tenant", slug, "ai", "evaluation-cases"],
    queryFn: () => aiInsightsService.cases(slug, 0, 100),
  });
  const runs = useQuery({
    queryKey: ["tenant", slug, "ai", "evaluations"],
    queryFn: () => aiInsightsService.evaluations(slug, 0, 20),
  });
  const watch = useQuery({
    queryKey: ["tenant", slug, "ai", "evaluation-watch"],
    queryFn: () => aiInsightsService.watch(slug),
  });

  const run = async () => {
    setRunning(true);
    try {
      await aiInsightsService.run(slug);
      await qc.invalidateQueries({
        queryKey: ["tenant", slug, "ai", "evaluations"],
      });
      appToast.success(t("ai_settings.evaluation.run_done"));
    } catch (error) {
      appToast.error(aiErrorMessage(error, t));
    } finally {
      setRunning(false);
    }
  };
  const toggleWatch = async (enabled: boolean) => {
    try {
      const saved = await aiInsightsService.setWatch(slug, enabled);
      qc.setQueryData(["tenant", slug, "ai", "evaluation-watch"], saved);
    } catch (error) {
      appToast.error(aiErrorMessage(error, t));
    }
  };

  if (!cases.data || !runs.data) {
    return (
      <QueryState
        loading={cases.isLoading || runs.isLoading}
        error={cases.isError || runs.isError}
        onRetry={() => {
          void cases.refetch();
          void runs.refetch();
        }}
      />
    );
  }
  const activeCases = cases.data.items.filter((c) => c.active).length;

  return (
    <div className="space-y-6">
      <p className="text-muted-foreground text-sm">
        {t("ai_settings.evaluation.description")}
      </p>
      <Card>
        <CardContent className="flex flex-wrap items-center gap-4 pt-6">
          <div className="me-auto flex items-start gap-3">
            <Switch
              id="ev-watch"
              checked={watch.data?.enabled ?? false}
              disabled={!canManage || !watch.data}
              onCheckedChange={(v) => void toggleWatch(v)}
            />
            <div className="space-y-1">
              <Label htmlFor="ev-watch">
                {t("ai_settings.evaluation.watch")}
              </Label>
              <p className="text-muted-foreground text-xs">
                {t("ai_settings.evaluation.watch_hint")}
              </p>
            </div>
          </div>
          <Button
            disabled={!canManage || running || activeCases === 0}
            onClick={() => void run()}
          >
            <Play className="size-4" />
            {running
              ? t("ai_settings.evaluation.running")
              : t("ai_settings.evaluation.run", { count: activeCases })}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle className="text-base">
            {t("ai_settings.evaluation.cases_title", {
              count: cases.data.total,
            })}
          </CardTitle>
          {canManage && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setDialog({ editing: null })}
            >
              <Plus className="size-4" />
              {t("ai_settings.evaluation.add_case")}
            </Button>
          )}
        </CardHeader>
        <CardContent>
          {cases.data.items.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("ai_settings.evaluation.cases_empty")}
            </p>
          ) : (
            <ul className="divide-y">
              {cases.data.items.map((c) => (
                <li key={c.uuid} className="flex items-start gap-3 py-2">
                  <div className="min-w-0 flex-1">
                    <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                      <span className="truncate">{c.name}</span>
                      {c.category && (
                        <Badge variant="secondary">{c.category}</Badge>
                      )}
                      {!c.active && (
                        <Badge variant="outline">
                          {t("ai_settings.evaluation.inactive")}
                        </Badge>
                      )}
                    </p>
                    <p className="text-muted-foreground truncate text-xs">
                      {c.transcript.replace(/^Customer: /, "")}
                    </p>
                  </div>
                  {canManage && (
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("ai_settings.evaluation.edit_case")}
                      onClick={() => setDialog({ editing: c })}
                    >
                      <Pencil className="size-4" />
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("ai_settings.evaluation.history")}
          </CardTitle>
        </CardHeader>
        <CardContent className="overflow-x-auto">
          {runs.data.items.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("ai_settings.evaluation.history_empty")}
            </p>
          ) : (
            <table className="w-full min-w-[44rem] text-sm">
              <thead className="text-muted-foreground text-xs">
                <tr className="text-start">
                  <th className="py-2 text-start font-medium">
                    {t("ai_settings.evaluation.when")}
                  </th>
                  <th className="py-2 text-start font-medium">
                    {t("ai_settings.evaluation.trigger_label")}
                  </th>
                  {METRICS.map((m) => (
                    <th key={m} className="py-2 text-end font-medium">
                      {t(`ai_settings.evaluation.metric.${m}`)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y">
                {runs.data.items.map((r) => (
                  <tr key={r.uuid}>
                    <td className="py-2 whitespace-nowrap">
                      {datetime(r.created_at, undefined, locale)}
                      {r.status !== "completed" && (
                        <Badge variant="outline" className="ms-2">
                          {t(`ai_settings.evaluation.status.${r.status}`)}
                        </Badge>
                      )}
                    </td>
                    <td className="py-2">
                      {t(`ai_settings.evaluation.trigger.${r.trigger_kind}`)}
                      {r.model && (
                        <span className="text-muted-foreground block text-xs">
                          {r.model}
                        </span>
                      )}
                    </td>
                    {METRICS.map((m) => (
                      <td key={m} className="py-2 text-end tabular-nums">
                        {metric(r, m, locale)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardContent>
      </Card>

      {dialog && (
        <CaseDialog
          key={dialog.editing?.uuid ?? "new"}
          slug={slug}
          editing={dialog.editing}
          open
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  );
}
