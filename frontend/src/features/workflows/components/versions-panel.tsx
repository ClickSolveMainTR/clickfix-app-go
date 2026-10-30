"use client";

import { History, RotateCcw } from "lucide-react";

import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useWorkflowVersions } from "../hooks/use-workflows";
import {
  diffGraphs,
  isEmptyDiff,
  slug,
  type GraphDiff,
  type WorkflowGraph,
} from "../lib/graph";

/** One-line "+2 nodes, 1 changed, 1 edge removed" summary. */
export function DiffSummary({ diff }: { diff: GraphDiff }) {
  const { t } = useLocale();
  if (isEmptyDiff(diff))
    return (
      <span className="text-muted-foreground text-xs">
        {t("workflows.diff.none")}
      </span>
    );
  const parts = [
    diff.added.length &&
      t("workflows.diff.added", { count: diff.added.length }),
    diff.removed.length &&
      t("workflows.diff.removed", { count: diff.removed.length }),
    diff.changed.length &&
      t("workflows.diff.changed", { count: diff.changed.length }),
    (diff.edgesAdded || diff.edgesRemoved) &&
      t("workflows.diff.edges", {
        added: diff.edgesAdded,
        removed: diff.edgesRemoved,
      }),
  ].filter(Boolean);
  return <span className="text-xs">{parts.join(" · ")}</span>;
}

/** Detailed list used by the publish confirmation. */
export function DiffDetails({
  diff,
  label,
}: {
  diff: GraphDiff;
  label: (id: string) => string;
}) {
  const { t } = useLocale();
  const rows = [
    ...diff.added.map((n) => ({ id: n.id, kind: "added", node: n })),
    ...diff.removed.map((n) => ({ id: n.id, kind: "removed", node: n })),
    ...diff.changed.map((n) => ({ id: n.id, kind: "changed", node: n })),
  ];
  return (
    <div className="space-y-2">
      <DiffSummary diff={diff} />
      {rows.length > 0 && (
        <ul className="max-h-48 space-y-1 overflow-y-auto text-xs">
          {rows.map((r) => (
            <li key={`${r.kind}-${r.id}`} className="flex items-center gap-2">
              <Badge
                variant={
                  r.kind === "added"
                    ? "success"
                    : r.kind === "removed"
                      ? "danger"
                      : "warning"
                }
              >
                {t(`workflows.diff.kind_${r.kind}`)}
              </Badge>
              <span className="text-muted-foreground">
                {t(`workflows.kinds.${r.node.type}`)}
              </span>
              <span className="truncate">{label(r.id)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * Immutable published versions, newest first. Each row shows how the
 * current draft differs from it; "restore" copies its graph into the
 * editor as an undoable change (it still has to be saved and published).
 */
export function VersionsPanel({
  uuid,
  draft,
  publishedVersion,
  readOnly,
  onRestore,
}: {
  uuid: string;
  draft: WorkflowGraph;
  publishedVersion: number | null;
  readOnly: boolean;
  onRestore: (graph: WorkflowGraph) => void;
}) {
  const { t } = useLocale();
  const versions = useWorkflowVersions(uuid);
  if (versions.isLoading) return <Loading label={t("common.loading")} />;
  if (versions.isError)
    return (
      <p className="text-destructive text-sm">
        {t("workflows.versions.error")}
      </p>
    );
  const items = [...(versions.data ?? [])].sort(
    (a, b) => b.version - a.version,
  );
  if (items.length === 0)
    return (
      <div className="text-muted-foreground flex flex-col items-center gap-2 py-8 text-center text-sm">
        <History className="size-6" />
        {t("workflows.versions.empty")}
      </div>
    );
  return (
    <ul className="space-y-2">
      {items.map((v) => (
        <li key={v.uuid} className="space-y-1.5 rounded-md border p-2.5">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold">v{v.version}</span>
            {v.version === publishedVersion && (
              <Badge variant="success">{t("workflows.versions.current")}</Badge>
            )}
            <span className="text-muted-foreground ml-auto text-xs">
              {datetime(v.created_at, "dd.MM.yyyy HH:mm")}
            </span>
          </div>
          <p className="text-muted-foreground text-xs">
            {t(`workflows.triggers.${slug(v.trigger_event)}`)}
            {v.schedule_minutes > 0 &&
              ` · ${t("workflows.summary.every_minutes", { count: v.schedule_minutes })}`}
          </p>
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">
              {t("workflows.versions.vs_draft")}
            </span>
            <DiffSummary diff={diffGraphs(v.graph, draft)} />
          </div>
          {!readOnly && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-7 px-2"
              onClick={() => onRestore(v.graph)}
            >
              <RotateCcw className="size-3.5" />
              {t("workflows.versions.restore")}
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}
