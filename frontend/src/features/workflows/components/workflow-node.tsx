"use client";

import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import {
  AlertTriangle,
  CircleStop,
  Clock,
  GitBranch,
  GitFork,
  Play,
  Zap,
  type LucideIcon,
} from "lucide-react";
import { createContext, memo, useContext } from "react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  hasInput,
  ports,
  slug,
  splitDelay,
  type Condition,
  type Issue,
  type NodeKind,
  type WorkflowNode,
} from "../lib/graph";

export type FlowNodeData = { node: WorkflowNode };
export type FlowNode = Node<FlowNodeData, "wf">;

/** Per-node decorations kept outside node data so they never enter history. */
export type TraceMark = { status: string; port: string; order: number };
export const EditorDecorations = createContext<{
  issues: Record<string, Issue[]>;
  trace: Record<string, TraceMark>;
}>({ issues: {}, trace: {} });

export const KIND_ICON: Record<NodeKind, LucideIcon> = {
  trigger: Zap,
  condition: GitBranch,
  branch: GitFork,
  action: Play,
  wait: Clock,
  end: CircleStop,
};

/** Accent per kind; works in light and dark through opacity. */
export const KIND_ACCENT: Record<NodeKind, string> = {
  trigger: "border-l-violet-500 [&_[data-kind-icon]]:text-violet-500",
  condition: "border-l-sky-500 [&_[data-kind-icon]]:text-sky-500",
  branch: "border-l-cyan-500 [&_[data-kind-icon]]:text-cyan-500",
  action: "border-l-emerald-500 [&_[data-kind-icon]]:text-emerald-500",
  wait: "border-l-amber-500 [&_[data-kind-icon]]:text-amber-500",
  end: "border-l-zinc-500 [&_[data-kind-icon]]:text-zinc-500",
};

function conditionSummary(
  c: Condition | undefined,
  t: (k: string, p?: Record<string, string | number>) => string,
): string {
  if (!c) return "—";
  if (c.all || c.any) {
    const list = c.all ?? c.any ?? [];
    if (list.length === 1) return conditionSummary(list[0], t);
    return t(c.all ? "workflows.summary.all" : "workflows.summary.any", {
      count: list.length,
    });
  }
  const value = Array.isArray(c.value)
    ? c.value.join(", ")
    : c.value === undefined
      ? ""
      : String(c.value);
  return `${c.field} ${t(`workflows.operators.${c.operator ?? "eq"}`)} ${value}`.trim();
}

export function useNodeSummary() {
  const { t } = useLocale();
  return (n: WorkflowNode): string => {
    switch (n.type) {
      case "trigger":
        return n.trigger === "scheduled"
          ? t("workflows.summary.every_minutes", {
              count: n.schedule_minutes ?? 0,
            })
          : t(`workflows.triggers.${slug(n.trigger ?? "")}`);
      case "condition":
      case "branch":
        return conditionSummary(n.condition, t);
      case "action": {
        const label = t(`workflows.actions.${n.action?.type ?? "add_note"}`);
        const detail = n.action?.value || n.action?.content || "";
        return detail && n.action?.type !== "route"
          ? `${label}: ${detail.slice(0, 40)}`
          : label;
      }
      case "wait": {
        const { amount, unit } = splitDelay(n.delay_seconds ?? 0);
        return t(`workflows.summary.wait_${unit}`, { count: amount });
      }
      default:
        return t("workflows.summary.end");
    }
  };
}

const PORT_TONE: Record<string, string> = {
  next: "!bg-foreground",
  true: "!bg-emerald-500",
  false: "!bg-rose-500",
};

function WorkflowNodeView({ id, data, selected }: NodeProps<FlowNode>) {
  const { t } = useLocale();
  const { issues, trace } = useContext(EditorDecorations);
  const summary = useNodeSummary();
  const n = data.node;
  const Icon = KIND_ICON[n.type];
  const nodeIssues = issues[id] ?? [];
  const mark = trace[id];
  const out = ports(n.type);
  return (
    <div
      className={cn(
        "bg-card text-card-foreground relative w-56 rounded-lg border border-l-4 shadow-sm transition-shadow",
        KIND_ACCENT[n.type],
        selected && "ring-ring ring-2 ring-offset-1 ring-offset-transparent",
        nodeIssues.length > 0 && "border-destructive",
        mark && "ring-2 ring-emerald-500/70",
        mark?.status === "failed" && "ring-rose-500/80",
        mark?.status === "uncertain" && "ring-amber-500/80",
      )}
      data-testid={`wf-node-${n.type}`}
    >
      {hasInput(n.type) && (
        <Handle
          type="target"
          id="in"
          position={Position.Left}
          className="!bg-muted-foreground !size-2.5 !border-2 !border-[var(--card)]"
          aria-label={t("workflows.ports.in")}
        />
      )}
      <div className="flex items-center gap-2 px-3 pt-2">
        <Icon data-kind-icon className="size-3.5 shrink-0" />
        <span className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
          {t(`workflows.kinds.${n.type}`)}
        </span>
        {mark && (
          <span className="ml-auto rounded bg-emerald-500/15 px-1.5 text-[10px] font-medium text-emerald-700 tabular-nums dark:text-emerald-300">
            {mark.order}
          </span>
        )}
        {nodeIssues.length > 0 && (
          <span
            className={cn("text-destructive", !mark && "ml-auto")}
            title={nodeIssues.map((i) => t(i.key, i.params)).join("\n")}
          >
            <AlertTriangle className="size-3.5" />
            <span className="sr-only">
              {t("workflows.issues.count", { count: nodeIssues.length })}
            </span>
          </span>
        )}
      </div>
      <p className="line-clamp-2 px-3 pt-0.5 pb-2 text-xs leading-snug break-words">
        {summary(n)}
      </p>
      {nodeIssues.length > 0 && (
        <p className="border-destructive/30 bg-destructive/5 text-destructive line-clamp-2 border-t px-3 py-1 text-[11px]">
          {t(nodeIssues[0].key, nodeIssues[0].params)}
        </p>
      )}
      {out.length === 1 && (
        <Handle
          type="source"
          id={out[0]}
          position={Position.Right}
          className={cn(
            "!size-2.5 !border-2 !border-[var(--card)]",
            PORT_TONE.next,
          )}
          aria-label={t("workflows.ports.next")}
        />
      )}
      {out.length === 2 && (
        <>
          {out.map((port, i) => (
            <Handle
              key={port}
              type="source"
              id={port}
              position={Position.Right}
              style={{ top: i === 0 ? "35%" : "75%" }}
              className={cn(
                "!size-2.5 !border-2 !border-[var(--card)]",
                PORT_TONE[port],
              )}
              aria-label={t(`workflows.ports.${port}`)}
            />
          ))}
          <span className="text-muted-foreground pointer-events-none absolute top-[35%] right-2 -translate-y-1/2 text-[10px]">
            {t("workflows.ports.true")}
          </span>
          <span className="text-muted-foreground pointer-events-none absolute top-[75%] right-2 -translate-y-1/2 text-[10px]">
            {t("workflows.ports.false")}
          </span>
        </>
      )}
      {mark?.port && out.length === 2 && (
        <span className="sr-only">
          {t("workflows.test.took_port", { port: mark.port })}
        </span>
      )}
    </div>
  );
}

export const WorkflowNodeComponent = memo(WorkflowNodeView);
export const nodeTypes = { wf: WorkflowNodeComponent };
