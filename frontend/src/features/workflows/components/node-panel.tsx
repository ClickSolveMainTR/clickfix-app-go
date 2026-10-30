"use client";

import { AlertTriangle, Trash2 } from "lucide-react";
import { useId } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { useLocale } from "@/providers/locale-provider";

import { useTargetOptions } from "../hooks/use-workflows";
import {
  ACTIONS,
  ACTION_TARGET,
  LIMITS,
  PRIORITIES,
  ROUTE_STRATEGIES,
  STATUSES,
  TRIGGERS,
  WAIT_UNITS,
  isUuid,
  parseRouteSnapshot,
  slug,
  splitDelay,
  type ActionType,
  type Issue,
  type RouteSnapshot,
  type TargetKind,
  type WaitUnit,
  type WorkflowAction,
  type WorkflowNode,
} from "../lib/graph";
import { ConditionEditor } from "./condition-editor";
import { FieldSelect } from "./field-select";
import { KIND_ICON } from "./workflow-node";

/**
 * Side panel form for the selected node. Each kind edits exactly the fields
 * the backend graph schema defines; issues from the client validator are
 * listed on top so the user sees why the node is red.
 */
export function NodePanel({
  node,
  issues,
  readOnly,
  onChange,
  onDelete,
}: {
  node: WorkflowNode;
  issues: Issue[];
  readOnly: boolean;
  onChange: (next: WorkflowNode) => void;
  onDelete: () => void;
}) {
  const { t } = useLocale();
  const Icon = KIND_ICON[node.type];
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Icon className="text-muted-foreground size-4" />
        <h3 className="text-sm font-semibold">
          {t(`workflows.kinds.${node.type}`)}
        </h3>
        <code className="text-muted-foreground truncate text-[10px]">
          {node.id}
        </code>
        {!readOnly && node.type !== "trigger" && (
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="ml-auto size-8"
            aria-label={t("workflows.editor.delete_node")}
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
          </Button>
        )}
      </div>
      <p className="text-muted-foreground text-xs">
        {t(`workflows.kind_help.${node.type}`)}
      </p>
      {issues.length > 0 && (
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
      )}
      {node.type === "trigger" && (
        <TriggerForm node={node} readOnly={readOnly} onChange={onChange} />
      )}
      {(node.type === "condition" || node.type === "branch") && (
        <ConditionEditor
          value={node.condition}
          disabled={readOnly}
          onChange={(condition) => onChange({ ...node, condition })}
        />
      )}
      {node.type === "action" && (
        <ActionForm
          action={node.action ?? { type: "add_note" }}
          readOnly={readOnly}
          onChange={(action) => onChange({ ...node, action })}
        />
      )}
      {node.type === "wait" && (
        <WaitForm node={node} readOnly={readOnly} onChange={onChange} />
      )}
    </div>
  );
}

function TriggerForm({
  node,
  readOnly,
  onChange,
}: {
  node: WorkflowNode;
  readOnly: boolean;
  onChange: (next: WorkflowNode) => void;
}) {
  const { t } = useLocale();
  const id = useId();
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-trigger`}>
          {t("workflows.fields_ui.trigger")}
        </Label>
        <FieldSelect
          id={`${id}-trigger`}
          value={node.trigger ?? ""}
          disabled={readOnly}
          onChange={(v) => {
            const trigger = v as NonNullable<WorkflowNode["trigger"]>;
            const next: WorkflowNode = { ...node, trigger };
            if (trigger === "scheduled")
              next.schedule_minutes = node.schedule_minutes || 60;
            else delete next.schedule_minutes;
            onChange(next);
          }}
          options={TRIGGERS.map((tr) => ({
            value: tr,
            label: t(`workflows.triggers.${slug(tr)}`),
          }))}
        />
        <p className="text-muted-foreground text-xs">
          {t(
            `workflows.trigger_help.${slug(node.trigger ?? "conversation.created")}`,
          )}
        </p>
      </div>
      {node.trigger === "scheduled" && (
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-minutes`}>
            {t("workflows.fields_ui.schedule_minutes")}
          </Label>
          <Input
            id={`${id}-minutes`}
            type="number"
            min={1}
            max={LIMITS.maxScheduleMinutes}
            disabled={readOnly}
            value={node.schedule_minutes ?? ""}
            onChange={(e) =>
              onChange({
                ...node,
                schedule_minutes: Math.max(
                  0,
                  Math.trunc(Number(e.target.value)),
                ),
              })
            }
          />
        </div>
      )}
    </div>
  );
}

function WaitForm({
  node,
  readOnly,
  onChange,
}: {
  node: WorkflowNode;
  readOnly: boolean;
  onChange: (next: WorkflowNode) => void;
}) {
  const { t } = useLocale();
  const id = useId();
  const { amount, unit } = splitDelay(node.delay_seconds ?? 0);
  const set = (a: number, u: WaitUnit) =>
    onChange({
      ...node,
      delay_seconds: Math.max(0, Math.round(a * WAIT_UNITS[u])),
    });
  return (
    <div className="space-y-1.5">
      <Label htmlFor={`${id}-amount`}>{t("workflows.fields_ui.delay")}</Label>
      <div className="grid grid-cols-2 gap-2">
        <Input
          id={`${id}-amount`}
          type="number"
          min={1}
          disabled={readOnly}
          value={amount}
          onChange={(e) => set(Number(e.target.value), unit)}
        />
        <FieldSelect
          value={unit}
          disabled={readOnly}
          ariaLabel={t("workflows.fields_ui.delay_unit")}
          onChange={(u) => set(amount, u as WaitUnit)}
          options={(Object.keys(WAIT_UNITS) as WaitUnit[]).map((u) => ({
            value: u,
            label: t(`workflows.units.${u}`),
          }))}
        />
      </div>
      <p className="text-muted-foreground text-xs">
        {t("workflows.wait_help")}
      </p>
    </div>
  );
}

/**
 * Action fields of a workflow node. Also used by the rule editor (CF-94),
 * which passes the narrower action list automation rules accept.
 */
export function ActionForm({
  action,
  readOnly,
  onChange,
  actions = ACTIONS,
}: {
  action: WorkflowAction;
  readOnly: boolean;
  onChange: (next: WorkflowAction) => void;
  actions?: readonly ActionType[];
}) {
  const { t } = useLocale();
  const id = useId();
  const target = ACTION_TARGET[action.type];
  const setType = (type: ActionType) => {
    const next: WorkflowAction = { type };
    if (type === "set_status") next.value = "resolved";
    if (type === "set_priority") next.value = "high";
    if (type === "route")
      next.content = JSON.stringify(parseRouteSnapshot(undefined));
    onChange(next);
  };
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-type`}>{t("workflows.fields_ui.action")}</Label>
        <FieldSelect
          id={`${id}-type`}
          value={action.type}
          disabled={readOnly}
          onChange={(v) => setType(v as ActionType)}
          options={actions.map((a) => ({
            value: a,
            label: t(`workflows.actions.${a}`),
          }))}
        />
        <p className="text-muted-foreground text-xs">
          {t(`workflows.action_help.${action.type}`)}
        </p>
      </div>
      {target && (
        <TargetField
          kind={target}
          value={action.target_uuid ?? ""}
          readOnly={readOnly}
          onChange={(target_uuid) => onChange({ ...action, target_uuid })}
        />
      )}
      {action.type === "set_status" && (
        <FieldSelect
          value={action.value ?? ""}
          disabled={readOnly}
          ariaLabel={t("workflows.fields_ui.status")}
          onChange={(value) => onChange({ ...action, value })}
          options={STATUSES.map((s) => ({
            value: s,
            label: t(`workflows.statuses.${s}`),
          }))}
        />
      )}
      {action.type === "set_priority" && (
        <FieldSelect
          value={action.value ?? ""}
          disabled={readOnly}
          ariaLabel={t("workflows.fields_ui.priority")}
          onChange={(value) => onChange({ ...action, value })}
          options={PRIORITIES.map((p) => ({
            value: p,
            label: t(`workflows.priorities.${p}`),
          }))}
        />
      )}
      {action.type === "create_engineering_issue" && (
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-title`}>
            {t("workflows.fields_ui.title")}
          </Label>
          <Input
            id={`${id}-title`}
            value={action.value ?? ""}
            maxLength={LIMITS.maxTitle}
            disabled={readOnly}
            onChange={(e) => onChange({ ...action, value: e.target.value })}
          />
        </div>
      )}
      {(action.type === "add_note" ||
        action.type === "send_canned_reply" ||
        action.type === "create_engineering_issue") && (
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-content`}>
            {action.type === "create_engineering_issue"
              ? t("workflows.fields_ui.description")
              : t("workflows.fields_ui.content")}
          </Label>
          <Textarea
            id={`${id}-content`}
            rows={5}
            value={action.content ?? ""}
            maxLength={LIMITS.maxContent}
            disabled={readOnly}
            onChange={(e) => onChange({ ...action, content: e.target.value })}
          />
        </div>
      )}
      {action.type === "route" && (
        <RouteForm
          value={parseRouteSnapshot(action.content)}
          readOnly={readOnly}
          onChange={(snap) =>
            onChange({ ...action, content: JSON.stringify(snap) })
          }
        />
      )}
    </div>
  );
}

function TargetField({
  kind,
  value,
  readOnly,
  onChange,
}: {
  kind: TargetKind | "team";
  value: string;
  readOnly: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useLocale();
  const id = useId();
  const options = useTargetOptions(kind);
  const list = options.data ?? [];
  const known = !value || list.some((o) => o.value === value);
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{t(`workflows.targets.${kind}`)}</Label>
      {options.isError ? (
        <>
          <Input
            id={id}
            value={value}
            disabled={readOnly}
            className="font-mono text-xs"
            placeholder="00000000-0000-0000-0000-000000000000"
            aria-invalid={Boolean(value) && !isUuid(value)}
            onChange={(e) => onChange(e.target.value.trim())}
          />
          <p className="text-muted-foreground text-xs">
            {t("workflows.targets.manual")}
          </p>
        </>
      ) : (
        <>
          <AsyncCombobox
            id={id}
            value={value}
            disabled={readOnly || options.isLoading}
            onValueChange={onChange}
            options={list.map((o) => ({
              value: o.value,
              label: o.label,
              description: o.hint,
            }))}
            placeholder={t("workflows.targets.placeholder")}
            searchPlaceholder={t("workflows.targets.search")}
            emptyText={t("workflows.targets.empty")}
          />
          {!known && (
            <p className="text-muted-foreground text-xs">
              {t("workflows.targets.unknown", { uuid: value })}
            </p>
          )}
        </>
      )}
    </div>
  );
}

function RouteForm({
  value,
  readOnly,
  onChange,
}: {
  value: RouteSnapshot;
  readOnly: boolean;
  onChange: (next: RouteSnapshot) => void;
}) {
  const { t } = useLocale();
  const id = useId();
  const settings = value.settings;
  const setSettings = (patch: Partial<RouteSnapshot["settings"]>) =>
    onChange({ ...value, settings: { ...settings, ...patch } });
  return (
    <div className="space-y-3">
      <TargetField
        kind="team"
        value={value.team_uuid ?? ""}
        readOnly={readOnly}
        onChange={(team) => onChange({ ...value, team_uuid: team || null })}
      />
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-strategy`}>
          {t("workflows.route.strategy")}
        </Label>
        <FieldSelect
          id={`${id}-strategy`}
          value={settings.strategy}
          disabled={readOnly}
          onChange={(s) =>
            setSettings({
              strategy: s as RouteSnapshot["settings"]["strategy"],
            })
          }
          options={ROUTE_STRATEGIES.map((s) => ({
            value: s,
            label: t(`workflows.route.strategies.${s}`),
          }))}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-skills`}>{t("workflows.route.skills")}</Label>
        <Input
          id={`${id}-skills`}
          value={value.skills.join(", ")}
          disabled={readOnly}
          placeholder={t("workflows.condition.list_hint")}
          onChange={(e) =>
            onChange({
              ...value,
              skills: e.target.value
                .split(",")
                .map((s) => s.trim())
                .filter(Boolean)
                .slice(0, LIMITS.maxSkills),
            })
          }
        />
      </div>
      {(
        [
          "respect_capacity",
          "vip_account_owner",
          "reassign_on_offline",
        ] as const
      ).map((key) => (
        <label
          key={key}
          className="flex items-center justify-between gap-3 text-sm"
        >
          {t(`workflows.route.${key}`)}
          <Switch
            checked={settings[key]}
            disabled={readOnly}
            onCheckedChange={(checked) => setSettings({ [key]: checked })}
          />
        </label>
      ))}
      <p className="text-muted-foreground text-xs">
        {t("workflows.route.help")}
      </p>
    </div>
  );
}
