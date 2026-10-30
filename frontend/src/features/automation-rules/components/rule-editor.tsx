"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  FlaskConical,
  Plus,
  Save,
  Trash2,
  Zap,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useId, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { ConditionEditor } from "@/features/workflows/components/condition-editor";
import { FieldSelect } from "@/features/workflows/components/field-select";
import { ActionForm } from "@/features/workflows/components/node-panel";
import type { Issue, WorkflowAction } from "@/features/workflows/lib/graph";
import { useUnsavedChangesGuard } from "@/hooks/use-unsaved-changes-guard";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { ruleKeys, useRule, useRuleTriggers } from "../hooks/use-rules";
import {
  RULE_ACTIONS,
  RULE_LIMITS,
  emptyRule,
  hasIssues,
  toInput,
  triggerLabel,
  validateRule,
} from "../lib/rules";
import {
  rulesService,
  type RuleAction,
  type RuleInput,
} from "../services/rules.service";
import { RuleTestDialog } from "./rule-test-dialog";

/**
 * Form editor of one automation rule (CF-94): trigger, an AND/OR condition
 * tree and an ordered action list. Condition and action fields are the same
 * components the visual workflow editor uses for its node panel.
 */
export function RuleEditorPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantAutomationsRead);
  const canWrite = perms.includes(Permission.TenantAutomationsWrite);
  const isNew = uuid === "new";
  const rule = useRule(uuid);

  if (org && !canRead)
    return (
      <p className="text-muted-foreground text-sm">{t("rules.forbidden")}</p>
    );
  if (isNew && !canWrite)
    return (
      <p className="text-muted-foreground text-sm">{t("rules.forbidden")}</p>
    );
  if (!isNew && rule.isLoading) return <Loading label={t("common.loading")} />;
  if (!isNew && (rule.isError || !rule.data))
    return (
      <ErrorState
        title={t("rules.error.title")}
        description={t("rules.error.description")}
        onRetry={() => void rule.refetch()}
      />
    );
  return (
    <RuleForm
      key={rule.data?.uuid ?? "new"}
      uuid={isNew ? null : uuid}
      initial={rule.data ? toInput(rule.data) : emptyRule()}
      readOnly={!canWrite}
    />
  );
}

function RuleForm({
  uuid,
  initial,
  readOnly,
}: {
  uuid: string | null;
  initial: RuleInput;
  readOnly: boolean;
}) {
  const { t } = useLocale();
  const id = useId();
  const router = useRouter();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const slug = org?.slug ?? "";
  const [draft, setDraft] = useState<RuleInput>(initial);
  const [saved, setSaved] = useState(initial);
  const [testing, setTesting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const triggers = useRuleTriggers();
  const issues = validateRule(draft);
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved);

  useUnsavedChangesGuard(dirty && !readOnly);

  const patch = (p: Partial<RuleInput>) => setDraft((d) => ({ ...d, ...p }));
  const setActions = (actions: RuleAction[]) => patch({ actions });

  const save = useMutation({
    mutationFn: () => {
      const body: RuleInput = {
        ...draft,
        name: draft.name.trim(),
        schedule_minutes:
          draft.trigger === "scheduled" ? draft.schedule_minutes : 0,
      };
      return uuid
        ? rulesService.update(slug, uuid, body)
        : rulesService.create(slug, body);
    },
    onSuccess: async (row) => {
      await qc.invalidateQueries({ queryKey: ruleKeys.all(slug) });
      appToast.success(t("rules.toast.saved"));
      if (!uuid) {
        router.replace(routes.cms.rules.detail(row.uuid));
        return;
      }
      const next = toInput(row);
      setDraft(next);
      setSaved(next);
    },
  });
  const remove = useMutation({
    mutationFn: () => rulesService.remove(slug, uuid ?? ""),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ruleKeys.all(slug) });
      appToast.success(t("rules.toast.deleted"));
      router.push(routes.cms.rules.root);
    },
  });

  const triggerOptions = Array.from(
    new Set([...(triggers.data ?? []), draft.trigger]),
  ).map((tr) => ({ value: tr, label: triggerLabel(t, tr) }));
  const move = (from: number, to: number) => {
    const next = [...draft.actions];
    const [item] = next.splice(from, 1);
    if (item) next.splice(to, 0, item);
    setActions(next);
  };

  return (
    <div className="space-y-6 pb-8">
      <PageHeader
        icon={<Zap className="size-7" />}
        title={uuid ? saved.name || t("rules.title") : t("rules.create")}
        description={t("rules.editor_description")}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="ghost" asChild>
              <Link href={routes.cms.rules.root}>
                <ArrowLeft className="size-4" />
                {t("rules.back")}
              </Link>
            </Button>
            {uuid && !readOnly && (
              <Button
                size="sm"
                variant="outline"
                disabled={dirty}
                title={dirty ? t("rules.test.save_first") : undefined}
                onClick={() => setTesting(true)}
              >
                <FlaskConical className="size-4" />
                {t("rules.test.action")}
              </Button>
            )}
            {uuid && !readOnly && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setDeleting(true)}
              >
                <Trash2 className="size-4" />
                {t("common.delete")}
              </Button>
            )}
            {!readOnly && (
              <Button
                size="sm"
                disabled={!dirty || hasIssues(issues) || save.isPending}
                onClick={() => save.mutate()}
              >
                <Save className="size-4" />
                {save.isPending ? t("rules.saving") : t("rules.save")}
              </Button>
            )}
          </div>
        }
      />

      {save.isError && (
        <p
          className="border-destructive/40 bg-destructive/5 text-destructive rounded-md border p-3 text-sm"
          role="alert"
        >
          {isApiError(save.error)
            ? save.error.message
            : t("rules.toast.failed")}
        </p>
      )}
      <IssueList issues={issues.form} />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                {t("rules.sections.general")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor={`${id}-name`}>{t("rules.fields.name")}</Label>
                <Input
                  id={`${id}-name`}
                  value={draft.name}
                  maxLength={RULE_LIMITS.maxName}
                  disabled={readOnly}
                  autoFocus={!uuid}
                  onChange={(e) => patch({ name: e.target.value })}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor={`${id}-position`}>
                  {t("rules.fields.position")}
                </Label>
                <Input
                  id={`${id}-position`}
                  type="number"
                  inputMode="numeric"
                  className="w-28"
                  value={draft.position ?? 0}
                  disabled={readOnly}
                  onChange={(e) =>
                    patch({ position: Math.trunc(Number(e.target.value)) || 0 })
                  }
                />
                <p className="text-muted-foreground text-xs">
                  {t("rules.fields.position_hint")}
                </p>
              </div>
              <label className="flex items-center justify-between gap-3 text-sm">
                <span>
                  {t("rules.fields.enabled")}
                  <span className="text-muted-foreground block text-xs">
                    {t("rules.fields.enabled_hint")}
                  </span>
                </span>
                <Switch
                  checked={draft.enabled}
                  disabled={readOnly}
                  onCheckedChange={(enabled) => patch({ enabled })}
                />
              </label>
              <label className="flex items-center justify-between gap-3 text-sm">
                <span>
                  {t("rules.fields.stop_on_match")}
                  <span className="text-muted-foreground block text-xs">
                    {t("rules.fields.stop_on_match_hint")}
                  </span>
                </span>
                <Switch
                  checked={Boolean(draft.stop_on_match)}
                  disabled={readOnly}
                  onCheckedChange={(stop_on_match) => patch({ stop_on_match })}
                />
              </label>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                {t("rules.sections.trigger")}
              </CardTitle>
              <CardDescription>{t("rules.trigger_hint")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <FieldSelect
                id={`${id}-trigger`}
                ariaLabel={t("rules.fields.trigger")}
                value={draft.trigger}
                disabled={readOnly || triggers.isLoading}
                onChange={(trigger) =>
                  patch({
                    trigger,
                    schedule_minutes:
                      trigger === "scheduled"
                        ? draft.schedule_minutes || 60
                        : 0,
                  })
                }
                options={triggerOptions}
              />
              {draft.trigger === "scheduled" && (
                <div className="space-y-1.5">
                  <Label htmlFor={`${id}-minutes`}>
                    {t("rules.fields.schedule_minutes")}
                  </Label>
                  <Input
                    id={`${id}-minutes`}
                    type="number"
                    min={1}
                    max={RULE_LIMITS.maxScheduleMinutes}
                    disabled={readOnly}
                    value={draft.schedule_minutes ?? ""}
                    onChange={(e) =>
                      patch({
                        schedule_minutes: Math.max(
                          0,
                          Math.trunc(Number(e.target.value)),
                        ),
                      })
                    }
                  />
                  <p className="text-muted-foreground text-xs">
                    {t("rules.fields.schedule_hint")}
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <div className="min-w-0 space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                {t("rules.sections.conditions")}
              </CardTitle>
              <CardDescription>{t("rules.conditions_hint")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {issues.conditions && <IssueList issues={[issues.conditions]} />}
              <ConditionEditor
                value={draft.conditions}
                disabled={readOnly}
                onChange={(conditions) => patch({ conditions })}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                {t("rules.sections.actions")}
              </CardTitle>
              <CardDescription>{t("rules.actions_hint")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <ol className="space-y-3">
                {draft.actions.map((a, i) => {
                  const issue = issues.actions[i];
                  return (
                    <li
                      key={i}
                      className="bg-background space-y-2 rounded-md border p-3"
                    >
                      <div className="flex items-center gap-1">
                        <span className="text-muted-foreground text-xs font-medium tabular-nums">
                          {t("rules.action_step", { step: i + 1 })}
                        </span>
                        {!readOnly && (
                          <div className="ml-auto flex gap-0.5">
                            <Button
                              type="button"
                              size="icon"
                              variant="ghost"
                              className="size-8"
                              aria-label={t("rules.move_up")}
                              disabled={i === 0}
                              onClick={() => move(i, i - 1)}
                            >
                              <ArrowUp className="size-3.5" />
                            </Button>
                            <Button
                              type="button"
                              size="icon"
                              variant="ghost"
                              className="size-8"
                              aria-label={t("rules.move_down")}
                              disabled={i === draft.actions.length - 1}
                              onClick={() => move(i, i + 1)}
                            >
                              <ArrowDown className="size-3.5" />
                            </Button>
                            <Button
                              type="button"
                              size="icon"
                              variant="ghost"
                              className="size-8"
                              aria-label={t("rules.remove_action")}
                              onClick={() =>
                                setActions(
                                  draft.actions.filter((_, j) => j !== i),
                                )
                              }
                            >
                              <Trash2 className="size-3.5" />
                            </Button>
                          </div>
                        )}
                      </div>
                      {issue && <IssueList issues={[issue]} />}
                      <ActionForm
                        action={a as WorkflowAction}
                        actions={RULE_ACTIONS}
                        readOnly={readOnly}
                        onChange={(next) =>
                          setActions(
                            draft.actions.map((x, j) =>
                              j === i ? (next as RuleAction) : x,
                            ),
                          )
                        }
                      />
                    </li>
                  );
                })}
              </ol>
              {!readOnly && draft.actions.length < RULE_LIMITS.maxActions && (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    setActions([
                      ...draft.actions,
                      { type: "add_note", content: "" },
                    ])
                  }
                >
                  <Plus className="size-3.5" />
                  {t("rules.add_action")}
                </Button>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {testing && uuid && (
        <RuleTestDialog uuid={uuid} onClose={() => setTesting(false)} />
      )}
      <DeleteDialog
        open={deleting}
        title={t("rules.delete.title")}
        description={t("rules.delete.description", { name: saved.name })}
        isPending={remove.isPending}
        onConfirm={() => remove.mutate()}
        onCancel={() => setDeleting(false)}
      />
    </div>
  );
}

function IssueList({ issues }: { issues: Issue[] }) {
  const { t } = useLocale();
  if (issues.length === 0) return null;
  return (
    <ul
      className="border-destructive/40 bg-destructive/5 text-destructive space-y-1 rounded-md border p-2 text-xs"
      role="alert"
    >
      {issues.map((i, idx) => (
        <li key={idx} className="flex gap-1.5">
          <AlertTriangle className="mt-0.5 size-3 shrink-0" />
          {t(i.key, i.params)}
        </li>
      ))}
    </ul>
  );
}
