"use client";

import "@xyflow/react/dist/style.css";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  applyEdgeChanges,
  applyNodeChanges,
  useReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type IsValidConnection,
  type NodeChange,
} from "@xyflow/react";
import {
  AlertTriangle,
  ArrowLeft,
  Eraser,
  FlaskConical,
  Keyboard,
  LayoutGrid,
  Redo2,
  Rocket,
  Save,
  Undo2,
} from "lucide-react";
import { useTheme } from "next-themes";
import Link from "next/link";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type DragEvent,
} from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { useUnsavedChangesGuard } from "@/hooks/use-unsaved-changes-guard";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useWorkflow,
  useWorkflowVersions,
  workflowKeys,
} from "../hooks/use-workflows";
import {
  NODE_KINDS,
  autoLayout,
  createNode,
  createsCycle,
  diffGraphs,
  hasInput,
  validateGraph,
  type NodeKind,
  type WorkflowEdge,
  type WorkflowGraph,
  type WorkflowNode,
} from "../lib/graph";
import { workflowsService, type Workflow } from "../services/workflows.service";
import { NodePanel } from "./node-panel";
import { RunsPanel } from "./runs-panel";
import { TestDialog } from "./test-dialog";
import { DiffDetails, VersionsPanel } from "./versions-panel";
import {
  EditorDecorations,
  KIND_ICON,
  nodeTypes,
  useNodeSummary,
  type FlowNode,
  type TraceMark,
} from "./workflow-node";

type FlowEdge = Edge<{ loop: boolean }>;
type PanelTab = "node" | "issues" | "versions" | "runs";

const DND_TYPE = "application/x-clickfix-workflow-node";
const HISTORY_LIMIT = 100;
/** Typing in one node's form within this window is a single undo step. */
const COALESCE_MS = 800;

const MINIMAP_COLOR: Record<NodeKind, string> = {
  trigger: "#8b5cf6",
  condition: "#0ea5e9",
  branch: "#06b6d4",
  action: "#10b981",
  wait: "#f59e0b",
  end: "#71717a",
};

function toFlowNodes(g: WorkflowGraph, previous: FlowNode[] = []): FlowNode[] {
  const prev = new Map(previous.map((n) => [n.id, n]));
  return g.nodes.map((n, i) => {
    const node = { ...n };
    delete node.position;
    const old = prev.get(n.id);
    return {
      id: n.id,
      type: "wf",
      position: {
        x: n.position?.x ?? i * 280,
        y: n.position?.y ?? 0,
      },
      data: { node },
      deletable: n.type !== "trigger",
      // Keep measured size so restored nodes stay visible without re-measuring.
      ...(old?.measured ? { measured: old.measured } : {}),
      ...(old?.selected ? { selected: true } : {}),
    };
  });
}

function edgeId(e: WorkflowEdge) {
  return `${e.from}:${e.port}`;
}

function toFlowEdge(e: WorkflowEdge): FlowEdge {
  return {
    id: edgeId(e),
    source: e.from,
    sourceHandle: e.port,
    target: e.to,
    targetHandle: "in",
    data: { loop: Boolean(e.loop) },
  };
}

function toGraph(nodes: FlowNode[], edges: FlowEdge[]): WorkflowGraph {
  return {
    schema_version: 1,
    nodes: nodes.map((n) => ({
      ...n.data.node,
      position: { x: Math.round(n.position.x), y: Math.round(n.position.y) },
    })),
    edges: edges.map((e) => {
      const edge: WorkflowEdge = {
        from: e.source,
        port: (e.sourceHandle ?? "next") as WorkflowEdge["port"],
        to: e.target,
        input: "in",
      };
      if (e.data?.loop) edge.loop = true;
      return edge;
    }),
  };
}

const serialize = (name: string, g: WorkflowGraph) =>
  JSON.stringify({ name, g });

function isTyping(target: EventTarget | null) {
  const el = target as HTMLElement | null;
  if (!el) return false;
  return (
    el.isContentEditable ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName) ||
    el.getAttribute("role") === "combobox"
  );
}

/** Route entry: loads the workflow, then mounts the canvas editor. */
export function WorkflowEditorPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const workflow = useWorkflow(uuid);
  if (workflow.isLoading) return <Loading label={t("common.loading")} />;
  if (workflow.isError || !workflow.data)
    return (
      <ErrorState
        title={t("workflows.error.title")}
        description={t("workflows.error.description")}
        onRetry={() => void workflow.refetch()}
      />
    );
  return (
    <ReactFlowProvider>
      <Editor key={workflow.data.uuid} initial={workflow.data} />
    </ReactFlowProvider>
  );
}

function Editor({ initial }: { initial: Workflow }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { resolvedTheme } = useTheme();
  const { active: org } = useActiveOrganization();
  const canWrite = (org?.permissions ?? []).includes(
    Permission.TenantAutomationsWrite,
  );
  const readOnly = !canWrite;
  const flow = useReactFlow<FlowNode, FlowEdge>();
  const summary = useNodeSummary();
  const uuid = initial.uuid;

  const [meta, setMeta] = useState(initial);
  const [name, setName] = useState(initial.name);
  const [nodes, setNodes] = useState<FlowNode[]>(() =>
    toFlowNodes(initial.draft),
  );
  const [edges, setEdges] = useState<FlowEdge[]>(() =>
    initial.draft.edges.map(toFlowEdge),
  );
  const [saved, setSaved] = useState(() =>
    serialize(
      initial.name,
      toGraph(toFlowNodes(initial.draft), initial.draft.edges.map(toFlowEdge)),
    ),
  );
  const [tab, setTab] = useState<PanelTab>("node");
  const [serverError, setServerError] = useState<string | null>(null);
  const [trace, setTrace] = useState<Record<string, TraceMark>>({});
  const [testing, setTesting] = useState(false);
  const [publishing, setPublishing] = useState(false);

  const graph = useMemo(() => toGraph(nodes, edges), [nodes, edges]);
  const validation = useMemo(() => validateGraph(graph), [graph]);
  const dirty = serialize(name, graph) !== saved;
  const selected = nodes.find((n) => n.selected) ?? null;
  const versions = useWorkflowVersions(uuid);

  // History holds whole graph snapshots; selection and measuring never enter it.
  const graphRef = useRef(graph);
  useEffect(() => {
    graphRef.current = graph;
  }, [graph]);
  const history = useRef<{ past: WorkflowGraph[]; future: WorkflowGraph[] }>({
    past: [],
    future: [],
  });
  const lastCommit = useRef<{ tag: string; at: number } | null>(null);
  const dragStart = useRef<WorkflowGraph | null>(null);
  const [depth, setDepth] = useState({ past: 0, future: 0 });
  const bump = () =>
    setDepth({
      past: history.current.past.length,
      future: history.current.future.length,
    });

  const pushPast = useCallback((g: WorkflowGraph) => {
    const h = history.current;
    h.past = [...h.past, g].slice(-HISTORY_LIMIT);
    h.future = [];
    bump();
  }, []);
  /** Records the current graph before a change; same-tag edits coalesce. */
  const commit = useCallback(
    (tag = "") => {
      const now = Date.now();
      const last = lastCommit.current;
      lastCommit.current = { tag, at: now };
      if (tag && last && last.tag === tag && now - last.at < COALESCE_MS)
        return;
      pushPast(graphRef.current);
    },
    [pushPast],
  );
  const applyGraph = useCallback((g: WorkflowGraph) => {
    setNodes((prev) => toFlowNodes(g, prev));
    setEdges(g.edges.map(toFlowEdge));
  }, []);
  const undo = useCallback(() => {
    const h = history.current;
    const prev = h.past.at(-1);
    if (!prev) return;
    h.past = h.past.slice(0, -1);
    h.future = [graphRef.current, ...h.future];
    lastCommit.current = null;
    applyGraph(prev);
    bump();
  }, [applyGraph]);
  const redo = useCallback(() => {
    const h = history.current;
    const next = h.future[0];
    if (!next) return;
    h.future = h.future.slice(1);
    h.past = [...h.past, graphRef.current];
    lastCommit.current = null;
    applyGraph(next);
    bump();
  }, [applyGraph]);

  const onNodesChange = useCallback(
    (changes: NodeChange<FlowNode>[]) =>
      setNodes((ns) => applyNodeChanges(changes, ns)),
    [],
  );
  const onEdgesChange = useCallback(
    (changes: EdgeChange<FlowEdge>[]) =>
      setEdges((es) => applyEdgeChanges(changes, es)),
    [],
  );

  const nodeType = useCallback(
    (id: string | null | undefined) =>
      graphRef.current.nodes.find((n) => n.id === id)?.type,
    [],
  );
  const isValidConnection: IsValidConnection<FlowEdge> = useCallback(
    (c) => {
      if (!c.source || !c.target || c.source === c.target) return false;
      if (c.targetHandle !== "in") return false;
      const target = nodeType(c.target);
      if (!target || !hasInput(target)) return false;
      const others = graphRef.current.edges.filter(
        (e) => !(e.from === c.source && e.port === c.sourceHandle),
      );
      // Cycles are only allowed as an explicit loop leaving a wait.
      return (
        nodeType(c.source) === "wait" ||
        !createsCycle(others, c.source, c.target)
      );
    },
    [nodeType],
  );
  const onConnect = useCallback(
    (c: Connection) => {
      if (readOnly || !isValidConnection(c)) return;
      commit();
      setEdges((es) => {
        const others = es.filter(
          (e) => !(e.source === c.source && e.sourceHandle === c.sourceHandle),
        );
        const loop =
          nodeType(c.source) === "wait" &&
          createsCycle(
            graphRef.current.edges.filter(
              (e) => !(e.from === c.source && e.port === c.sourceHandle),
            ),
            c.source,
            c.target,
          );
        return [
          ...others,
          toFlowEdge({
            from: c.source,
            port: (c.sourceHandle ?? "next") as WorkflowEdge["port"],
            to: c.target,
            input: "in",
            ...(loop ? { loop: true } : {}),
          }),
        ];
      });
    },
    [commit, isValidConnection, nodeType, readOnly],
  );

  const addNode = useCallback(
    (kind: NodeKind, position?: { x: number; y: number }) => {
      if (readOnly) return;
      const wrapper = document.querySelector<HTMLElement>(".react-flow");
      const rect = wrapper?.getBoundingClientRect();
      const at =
        position ??
        flow.screenToFlowPosition({
          x: (rect?.left ?? 0) + (rect?.width ?? 600) / 2,
          y: (rect?.top ?? 0) + (rect?.height ?? 400) / 2,
        });
      const node = createNode(kind, { x: at.x - 112, y: at.y - 30 });
      commit();
      setNodes((ns) => [
        ...ns.map((n) => (n.selected ? { ...n, selected: false } : n)),
        { ...toFlowNodes({ ...graph, nodes: [node] })[0], selected: true },
      ]);
      setTab("node");
    },
    [commit, flow, graph, readOnly],
  );
  const updateNode = useCallback(
    (next: WorkflowNode) => {
      commit(`node:${next.id}`);
      setNodes((ns) =>
        ns.map((n) => {
          if (n.id !== next.id) return n;
          const node = { ...next };
          delete node.position;
          return { ...n, data: { node } };
        }),
      );
    },
    [commit],
  );
  const relayout = useCallback(() => {
    if (readOnly) return;
    commit();
    applyGraph(autoLayout(graphRef.current));
    requestAnimationFrame(
      () => void flow.fitView({ padding: 0.2, duration: 200 }),
    );
  }, [applyGraph, commit, flow, readOnly]);
  const restore = useCallback(
    (g: WorkflowGraph) => {
      commit();
      applyGraph(g);
      appToast.success(t("workflows.versions.restored"));
      requestAnimationFrame(() => void flow.fitView({ padding: 0.2 }));
    },
    [applyGraph, commit, flow, t],
  );

  const label = useCallback(
    (id: string) => {
      const n = graph.nodes.find((x) => x.id === id);
      return n ? summary(n) : id;
    },
    [graph.nodes, summary],
  );

  const invalidate = () =>
    qc.invalidateQueries({ queryKey: workflowKeys.all(org?.slug) });
  const save = useMutation({
    mutationFn: () =>
      workflowsService.save(uuid, {
        name: name.trim(),
        graph,
        revision: meta.revision,
      }),
    onSuccess: (row) => {
      setMeta(row);
      setSaved(serialize(name, graph));
      setServerError(null);
      qc.setQueryData(workflowKeys.detail(org?.slug, uuid), row);
      void invalidate();
      appToast.success(t("workflows.toast.saved"));
    },
    onError: (error) => {
      if (isApiError(error) && error.status === 409) {
        appToast.error(t("workflows.toast.conflict"));
        return;
      }
      const message = isApiError(error)
        ? error.message
        : t("workflows.toast.failed");
      setServerError(message);
      setTab("issues");
      appToast.error(message);
    },
  });
  const trySave = useCallback(() => {
    if (readOnly || save.isPending) return;
    if (!name.trim()) {
      appToast.error(t("workflows.toast.name_required"));
      return;
    }
    if (validation.count > 0) {
      setTab("issues");
      appToast.error(t("workflows.toast.fix_issues"));
      return;
    }
    save.mutate();
  }, [name, readOnly, save, t, validation.count]);

  const publish = useMutation({
    mutationFn: () => workflowsService.publish(uuid, meta.revision),
    onSuccess: (row) => {
      setMeta(row);
      setPublishing(false);
      void invalidate();
      appToast.success(
        t("workflows.toast.published", { version: row.published_version ?? 0 }),
      );
    },
    onError: (error) => {
      setPublishing(false);
      appToast.error(
        isApiError(error) && error.status === 409
          ? t("workflows.toast.conflict")
          : isApiError(error)
            ? error.message
            : t("workflows.toast.failed"),
      );
    },
  });
  const enable = useMutation({
    mutationFn: (enabled: boolean) => workflowsService.enable(uuid, enabled),
    onSuccess: (row) => {
      setMeta(row);
      void invalidate();
      appToast.success(
        row.enabled
          ? t("workflows.toast.enabled")
          : t("workflows.toast.disabled"),
      );
    },
  });

  // Keyboard: undo/redo, save, zoom. Delete/Backspace is handled by React Flow.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      const key = e.key.toLowerCase();
      if (mod && key === "s") {
        e.preventDefault();
        trySave();
        return;
      }
      if (isTyping(e.target)) return;
      if (mod && key === "z") {
        e.preventDefault();
        if (e.shiftKey) redo();
        else undo();
      } else if (mod && key === "y") {
        e.preventDefault();
        redo();
      } else if (!mod && (key === "+" || key === "=")) {
        void flow.zoomIn({ duration: 150 });
      } else if (!mod && key === "-") {
        void flow.zoomOut({ duration: 150 });
      } else if (!mod && key === "0") {
        void flow.fitView({ padding: 0.2, duration: 200 });
      } else if (!mod && key === "l" && e.shiftKey) {
        relayout();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [flow, redo, relayout, trySave, undo]);

  useUnsavedChangesGuard(dirty);

  const onDrop = (e: DragEvent) => {
    const kind = e.dataTransfer.getData(DND_TYPE) as NodeKind;
    if (!NODE_KINDS.includes(kind)) return;
    e.preventDefault();
    addNode(kind, flow.screenToFlowPosition({ x: e.clientX, y: e.clientY }));
  };

  const displayEdges = useMemo(() => {
    const traversed = new Set<string>();
    for (const [id, mark] of Object.entries(trace))
      if (mark.port) traversed.add(`${id}:${mark.port}`);
    return edges.map((e) => {
      const port = e.sourceHandle ?? "next";
      const hot = traversed.has(e.id);
      const color = hot
        ? "#10b981"
        : port === "true"
          ? "#10b981"
          : port === "false"
            ? "#f43f5e"
            : undefined;
      return {
        ...e,
        animated: hot || e.data?.loop,
        label: e.data?.loop
          ? t("workflows.ports.loop")
          : port === "true" || port === "false"
            ? t(`workflows.ports.${port}`)
            : undefined,
        markerEnd: { type: MarkerType.ArrowClosed, color },
        style: {
          stroke: color,
          strokeWidth: hot ? 2.5 : 1.5,
          strokeDasharray: e.data?.loop ? "6 4" : undefined,
        },
      };
    });
  }, [edges, t, trace]);

  const latest = useMemo(
    () =>
      [...(versions.data ?? [])].sort((a, b) => b.version - a.version)[0] ??
      null,
    [versions.data],
  );
  const publishDiff = useMemo(
    () =>
      diffGraphs(
        latest?.graph ?? { schema_version: 1, nodes: [], edges: [] },
        graph,
      ),
    [graph, latest],
  );
  const draftAhead =
    meta.published_version === null || meta.published_version !== meta.revision;
  const canUndo = depth.past > 0;
  const canRedo = depth.future > 0;
  const hasTrigger = graph.nodes.some((n) => n.type === "trigger");
  const traceActive = Object.keys(trace).length > 0;

  return (
    <div className="bg-background -mx-4 -mb-6 flex h-[calc(100svh-4.5rem)] min-h-[520px] flex-col overflow-hidden border-t md:-mx-6">
      <header className="flex flex-wrap items-center gap-2 border-b px-3 py-2">
        <Button asChild size="icon" variant="ghost" className="size-8">
          <Link
            href={routes.cms.workflows.root}
            aria-label={t("workflows.editor.back")}
          >
            <ArrowLeft className="size-4" />
          </Link>
        </Button>
        <Input
          value={name}
          maxLength={200}
          disabled={readOnly}
          aria-label={t("workflows.fields_ui.name")}
          className="h-8 w-56 font-medium"
          onChange={(e) => setName(e.target.value)}
        />
        <div className="flex items-center gap-1.5">
          {meta.published_version !== null ? (
            <Badge variant="success">v{meta.published_version}</Badge>
          ) : (
            <Badge variant="secondary">{t("workflows.status.draft")}</Badge>
          )}
          {(dirty || (meta.published_version !== null && draftAhead)) && (
            <Badge variant="warning">
              {dirty
                ? t("workflows.status.unsaved")
                : t("workflows.status.unpublished")}
            </Badge>
          )}
          <span className="text-muted-foreground text-xs tabular-nums">
            {t("workflows.editor.revision", { revision: meta.revision })}
          </span>
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-1">
          {!readOnly && (
            <>
              <Button
                size="icon"
                variant="ghost"
                className="size-8"
                disabled={!canUndo}
                onClick={undo}
                aria-label={t("workflows.editor.undo")}
                title={t("workflows.editor.undo")}
              >
                <Undo2 className="size-4" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                className="size-8"
                disabled={!canRedo}
                onClick={redo}
                aria-label={t("workflows.editor.redo")}
                title={t("workflows.editor.redo")}
              >
                <Redo2 className="size-4" />
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={relayout}
                title={t("workflows.editor.layout")}
              >
                <LayoutGrid className="size-4" />
                <span className="hidden lg:inline">
                  {t("workflows.editor.layout")}
                </span>
              </Button>
            </>
          )}
          <ShortcutsHelp />
          {traceActive && (
            <Button size="sm" variant="ghost" onClick={() => setTrace({})}>
              <Eraser className="size-4" />
              <span className="hidden lg:inline">
                {t("workflows.editor.clear_trace")}
              </span>
            </Button>
          )}
          {!readOnly && (
            <Button
              size="sm"
              variant="outline"
              disabled={dirty}
              title={dirty ? t("workflows.test.save_first") : undefined}
              onClick={() => setTesting(true)}
            >
              <FlaskConical className="size-4" />
              {t("workflows.editor.test")}
            </Button>
          )}
          {!readOnly && (
            <Button
              size="sm"
              variant="outline"
              disabled={!dirty || save.isPending}
              onClick={trySave}
            >
              <Save className="size-4" />
              {save.isPending ? t("workflows.editor.saving") : t("common.save")}
            </Button>
          )}
          {!readOnly && (
            <Button
              size="sm"
              disabled={dirty || !draftAhead || validation.count > 0}
              title={dirty ? t("workflows.publish.save_first") : undefined}
              onClick={() => setPublishing(true)}
            >
              <Rocket className="size-4" />
              {t("workflows.editor.publish")}
            </Button>
          )}
          <label className="ml-1 flex items-center gap-2 text-xs">
            <Switch
              checked={meta.enabled}
              disabled={
                readOnly || meta.published_version === null || enable.isPending
              }
              onCheckedChange={(v) => enable.mutate(v)}
              aria-label={t("workflows.fields_ui.enabled")}
            />
            {meta.enabled
              ? t("workflows.status.enabled")
              : t("workflows.status.disabled")}
          </label>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        {!readOnly && (
          <aside
            className="hidden w-40 shrink-0 flex-col gap-1 overflow-y-auto border-r p-2 md:flex"
            aria-label={t("workflows.editor.palette")}
          >
            <p className="text-muted-foreground px-1 pb-1 text-[11px] font-semibold tracking-wide uppercase">
              {t("workflows.editor.palette")}
            </p>
            {NODE_KINDS.map((kind) => {
              const Icon = KIND_ICON[kind];
              const disabled = kind === "trigger" && hasTrigger;
              return (
                <button
                  key={kind}
                  type="button"
                  draggable={!disabled}
                  disabled={disabled}
                  onDragStart={(e) => {
                    e.dataTransfer.setData(DND_TYPE, kind);
                    e.dataTransfer.effectAllowed = "move";
                  }}
                  onClick={() => addNode(kind)}
                  title={t(`workflows.kind_help.${kind}`)}
                  className="hover:bg-muted flex items-center gap-2 rounded-md border px-2 py-1.5 text-left text-sm disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <Icon className="text-muted-foreground size-4 shrink-0" />
                  {t(`workflows.kinds.${kind}`)}
                </button>
              );
            })}
            <p className="text-muted-foreground mt-2 px-1 text-[11px] leading-snug">
              {t("workflows.editor.palette_hint")}
            </p>
          </aside>
        )}

        <div
          className="relative min-w-0 flex-1"
          onDragOver={(e) => {
            if (e.dataTransfer.types.includes(DND_TYPE)) {
              e.preventDefault();
              e.dataTransfer.dropEffect = "move";
            }
          }}
          onDrop={onDrop}
        >
          <EditorDecorations.Provider
            value={{ issues: validation.nodes, trace }}
          >
            <ReactFlow<FlowNode, FlowEdge>
              nodes={nodes}
              edges={displayEdges}
              nodeTypes={nodeTypes}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              isValidConnection={isValidConnection}
              onBeforeDelete={async () => {
                commit();
                return true;
              }}
              onNodeDragStart={() => {
                dragStart.current = graphRef.current;
              }}
              onNodeDragStop={() => {
                const before = dragStart.current;
                dragStart.current = null;
                if (
                  before &&
                  JSON.stringify(before) !== JSON.stringify(graphRef.current)
                )
                  pushPast(before);
              }}
              onNodeClick={() => setTab("node")}
              colorMode={resolvedTheme === "dark" ? "dark" : "light"}
              nodesDraggable={!readOnly}
              nodesConnectable={!readOnly}
              edgesReconnectable={false}
              deleteKeyCode={readOnly ? null : ["Delete", "Backspace"]}
              multiSelectionKeyCode={["Meta", "Control", "Shift"]}
              fitView
              fitViewOptions={{ padding: 0.2 }}
              minZoom={0.2}
              maxZoom={2}
              proOptions={{ hideAttribution: true }}
              className="!bg-muted/30"
            >
              <Background variant={BackgroundVariant.Dots} gap={18} size={1} />
              <MiniMap
                pannable
                zoomable
                nodeStrokeWidth={3}
                nodeColor={(n) =>
                  MINIMAP_COLOR[(n as FlowNode).data.node.type] ?? "#71717a"
                }
                ariaLabel={t("workflows.editor.minimap")}
              />
              <Controls showInteractive={false} />
            </ReactFlow>
          </EditorDecorations.Provider>
          {validation.count > 0 && (
            <button
              type="button"
              onClick={() => setTab("issues")}
              className="border-destructive/40 bg-background/95 text-destructive absolute top-3 left-3 flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs shadow-sm"
            >
              <AlertTriangle className="size-3.5" />
              {t("workflows.issues.count", { count: validation.count })}
            </button>
          )}
        </div>

        <aside className="flex w-80 shrink-0 flex-col border-l xl:w-96">
          <Tabs
            value={tab}
            onValueChange={(v) => setTab(v as PanelTab)}
            className="flex min-h-0 flex-1 flex-col"
          >
            <TabsList className="m-2 grid grid-cols-4">
              <TabsTrigger value="node">{t("workflows.tabs.node")}</TabsTrigger>
              <TabsTrigger value="issues">
                {t("workflows.tabs.issues")}
                {validation.count + (serverError ? 1 : 0) > 0 && (
                  <span className="bg-destructive text-destructive-foreground ml-1 rounded-full px-1.5 text-[10px]">
                    {validation.count + (serverError ? 1 : 0)}
                  </span>
                )}
              </TabsTrigger>
              <TabsTrigger value="versions">
                {t("workflows.tabs.versions")}
              </TabsTrigger>
              <TabsTrigger value="runs">{t("workflows.tabs.runs")}</TabsTrigger>
            </TabsList>
            <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
              <TabsContent value="node">
                {selected ? (
                  <NodePanel
                    key={selected.id}
                    node={selected.data.node}
                    issues={validation.nodes[selected.id] ?? []}
                    readOnly={readOnly}
                    onChange={updateNode}
                    onDelete={() =>
                      void flow.deleteElements({ nodes: [{ id: selected.id }] })
                    }
                  />
                ) : (
                  <p className="text-muted-foreground py-8 text-center text-sm">
                    {t("workflows.editor.select_node")}
                  </p>
                )}
              </TabsContent>
              <TabsContent value="issues">
                <IssuesList
                  validation={validation}
                  serverError={serverError}
                  label={label}
                  onSelect={(id) => {
                    setNodes((ns) =>
                      ns.map((n) => ({ ...n, selected: n.id === id })),
                    );
                    const n = nodes.find((x) => x.id === id);
                    if (n)
                      void flow.setCenter(
                        n.position.x + 112,
                        n.position.y + 30,
                        {
                          zoom: 1.1,
                          duration: 200,
                        },
                      );
                    setTab("node");
                  }}
                />
              </TabsContent>
              <TabsContent value="versions">
                <VersionsPanel
                  uuid={uuid}
                  draft={graph}
                  publishedVersion={meta.published_version}
                  readOnly={readOnly}
                  onRestore={restore}
                />
              </TabsContent>
              <TabsContent value="runs">
                <RunsPanel uuid={uuid} onHighlight={setTrace} />
              </TabsContent>
            </div>
          </Tabs>
        </aside>
      </div>

      {testing && (
        <TestDialog
          uuid={uuid}
          graph={graph}
          label={label}
          onClose={() => setTesting(false)}
          onTrace={setTrace}
        />
      )}
      <AppDialog
        open={publishing}
        onOpenChange={setPublishing}
        title={t("workflows.publish.title", { version: meta.revision })}
        description={t("workflows.publish.description")}
      >
        <div className="space-y-2">
          <p className="text-sm font-medium">
            {latest
              ? t("workflows.publish.vs", { version: latest.version })
              : t("workflows.publish.first")}
          </p>
          <DiffDetails diff={publishDiff} label={label} />
          {!meta.enabled && (
            <p className="text-muted-foreground text-xs">
              {t("workflows.publish.enables")}
            </p>
          )}
        </div>
        <FormActions sticky={false}>
          <Button
            type="button"
            variant="outline"
            onClick={() => setPublishing(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={publish.isPending}
            onClick={() => publish.mutate()}
          >
            <Rocket className="size-4" />
            {t("workflows.editor.publish")}
          </Button>
        </FormActions>
      </AppDialog>
    </div>
  );
}

function IssuesList({
  validation,
  serverError,
  label,
  onSelect,
}: {
  validation: ReturnType<typeof validateGraph>;
  serverError: string | null;
  label: (id: string) => string;
  onSelect: (id: string) => void;
}) {
  const { t } = useLocale();
  if (validation.count === 0 && !serverError)
    return (
      <p className="py-8 text-center text-sm text-emerald-700 dark:text-emerald-300">
        {t("workflows.issues.none")}
      </p>
    );
  return (
    <div className="space-y-3">
      {serverError && (
        <div className="border-destructive/40 bg-destructive/5 rounded-md border p-2.5 text-xs">
          <p className="text-destructive font-medium">
            {t("workflows.issues.server")}
          </p>
          <p className="text-destructive mt-0.5">{serverError}</p>
        </div>
      )}
      {validation.graph.length > 0 && (
        <ul className="space-y-1">
          {validation.graph.map((i, idx) => (
            <li key={idx} className="text-destructive flex gap-1.5 text-xs">
              <AlertTriangle className="mt-0.5 size-3 shrink-0" />
              {t(i.key, i.params)}
            </li>
          ))}
        </ul>
      )}
      <ul className="space-y-1.5">
        {Object.entries(validation.nodes).map(([id, issues]) => (
          <li key={id}>
            <button
              type="button"
              onClick={() => onSelect(id)}
              className="hover:bg-muted w-full rounded-md border px-2.5 py-2 text-left"
            >
              <span className="block truncate text-sm font-medium">
                {label(id)}
              </span>
              {issues.map((i, idx) => (
                <span
                  key={idx}
                  className={cn("text-destructive block text-xs")}
                >
                  {t(i.key, i.params)}
                </span>
              ))}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

function ShortcutsHelp() {
  const { t } = useLocale();
  const rows: [string, string][] = [
    ["Delete / Backspace", t("workflows.shortcuts.delete")],
    ["⌘/Ctrl + Z", t("workflows.shortcuts.undo")],
    ["⌘/Ctrl + Shift + Z, Ctrl + Y", t("workflows.shortcuts.redo")],
    ["⌘/Ctrl + S", t("workflows.shortcuts.save")],
    ["+ / −", t("workflows.shortcuts.zoom")],
    ["0", t("workflows.shortcuts.fit")],
    ["Shift + L", t("workflows.shortcuts.layout")],
  ];
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          size="icon"
          variant="ghost"
          className="size-8"
          aria-label={t("workflows.shortcuts.title")}
          title={t("workflows.shortcuts.title")}
        >
          <Keyboard className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <p className="mb-2 text-sm font-semibold">
          {t("workflows.shortcuts.title")}
        </p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs">
          {rows.map(([k, v]) => (
            <div key={k} className="contents">
              <dt>
                <kbd className="bg-muted rounded border px-1.5 py-0.5 font-mono text-[10px]">
                  {k}
                </kbd>
              </dt>
              <dd className="text-muted-foreground">{v}</dd>
            </div>
          ))}
        </dl>
      </PopoverContent>
    </Popover>
  );
}
