"use client";

import { useQuery } from "@tanstack/react-query";
import { Eye, ListTree } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { workflowKeys } from "../hooks/use-workflows";
import { workflowsService } from "../services/workflows.service";
import { RunStatus } from "./run-status";
import type { TraceMark } from "./workflow-node";

const PAGE = 20;

/** Run history of the workflow; a row opens the per-step log. */
export function RunsPanel({
  uuid,
  onHighlight,
}: {
  uuid: string;
  onHighlight: (trace: Record<string, TraceMark>) => void;
}) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const [limit, setLimit] = useState(PAGE);
  const [open, setOpen] = useState<string | null>(null);
  const runs = useQuery({
    queryKey: workflowKeys.runs(org?.slug, uuid, limit),
    queryFn: () => workflowsService.runs(uuid, limit, 0),
    enabled: Boolean(org),
    refetchInterval: 30_000,
  });
  if (runs.isLoading) return <Loading label={t("common.loading")} />;
  if (runs.isError)
    return (
      <p className="text-destructive text-sm">{t("workflows.runs.error")}</p>
    );
  const items = runs.data?.items ?? [];
  return (
    <div className="space-y-2">
      {items.length === 0 && (
        <div className="text-muted-foreground flex flex-col items-center gap-2 py-8 text-center text-sm">
          <ListTree className="size-6" />
          {t("workflows.runs.empty")}
        </div>
      )}
      <ul className="space-y-1.5">
        {items.map((r) => (
          <li key={r.uuid}>
            <button
              type="button"
              onClick={() => setOpen(r.uuid)}
              className="hover:bg-muted flex w-full flex-col gap-1 rounded-md border px-2.5 py-2 text-left"
            >
              <span className="flex items-center gap-2">
                <RunStatus status={r.status} />
                <span className="text-muted-foreground ml-auto text-xs">
                  {datetime(r.created_at, "dd.MM HH:mm:ss")}
                </span>
              </span>
              <span className="text-muted-foreground truncate font-mono text-[10px]">
                {r.conversation_uuid}
              </span>
              {r.error && (
                <span className="text-destructive line-clamp-2 text-xs">
                  {r.error}
                </span>
              )}
            </button>
          </li>
        ))}
      </ul>
      {(runs.data?.total ?? 0) > items.length && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="w-full"
          onClick={() => setLimit((l) => l + PAGE)}
        >
          {t("workflows.runs.more")}
        </Button>
      )}
      {open && (
        <RunDialog
          run={open}
          onClose={() => setOpen(null)}
          onHighlight={(trace) => {
            onHighlight(trace);
            setOpen(null);
          }}
        />
      )}
    </div>
  );
}

function RunDialog({
  run,
  onClose,
  onHighlight,
}: {
  run: string;
  onClose: () => void;
  onHighlight: (trace: Record<string, TraceMark>) => void;
}) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const detail = useQuery({
    queryKey: workflowKeys.run(org?.slug, run),
    queryFn: () => workflowsService.run(run),
    enabled: Boolean(org),
    retry: false,
  });
  const steps = [...(detail.data?.steps ?? [])].sort(
    (a, b) => a.step_index - b.step_index,
  );
  const nodes = new Map(
    (detail.data?.version.graph.nodes ?? []).map((n) => [n.id, n]),
  );
  const highlight = () => {
    const graph = detail.data?.version.graph;
    const trace: Record<string, TraceMark> = {};
    steps.forEach((s, i) => {
      const next = steps[i + 1]?.node_id;
      const port =
        graph?.edges.find((e) => e.from === s.node_id && e.to === next)?.port ??
        "";
      trace[s.node_id] = { status: s.status, port, order: i + 1 };
    });
    onHighlight(trace);
  };
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("workflows.runs.detail")}
      description={
        detail.data
          ? t("workflows.runs.detail_description", {
              version: detail.data.version.version,
            })
          : undefined
      }
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
    >
      {detail.isLoading && <Loading label={t("common.loading")} />}
      {detail.isError && (
        <p className="text-destructive text-sm">
          {isApiError(detail.error) && detail.error.status === 403
            ? t("workflows.runs.forbidden")
            : t("workflows.runs.error")}
        </p>
      )}
      {detail.data && (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <RunStatus status={detail.data.run.status} />
            <Link
              className="text-primary text-xs underline-offset-2 hover:underline"
              href={`/inbox?c=${detail.data.run.conversation_uuid}`}
            >
              {t("workflows.runs.open_conversation")}
            </Link>
            <span className="text-muted-foreground ml-auto text-xs">
              {datetime(detail.data.run.created_at, "dd.MM.yyyy HH:mm:ss")}
            </span>
          </div>
          {detail.data.run.status === "waiting" && (
            <p className="text-muted-foreground text-xs">
              {t("workflows.runs.due", {
                at: datetime(detail.data.run.due_at, "dd.MM.yyyy HH:mm"),
              })}
            </p>
          )}
          {detail.data.run.error && (
            <p className="border-destructive/40 bg-destructive/5 text-destructive rounded-md border p-2 text-xs">
              {detail.data.run.error}
            </p>
          )}
          <ol className="space-y-2">
            {steps.map((s) => {
              const node = nodes.get(s.node_id);
              const output =
                s.output && Object.keys(s.output).length > 0
                  ? JSON.stringify(s.output, null, 2)
                  : "";
              return (
                <li key={s.uuid} className="rounded-md border p-2.5">
                  <div className="flex items-center gap-2 text-sm">
                    <span className="text-muted-foreground w-6 text-xs tabular-nums">
                      {s.step_index + 1}.
                    </span>
                    <span className="font-medium">
                      {node ? t(`workflows.kinds.${node.type}`) : s.node_id}
                    </span>
                    <code className="text-muted-foreground truncate text-[10px]">
                      {s.node_id}
                    </code>
                    <span className="ml-auto">
                      <RunStatus status={s.status} />
                    </span>
                  </div>
                  {s.status === "uncertain" && (
                    <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">
                      {t("workflows.runs.uncertain")}
                    </p>
                  )}
                  {s.error && (
                    <p className="text-destructive mt-1 text-xs">{s.error}</p>
                  )}
                  {output && (
                    <pre className="bg-muted mt-2 max-h-40 overflow-auto rounded p-2 text-[11px]">
                      {output}
                    </pre>
                  )}
                </li>
              );
            })}
          </ol>
          {steps.length === 0 && (
            <p className="text-muted-foreground text-sm">
              {t("workflows.runs.no_steps")}
            </p>
          )}
        </div>
      )}
      <FormActions sticky={false}>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.close")}
        </Button>
        {steps.length > 0 && (
          <Button type="button" onClick={highlight}>
            <Eye className="size-4" />
            {t("workflows.runs.highlight")}
          </Button>
        )}
      </FormActions>
    </AppDialog>
  );
}
