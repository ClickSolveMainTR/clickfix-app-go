"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Pencil,
  Plus,
  RotateCcw,
  Save,
  Trash2,
  Upload,
  Undo2,
} from "lucide-react";
import { useMemo, useState } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { diffLines } from "../lib/diff";
import { aiErrorMessage, isConflict } from "../lib/errors";
import {
  aiSettingsService,
  BUILTIN_TOOLS,
  type AIProcedureInput,
  type AIProcedureRecord,
} from "../services/ai-settings.service";
import { SettingRow } from "./parts";
import type { AISettingsTabProps } from "./tab-registry";
import { toolsKey } from "./tools-tab";

const MAX_STEPS = 20;
const MAX_STEP_CHARS = 1000;
const MAX_TOTAL_CHARS = 3000;
const MAX_TOOLS = 20;

export const proceduresKey = (slug: string) =>
  ["tenant", slug, "ai", "procedures"] as const;

type Step = { id: number; text: string };
type FormState = {
  intent: string;
  locale: AIProcedureInput["locale"];
  title: string;
  steps: Step[];
  tools: string[];
};

let nextStepId = 1;
const toSteps = (steps: string[]): Step[] =>
  steps.map((text) => ({ id: nextStepId++, text }));

function toForm(input?: AIProcedureInput): FormState {
  return {
    intent: input?.intent ?? "",
    locale: input?.locale ?? "tr",
    title: input?.title ?? "",
    steps: toSteps(input?.steps?.length ? input.steps : [""]),
    tools: input?.allowed_tools ?? [],
  };
}

function toInput(form: FormState, revision?: number): AIProcedureInput {
  return {
    intent: form.intent.trim(),
    locale: form.locale,
    title: form.title.trim(),
    steps: form.steps.map((s) => s.text.trim()),
    allowed_tools: form.tools,
    ...(revision ? { revision } : {}),
  };
}

function sameInput(a: AIProcedureInput, b: AIProcedureInput) {
  return (
    a.title === b.title &&
    JSON.stringify(a.steps) === JSON.stringify(b.steps) &&
    JSON.stringify([...a.allowed_tools].sort()) ===
      JSON.stringify([...b.allowed_tools].sort())
  );
}

function ProcedureStatus({ record }: { record: AIProcedureRecord }) {
  const { t } = useLocale();
  if (!record.published) {
    return (
      <Badge variant="secondary">
        {t("ai_settings.procedures.unpublished")}
      </Badge>
    );
  }
  if (record.published_revision === record.revision) {
    return (
      <Badge variant="success">
        {t("ai_settings.procedures.published", {
          revision: record.published_revision ?? 0,
        })}
      </Badge>
    );
  }
  return (
    <Badge variant="warning">
      {t("ai_settings.procedures.draft_ahead", {
        published: record.published_revision ?? 0,
        revision: record.revision,
      })}
    </Badge>
  );
}

/** "Prosedürler" tab (CF-68): intent-specific step lists with draft/publish. */
export function ProceduresTab({ slug, canWrite }: AISettingsTabProps) {
  const { t } = useLocale();
  const listState = useServerListState({ initialPageSize: 50 });
  const { limit, offset } = listState.params;
  const query = useQuery({
    queryKey: [...proceduresKey(slug), limit, offset],
    queryFn: () => aiSettingsService.procedures(slug, limit, offset),
    placeholderData: keepPreviousData,
  });
  const [editing, setEditing] = useState<AIProcedureRecord | "new" | null>(
    null,
  );

  const columns = useMemo(
    () =>
      [
        createColumn<AIProcedureRecord>({
          accessorKey: "title",
          labelKey: "ai_settings.procedures.title_label",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.title}</span>
          ),
        }),
        createColumn<AIProcedureRecord>({
          id: "status",
          accessorFn: (row) => row.published,
          labelKey: "ai_settings.runs.status_label",
          cell: ({ row }) => <ProcedureStatus record={row.original} />,
        }),
        createColumn<AIProcedureRecord>({
          accessorKey: "intent",
          labelKey: "ai_settings.procedures.intent",
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.intent}
            </code>
          ),
        }),
        createColumn<AIProcedureRecord>({
          accessorKey: "locale",
          labelKey: "ai_settings.procedures.locale",
          cell: ({ row }) => t(`ai_settings.locale.${row.original.locale}`),
        }),
        createColumn<AIProcedureRecord>({
          id: "steps",
          accessorFn: (row) => row.draft.steps.length,
          labelKey: "ai_settings.procedures.columns.steps",
          cell: ({ row }) =>
            t("ai_settings.procedures.step_count", {
              count: row.original.draft.steps.length,
            }),
        }),
        createColumn<AIProcedureRecord>({
          id: "actions",
          labelKey: "common.actions",
          enableHiding: false,
          meta: { headerClassName: "text-right" },
          cell: ({ row }) => (
            <div className="flex justify-end">
              <Button
                size="sm"
                variant="outline"
                onClick={() => setEditing(row.original)}
              >
                <Pencil className="size-4" />
                {canWrite
                  ? t("ai_settings.procedures.edit")
                  : t("ai_settings.procedures.view")}
              </Button>
            </div>
          ),
        }),
      ] as ColumnDef<AIProcedureRecord, unknown>[],
    [canWrite, t],
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="text-muted-foreground max-w-2xl text-sm">
          {t("ai_settings.procedures.description")}
        </p>
        {canWrite && (
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("ai_settings.procedures.new")}
          </Button>
        )}
      </div>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={setEditing}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("ai_settings.procedures.empty_title")}
        emptyDescription={t("ai_settings.procedures.empty")}
        total={query.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: "tenant-ai-procedures-v1",
          // The endpoint takes `limit` and `offset` only.
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
        }}
      />
      <Sheet
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      >
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
          {editing !== null && (
            <ProcedureEditor
              key={editing === "new" ? "new" : editing.uuid}
              slug={slug}
              canWrite={canWrite}
              record={editing === "new" ? null : editing}
              onRecord={(record) => setEditing(record)}
            />
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}

function ProcedureEditor({
  slug,
  canWrite,
  record: initial,
  onRecord,
}: {
  slug: string;
  canWrite: boolean;
  record: AIProcedureRecord | null;
  onRecord: (record: AIProcedureRecord) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [record, setRecord] = useState(initial);
  const [form, setForm] = useState<FormState>(() => toForm(initial?.draft));
  const [conflict, setConflict] = useState(false);
  const tools = useQuery({
    queryKey: toolsKey(slug),
    queryFn: () => aiSettingsService.tools(slug),
  });
  const readOnly = !canWrite;
  const isNew = record === null;

  const input = toInput(form, record?.revision);
  const dirty = isNew || !sameInput(input, record.draft);
  const totalChars = form.steps.reduce((sum, s) => sum + s.text.length, 0);
  const errors = {
    intent: !input.intent || input.intent.length > 100,
    title: !input.title || input.title.length > 200,
    steps:
      form.steps.length === 0 ||
      form.steps.some(
        (s) => !s.text.trim() || s.text.length > MAX_STEP_CHARS,
      ) ||
      totalChars > MAX_TOTAL_CHARS,
  };
  const invalid = errors.intent || errors.title || errors.steps;

  const accept = (next: AIProcedureRecord) => {
    setRecord(next);
    setForm(toForm(next.draft));
    setConflict(false);
    onRecord(next);
    void qc.invalidateQueries({ queryKey: proceduresKey(slug) });
  };
  const onError = (error: unknown) => {
    if (isConflict(error)) setConflict(true);
    else appToast.error(aiErrorMessage(error, t));
  };
  const save = useMutation({
    mutationFn: () =>
      isNew
        ? aiSettingsService.createProcedure(slug, input)
        : aiSettingsService.updateProcedure(slug, record.uuid, input),
    onSuccess: (next) => {
      accept(next);
      appToast.success(t("ai_settings.procedures.saved"));
    },
    onError: (error) => {
      if (isNew && isConflict(error))
        appToast.error(t("ai_settings.procedures.intent_taken"));
      else onError(error);
    },
  });
  const publish = useMutation({
    mutationFn: (value: boolean) =>
      aiSettingsService.publishProcedure(slug, record!.uuid, {
        revision: record!.revision,
        publish: value,
      }),
    onSuccess: (next, value) => {
      accept(next);
      appToast.success(
        t(
          value
            ? "ai_settings.procedures.published_toast"
            : "ai_settings.procedures.unpublished_toast",
        ),
      );
    },
    onError,
  });
  const reload = async () => {
    const page = await qc.fetchQuery({
      queryKey: proceduresKey(slug),
      queryFn: () => aiSettingsService.procedures(slug),
    });
    const fresh = page.items.find((r) => r.uuid === record?.uuid);
    if (fresh) accept(fresh);
  };

  const setStep = (index: number, text: string) =>
    setForm((f) => ({
      ...f,
      steps: f.steps.map((s, i) => (i === index ? { ...s, text } : s)),
    }));
  const moveStep = (index: number, delta: number) =>
    setForm((f) => {
      const steps = [...f.steps];
      const target = index + delta;
      if (target < 0 || target >= steps.length) return f;
      [steps[index], steps[target]] = [steps[target], steps[index]];
      return { ...f, steps };
    });
  const toggleTool = (name: string, checked: boolean) =>
    setForm((f) => ({
      ...f,
      tools: checked
        ? [...f.tools, name].slice(0, MAX_TOOLS)
        : f.tools.filter((x) => x !== name),
    }));

  const orgTools = (tools.data?.items ?? []).map((tool) => ({
    name: tool.name,
    requiresApproval: tool.requires_approval,
    enabled: tool.enabled,
    builtin: false,
  }));
  const toolOptions = [
    ...BUILTIN_TOOLS.map((tool) => ({ ...tool, enabled: true, builtin: true })),
    ...orgTools,
  ];
  // Keep tools that were removed from the catalog visible so they can be unticked.
  for (const name of form.tools) {
    if (!toolOptions.some((o) => o.name === name)) {
      toolOptions.push({
        name,
        requiresApproval: false,
        enabled: false,
        builtin: false,
      });
    }
  }

  const published = record?.published;
  const showDiff =
    published && record && record.published_revision !== record.revision;

  return (
    <div className="space-y-6">
      <SheetHeader className="pe-8">
        <SheetTitle>
          {isNew
            ? t("ai_settings.procedures.new")
            : record.title || t("ai_settings.procedures.edit")}
        </SheetTitle>
        <SheetDescription>
          {isNew ? (
            t("ai_settings.procedures.editor_hint")
          ) : (
            <span className="flex flex-wrap items-center gap-2">
              <ProcedureStatus record={record} />
              <span>
                {t("ai_settings.procedures.revision", {
                  revision: record.revision,
                })}
              </span>
            </span>
          )}
        </SheetDescription>
      </SheetHeader>

      {conflict && (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("ai_settings.conflict.title")}</AlertTitle>
          <AlertDescription className="space-y-3">
            <p>{t("ai_settings.procedures.conflict_description")}</p>
            <Button size="sm" variant="outline" onClick={() => void reload()}>
              <RotateCcw className="size-4" />
              {t("ai_settings.conflict.reload")}
            </Button>
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_10rem]">
        <SettingRow
          label={t("ai_settings.procedures.intent")}
          hint={
            isNew
              ? t("ai_settings.procedures.intent_hint")
              : t("ai_settings.procedures.intent_locked")
          }
          htmlFor="proc-intent"
        >
          <Input
            id="proc-intent"
            value={form.intent}
            maxLength={100}
            disabled={!isNew || readOnly}
            placeholder="refund_request"
            aria-invalid={errors.intent && form.intent !== ""}
            onChange={(e) => setForm((f) => ({ ...f, intent: e.target.value }))}
          />
        </SettingRow>
        <SettingRow
          label={t("ai_settings.procedures.locale")}
          htmlFor="proc-locale"
        >
          <Select
            value={form.locale}
            disabled={!isNew || readOnly}
            onValueChange={(value) =>
              setForm((f) => ({
                ...f,
                locale: value as AIProcedureInput["locale"],
              }))
            }
          >
            <SelectTrigger id="proc-locale" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="tr">{t("ai_settings.locale.tr")}</SelectItem>
              <SelectItem value="en">{t("ai_settings.locale.en")}</SelectItem>
            </SelectContent>
          </Select>
        </SettingRow>
      </div>

      <SettingRow
        label={t("ai_settings.procedures.title_label")}
        htmlFor="proc-title"
      >
        <Input
          id="proc-title"
          value={form.title}
          maxLength={200}
          disabled={readOnly}
          placeholder={t("ai_settings.procedures.title_placeholder")}
          onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
        />
      </SettingRow>

      <fieldset className="space-y-3">
        <legend className="text-sm font-medium">
          {t("ai_settings.procedures.steps")}
        </legend>
        <p className="text-muted-foreground text-xs">
          {t("ai_settings.procedures.steps_hint")}
        </p>
        <ol className="space-y-2">
          {form.steps.map((step, index) => (
            <li key={step.id} className="flex gap-2">
              <span
                aria-hidden
                className="bg-muted mt-2 flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-medium tabular-nums"
              >
                {index + 1}
              </span>
              <Textarea
                rows={2}
                value={step.text}
                maxLength={MAX_STEP_CHARS}
                disabled={readOnly}
                aria-label={t("ai_settings.procedures.step_label", {
                  n: index + 1,
                })}
                className="min-h-0 flex-1"
                onChange={(e) => setStep(index, e.target.value)}
              />
              {!readOnly && (
                <div className="flex shrink-0 flex-col gap-1">
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="size-7"
                    disabled={index === 0}
                    aria-label={t("ai_settings.procedures.move_up", {
                      n: index + 1,
                    })}
                    onClick={() => moveStep(index, -1)}
                  >
                    <ArrowUp className="size-3.5" />
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="size-7"
                    disabled={index === form.steps.length - 1}
                    aria-label={t("ai_settings.procedures.move_down", {
                      n: index + 1,
                    })}
                    onClick={() => moveStep(index, 1)}
                  >
                    <ArrowDown className="size-3.5" />
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="text-destructive size-7"
                    disabled={form.steps.length === 1}
                    aria-label={t("ai_settings.procedures.remove_step", {
                      n: index + 1,
                    })}
                    onClick={() =>
                      setForm((f) => ({
                        ...f,
                        steps: f.steps.filter((_, i) => i !== index),
                      }))
                    }
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ol>
        <div className="flex flex-wrap items-center justify-between gap-2">
          {!readOnly && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={form.steps.length >= MAX_STEPS}
              onClick={() =>
                setForm((f) => ({
                  ...f,
                  steps: [...f.steps, ...toSteps([""])],
                }))
              }
            >
              <Plus className="size-4" />
              {t("ai_settings.procedures.add_step")}
            </Button>
          )}
          <span
            className={cn(
              "text-muted-foreground ms-auto text-xs tabular-nums",
              totalChars > MAX_TOTAL_CHARS && "text-destructive",
            )}
          >
            {t("ai_settings.procedures.step_budget", {
              steps: form.steps.length,
              max_steps: MAX_STEPS,
              chars: totalChars,
              max_chars: MAX_TOTAL_CHARS,
            })}
          </span>
        </div>
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="text-sm font-medium">
          {t("ai_settings.procedures.tools")}
        </legend>
        <p className="text-muted-foreground text-xs">
          {t("ai_settings.procedures.tools_hint")}
        </p>
        <ul className="grid gap-2 sm:grid-cols-2">
          {toolOptions.map((tool) => {
            const id = `proc-tool-${tool.name}`;
            const checked = form.tools.includes(tool.name);
            return (
              <li key={tool.name}>
                <Label
                  htmlFor={id}
                  className="hover:bg-muted/50 flex cursor-pointer items-start gap-2 rounded-md border p-2 font-normal"
                >
                  <Checkbox
                    id={id}
                    checked={checked}
                    disabled={
                      readOnly || (!checked && form.tools.length >= MAX_TOOLS)
                    }
                    onCheckedChange={(value) =>
                      toggleTool(tool.name, value === true)
                    }
                    className="mt-0.5"
                  />
                  <span className="min-w-0 space-y-1">
                    <code className="block truncate text-xs">{tool.name}</code>
                    <span className="flex flex-wrap gap-1">
                      <Badge variant="outline" className="text-[10px]">
                        {tool.builtin
                          ? t("ai_settings.tools.builtin")
                          : t("ai_settings.tools.http")}
                      </Badge>
                      {tool.requiresApproval && (
                        <Badge variant="warning" className="text-[10px]">
                          {t("ai_settings.tools.requires_approval")}
                        </Badge>
                      )}
                      {!tool.enabled && (
                        <Badge variant="secondary" className="text-[10px]">
                          {t("ai_settings.tools.disabled")}
                        </Badge>
                      )}
                    </span>
                  </span>
                </Label>
              </li>
            );
          })}
        </ul>
      </fieldset>

      {showDiff && published && (
        <section className="space-y-2">
          <h3 className="text-sm font-medium">
            {t("ai_settings.procedures.diff_title", {
              published: record.published_revision ?? 0,
              revision: record.revision,
            })}
          </h3>
          <div className="rounded-md border text-xs">
            {published.title !== record.draft.title && (
              <div className="border-b p-2">
                <p className="text-muted-foreground mb-1">
                  {t("ai_settings.procedures.title_label")}
                </p>
                <p className="text-destructive line-through">
                  {published.title}
                </p>
                <p className="text-signal-text">{record.draft.title}</p>
              </div>
            )}
            <ol className="divide-y">
              {diffLines(published.steps, record.draft.steps).map(
                (line, index) => (
                  <li
                    key={index}
                    className={cn(
                      "flex gap-2 px-2 py-1.5 whitespace-pre-wrap",
                      line.kind === "added" && "bg-signal-soft",
                      line.kind === "removed" &&
                        "bg-destructive/10 text-destructive line-through",
                    )}
                  >
                    <span aria-hidden className="w-3 shrink-0 font-mono">
                      {line.kind === "added"
                        ? "+"
                        : line.kind === "removed"
                          ? "-"
                          : " "}
                    </span>
                    <span className="sr-only">
                      {t(`ai_settings.procedures.diff_${line.kind}`)}
                    </span>
                    <span className="min-w-0">{line.text}</span>
                  </li>
                ),
              )}
            </ol>
            {JSON.stringify([...published.allowed_tools].sort()) !==
              JSON.stringify([...record.draft.allowed_tools].sort()) && (
              <p className="border-t p-2">
                {t("ai_settings.procedures.diff_tools", {
                  before: published.allowed_tools.join(", ") || "-",
                  after: record.draft.allowed_tools.join(", ") || "-",
                })}
              </p>
            )}
          </div>
        </section>
      )}

      {canWrite && (
        <div className="bg-background/95 sticky bottom-0 -mx-6 flex flex-wrap items-center justify-end gap-2 border-t px-6 py-3 backdrop-blur">
          {!isNew && dirty && (
            <span className="text-muted-foreground me-auto text-xs">
              {t("ai_settings.procedures.save_before_publish")}
            </span>
          )}
          {!isNew && record.published && (
            <Button
              variant="outline"
              disabled={publish.isPending}
              onClick={() => publish.mutate(false)}
            >
              <Undo2 className="size-4" />
              {t("ai_settings.procedures.unpublish")}
            </Button>
          )}
          {!isNew && record.published_revision !== record.revision && (
            <Button
              variant="outline"
              disabled={dirty || publish.isPending}
              onClick={() => publish.mutate(true)}
            >
              <Upload className="size-4" />
              {t("ai_settings.procedures.publish", {
                revision: record.revision,
              })}
            </Button>
          )}
          <Button
            disabled={!dirty || invalid || save.isPending}
            onClick={() => save.mutate()}
          >
            <Save className="size-4" />
            {isNew
              ? t("ai_settings.procedures.create_draft")
              : t("ai_settings.procedures.save_draft")}
          </Button>
        </div>
      )}
    </div>
  );
}
