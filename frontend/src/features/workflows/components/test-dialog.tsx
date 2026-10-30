"use client";

import { useMutation } from "@tanstack/react-query";
import { FlaskConical, Search } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useDebounce } from "@/hooks/use-debounce";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useSampleConversations } from "../hooks/use-workflows";
import { isUuid, type WorkflowGraph } from "../lib/graph";
import {
  workflowsService,
  type WorkflowTrace,
} from "../services/workflows.service";
import { RunStatus } from "./run-status";
import type { TraceMark } from "./workflow-node";

/**
 * "Test et": dry-run of the saved draft on a sample conversation. The API
 * simulates deterministic mutations and stops at a wait; nothing is written
 * and external actions are only reported as "would execute".
 */
export function TestDialog({
  uuid,
  graph,
  label,
  onClose,
  onTrace,
}: {
  uuid: string;
  graph: WorkflowGraph;
  label: (id: string) => string;
  onClose: () => void;
  onTrace: (trace: Record<string, TraceMark>) => void;
}) {
  const { t } = useLocale();
  const [q, setQ] = useState("");
  const debounced = useDebounce(q, 300);
  const [conversation, setConversation] = useState("");
  const [result, setResult] = useState<WorkflowTrace | null>(null);
  const samples = useSampleConversations(isUuid(debounced) ? "" : debounced);
  const run = useMutation({
    mutationFn: (c: string) => workflowsService.test(uuid, c),
    onSuccess: (data) => {
      setResult(data);
      const marks: Record<string, TraceMark> = {};
      data.trace.forEach((s, i) => {
        marks[s.node_id] = { status: s.status, port: s.port, order: i + 1 };
      });
      onTrace(marks);
    },
  });
  const selected = isUuid(q.trim()) ? q.trim() : conversation;
  const kinds = new Map(graph.nodes.map((n) => [n.id, n.type]));

  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("workflows.test.title")}
      description={t("workflows.test.description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
    >
      <div className="space-y-4">
        <div className="relative">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={q}
            className="pl-8"
            placeholder={t("workflows.test.search")}
            aria-label={t("workflows.test.search")}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        {!isUuid(q.trim()) && (
          <ul
            className="max-h-56 space-y-1 overflow-y-auto"
            aria-label={t("workflows.test.samples")}
          >
            {(samples.data ?? []).map((c) => (
              <li key={c.uuid}>
                <button
                  type="button"
                  onClick={() => setConversation(c.uuid)}
                  aria-pressed={conversation === c.uuid}
                  className={cn(
                    "hover:bg-muted flex w-full items-center gap-2 rounded-md border px-2.5 py-1.5 text-left text-sm",
                    conversation === c.uuid && "border-primary bg-primary/5",
                  )}
                >
                  <span className="text-muted-foreground text-xs tabular-nums">
                    #{c.number}
                  </span>
                  <span className="truncate">
                    {c.subject ||
                      c.contact?.name ||
                      c.contact?.email ||
                      t("workflows.test.no_subject")}
                  </span>
                  <span className="text-muted-foreground ml-auto text-xs">
                    {t(`workflows.statuses.${c.status}`)}
                  </span>
                </button>
              </li>
            ))}
            {samples.isSuccess && (samples.data ?? []).length === 0 && (
              <li className="text-muted-foreground py-4 text-center text-sm">
                {t("workflows.test.no_samples")}
              </li>
            )}
          </ul>
        )}
        {run.isError && (
          <p className="text-destructive text-sm" role="alert">
            {isApiError(run.error)
              ? run.error.message
              : t("workflows.toast.failed")}
          </p>
        )}
        {result && (
          <div className="space-y-2">
            <h4 className="text-sm font-semibold">
              {t("workflows.test.trace")}
            </h4>
            <ol className="space-y-1">
              {result.trace.map((s, i) => (
                <li
                  key={`${s.node_id}-${i}`}
                  className="flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm"
                >
                  <span className="text-muted-foreground w-5 text-xs tabular-nums">
                    {i + 1}.
                  </span>
                  <span className="text-muted-foreground text-xs">
                    {t(`workflows.kinds.${kinds.get(s.node_id) ?? s.type}`)}
                  </span>
                  <span className="min-w-0 flex-1 truncate">
                    {label(s.node_id)}
                  </span>
                  {(s.port === "true" || s.port === "false") && (
                    <span className="text-muted-foreground text-xs">
                      → {t(`workflows.ports.${s.port}`)}
                    </span>
                  )}
                  <RunStatus status={s.status} />
                </li>
              ))}
            </ol>
            <p className="text-muted-foreground text-xs">
              {t("workflows.test.hypothetical")}
            </p>
          </div>
        )}
      </div>
      <FormActions sticky={false}>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.close")}
        </Button>
        <Button
          type="button"
          disabled={!selected || run.isPending}
          onClick={() => run.mutate(selected)}
        >
          <FlaskConical className="size-4" />
          {run.isPending
            ? t("workflows.test.running")
            : t("workflows.test.run")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
