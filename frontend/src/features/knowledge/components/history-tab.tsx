"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, History, Undo2, Upload, XCircle } from "lucide-react";
import { useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { useActiveOrganization } from "@/features/organizations";
import { datetime, download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  historyKeys,
  historyService,
  isActive,
  type HistoryImport,
} from "../services/history.service";
import { HistorySettingsCard } from "./history-settings";
import { HistorySuggestions } from "./history-suggestions";

const statusVariant: Record<
  HistoryImport["status"],
  "default" | "secondary" | "success" | "warning" | "danger" | "outline"
> = {
  pending: "secondary",
  running: "default",
  completed: "success",
  failed: "danger",
  cancelled: "warning",
  rolled_back: "outline",
};

/**
 * Past records become knowledge (CF-217): upload an export with the data
 * owner's consent, follow each run, cancel or roll it back, and review the
 * suggestions that reached the evidence threshold.
 */
export function HistoryTab({
  canManage,
  canReview,
}: {
  canManage: boolean;
  canReview: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const slug = org?.slug ?? "";
  const runs = useQuery({
    queryKey: historyKeys.imports(slug),
    queryFn: () => historyService.imports(slug),
    enabled: Boolean(slug),
    refetchInterval: (q) => (q.state.data?.items.some(isActive) ? 3000 : false),
  });
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [analysis, setAnalysis] = useState(false);
  const [training, setTraining] = useState(false);
  const [rollingBack, setRollingBack] = useState<HistoryImport | null>(null);

  const refresh = () =>
    qc.invalidateQueries({ queryKey: historyKeys.all(slug) });
  const upload = useMutation({
    mutationFn: () =>
      historyService.upload(slug, file as File, { analysis, training }),
    onSuccess: async () => {
      setFile(null);
      setAnalysis(false);
      setTraining(false);
      if (fileRef.current) fileRef.current.value = "";
      await refresh();
      appToast.success(t("knowledge.history.uploaded"));
    },
  });
  const cancel = useMutation({
    mutationFn: (uuid: string) => historyService.cancel(slug, uuid),
    onSuccess: refresh,
  });
  const rollback = useMutation({
    mutationFn: (uuid: string) => historyService.rollback(slug, uuid),
    onSuccess: async () => {
      setRollingBack(null);
      await refresh();
      appToast.success(t("knowledge.history.rolled_back"));
    },
    onError: () => setRollingBack(null),
  });
  const sample = useMutation({
    mutationFn: () => historyService.sample(slug),
    onSuccess: ({ blob, filename }) =>
      download(blob, filename ?? "gecmis-ornek.json"),
  });

  return (
    <div className="space-y-6">
      <p className="text-muted-foreground max-w-3xl text-sm">
        {t("knowledge.history.description")}
      </p>

      {canManage && (
        <Card>
          <CardContent className="space-y-4 p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3 className="font-medium">{t("knowledge.history.upload")}</h3>
              <Button
                size="sm"
                variant="outline"
                onClick={() => sample.mutate()}
                disabled={sample.isPending}
              >
                <Download className="size-4" />
                {t("knowledge.history.sample")}
              </Button>
            </div>
            <Input
              ref={fileRef}
              type="file"
              accept=".json,.zip,application/json,application/zip"
              aria-label={t("knowledge.history.file")}
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
            <div className="space-y-2">
              <div className="flex items-start gap-2">
                <Checkbox
                  id="history-consent-analysis"
                  checked={analysis}
                  onCheckedChange={(v) => setAnalysis(v === true)}
                />
                <Label
                  htmlFor="history-consent-analysis"
                  className="leading-snug font-normal"
                >
                  {t("knowledge.history.consent_analysis")}
                </Label>
              </div>
              <div className="flex items-start gap-2">
                <Checkbox
                  id="history-consent-training"
                  checked={training}
                  onCheckedChange={(v) => setTraining(v === true)}
                />
                <Label
                  htmlFor="history-consent-training"
                  className="leading-snug font-normal"
                >
                  {t("knowledge.history.consent_training")}
                </Label>
              </div>
            </div>
            <p className="text-muted-foreground text-xs">
              {t("knowledge.history.privacy_note")}
            </p>
            <Button
              size="sm"
              disabled={!file || !analysis || upload.isPending}
              onClick={() => upload.mutate()}
            >
              <Upload className="size-4" />
              {t("knowledge.history.start")}
            </Button>
          </CardContent>
        </Card>
      )}

      <section className="space-y-3">
        <h3 className="font-medium">{t("knowledge.history.runs")}</h3>
        {runs.isLoading ? (
          <Loading label={t("common.loading")} />
        ) : runs.isError ? (
          <ErrorState
            title={t("knowledge.error.title")}
            description={t("knowledge.error.description")}
            onRetry={() => void runs.refetch()}
          />
        ) : !runs.data?.items.length ? (
          <EmptyState
            icon={<History className="text-signal size-5" />}
            title={t("knowledge.history.empty_title")}
            description={t("knowledge.history.empty_description")}
          />
        ) : (
          <div className="space-y-2">
            {runs.data.items.map((run) => (
              <RunRow
                key={run.uuid}
                run={run}
                canManage={canManage}
                onCancel={() => cancel.mutate(run.uuid)}
                onRollback={() => setRollingBack(run)}
              />
            ))}
          </div>
        )}
      </section>

      <HistorySuggestions canReview={canReview} />
      <HistorySettingsCard canManage={canManage} />

      <ConfirmDialog
        open={rollingBack !== null}
        title={t("knowledge.history.rollback_title")}
        description={t("knowledge.history.rollback_description")}
        confirmLabel={t("knowledge.history.rollback")}
        variant="destructive"
        isPending={rollback.isPending}
        onConfirm={() => rollingBack && rollback.mutate(rollingBack.uuid)}
        onCancel={() => setRollingBack(null)}
      />
    </div>
  );
}

function RunRow({
  run,
  canManage,
  onCancel,
  onRollback,
}: {
  run: HistoryImport;
  canManage: boolean;
  onCancel: () => void;
  onRollback: () => void;
}) {
  const { t, locale } = useLocale();
  const percent =
    run.total_records > 0
      ? Math.round(
          ((run.processed_records + run.skipped_records) / run.total_records) *
            100,
        )
      : run.status === "completed"
        ? 100
        : 0;
  const name =
    run.source === "nightly"
      ? t("knowledge.history.nightly_run")
      : run.file_name || t("knowledge.history.upload");
  return (
    <Card className="py-0">
      <CardContent className="space-y-2 p-4">
        <div className="flex flex-wrap items-center gap-2">
          <p className="min-w-0 flex-1 truncate font-medium">{name}</p>
          <Badge variant={statusVariant[run.status]}>
            {t(`knowledge.history.status.${run.status}`)}
          </Badge>
          {canManage && isActive(run) && !run.cancel_requested && (
            <Button size="sm" variant="ghost" onClick={onCancel}>
              <XCircle className="size-4" />
              {t("knowledge.history.cancel")}
            </Button>
          )}
          {canManage &&
            ["completed", "failed", "cancelled"].includes(run.status) && (
              <Button size="sm" variant="ghost" onClick={onRollback}>
                <Undo2 className="size-4" />
                {t("knowledge.history.rollback")}
              </Button>
            )}
        </div>
        {isActive(run) && (
          <div className="space-y-1">
            <Progress
              value={percent}
              aria-label={t("knowledge.history.progress")}
            />
            <p className="text-muted-foreground text-xs">
              {t(`knowledge.history.stage.${run.stage}`)} · {percent}%
            </p>
          </div>
        )}
        <p className="text-muted-foreground text-xs">
          {t("knowledge.history.counts", {
            processed: run.processed_records,
            total: run.total_records,
            skipped: run.skipped_records,
            suggestions: run.suggestions_created,
            suppressed: run.suggestions_suppressed,
          })}
        </p>
        <p className="text-muted-foreground text-xs">
          {datetime(run.created_at, undefined, locale)} ·{" "}
          {run.consent_training
            ? t("knowledge.history.training_yes")
            : t("knowledge.history.training_no")}{" "}
          ·{" "}
          {run.raw_deleted
            ? t("knowledge.history.raw_deleted")
            : t("knowledge.history.raw_kept")}
        </p>
        {run.error && <p className="text-destructive text-xs">{run.error}</p>}
      </CardContent>
    </Card>
  );
}
