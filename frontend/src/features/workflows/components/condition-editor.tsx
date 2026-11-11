"use client";

import { Plus, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  useCatalogListFields,
  type CustomFieldDefinition,
} from "@/features/custom-fields";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  BOOLEAN_FIELDS,
  CONDITION_FIELDS,
  LIST_OPERATORS,
  NUMERIC_FIELDS,
  NUMERIC_OPERATORS,
  OPERATORS,
  PRIORITIES,
  STATUSES,
  UNARY_OPERATORS,
  slug,
  type Condition,
  type ConditionOperator,
} from "../lib/graph";
import { FieldSelect } from "./field-select";

const CUSTOM = "__custom__";

/** Condition path of a catalog entity's custom field. */
function catalogPath(entity: string, key: string) {
  return entity === "conversation"
    ? `custom_fields.${key}`
    : `${entity}.custom_fields.${key}`;
}

/**
 * Workflow-visible list fields of the catalog (CF-196) keyed by condition
 * path, so a leaf picks option ids instead of typing free text.
 */
function useCatalogConditionFields() {
  const catalog = useCatalogListFields();
  return useMemo(() => {
    const out = new Map<
      string,
      { entity: string; definition: CustomFieldDefinition }
    >();
    for (const entity of ["contact", "company", "conversation"] as const) {
      for (const d of catalog[entity]) {
        if (d.visibility?.workflow === false) continue;
        out.set(catalogPath(entity, d.key), { entity, definition: d });
      }
    }
    return out;
  }, [catalog]);
}
const MAX_UI_DEPTH = 3;

/**
 * Bounded AND/OR tree editor: groups (`all` / `any`) holding field leaves,
 * the same shape the automation rules use. A root leaf is shown as a
 * one-item `all` group so users can grow it without restructuring.
 */
export function ConditionEditor({
  value,
  onChange,
  disabled,
}: {
  value: Condition | undefined;
  onChange: (next: Condition) => void;
  disabled?: boolean;
}) {
  const root: Condition =
    value && (value.all || value.any) ? value : { all: value ? [value] : [] };
  return (
    <GroupEditor
      value={root}
      onChange={onChange}
      disabled={disabled}
      depth={0}
    />
  );
}

function GroupEditor({
  value,
  onChange,
  onRemove,
  disabled,
  depth,
}: {
  value: Condition;
  onChange: (next: Condition) => void;
  onRemove?: () => void;
  disabled?: boolean;
  depth: number;
}) {
  const { t } = useLocale();
  const mode: "all" | "any" = value.any ? "any" : "all";
  const items = value[mode] ?? [];
  const set = (next: Condition[]) => onChange({ [mode]: next });
  return (
    <div
      className={cn(
        "space-y-2 rounded-md border p-2",
        depth > 0 && "bg-muted/40",
      )}
    >
      <div className="flex items-center gap-2">
        <FieldSelect
          value={mode}
          disabled={disabled}
          ariaLabel={t("workflows.condition.mode")}
          className="h-8 w-auto"
          onChange={(m) => onChange({ [m]: items })}
          options={[
            { value: "all", label: t("workflows.condition.all") },
            { value: "any", label: t("workflows.condition.any") },
          ]}
        />
        {onRemove && !disabled && (
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="ml-auto size-8"
            aria-label={t("workflows.condition.remove_group")}
            onClick={onRemove}
          >
            <Trash2 className="size-3.5" />
          </Button>
        )}
      </div>
      {items.length === 0 && (
        <p className="text-muted-foreground text-xs">
          {mode === "all"
            ? t("workflows.condition.empty_all")
            : t("workflows.condition.empty_any")}
        </p>
      )}
      {items.map((c, i) => {
        const update = (next: Condition) =>
          set(items.map((x, j) => (j === i ? next : x)));
        const remove = () => set(items.filter((_, j) => j !== i));
        return c.all || c.any ? (
          <GroupEditor
            key={i}
            value={c}
            onChange={update}
            onRemove={remove}
            disabled={disabled}
            depth={depth + 1}
          />
        ) : (
          <LeafEditor
            key={i}
            value={c}
            onChange={update}
            onRemove={remove}
            disabled={disabled}
          />
        );
      })}
      {!disabled && (
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() =>
              set([
                ...items,
                { field: "status", operator: "eq", value: "open" },
              ])
            }
          >
            <Plus className="size-3.5" />
            {t("workflows.condition.add_rule")}
          </Button>
          {depth < MAX_UI_DEPTH && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => set([...items, { all: [] }])}
            >
              <Plus className="size-3.5" />
              {t("workflows.condition.add_group")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

function toInputValue(v: unknown): string {
  if (Array.isArray(v)) return v.join(", ");
  if (v === undefined || v === null) return "";
  return String(v);
}

/** Converts the text input into the JSON type the backend comparison expects. */
function parseValue(
  field: string,
  op: ConditionOperator,
  raw: string,
): unknown {
  const numeric = NUMERIC_FIELDS.has(field) || NUMERIC_OPERATORS.has(op);
  const one = (s: string): unknown => {
    const v = s.trim();
    if (numeric && v !== "" && !Number.isNaN(Number(v))) return Number(v);
    return v;
  };
  if (LIST_OPERATORS.has(op))
    return raw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .map(one);
  if (BOOLEAN_FIELDS.has(field)) return raw === "true";
  return one(raw);
}

function LeafEditor({
  value,
  onChange,
  onRemove,
  disabled,
}: {
  value: Condition;
  onChange: (next: Condition) => void;
  onRemove: () => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const catalog = useCatalogConditionFields();
  const field = value.field ?? "";
  const op = value.operator ?? "eq";
  const listField = catalog.get(field);
  const isCustom =
    !(CONDITION_FIELDS as readonly string[]).includes(field) && !listField;
  const enumOptions =
    field === "status"
      ? STATUSES.concat().map((s) => ({
          value: s,
          label: t(`workflows.statuses.${s}`),
        }))
      : field === "priority"
        ? PRIORITIES.concat().map((p) => ({
            value: p,
            label: t(`workflows.priorities.${p}`),
          }))
        : null;
  const setField = (f: string) => {
    const next: Condition = { field: f, operator: op };
    if (!UNARY_OPERATORS.has(op))
      next.value = parseValue(f, op, toInputValue(value.value));
    onChange(next);
  };
  const setOp = (o: string) => {
    const operator = o as ConditionOperator;
    const next: Condition = { field, operator };
    if (!UNARY_OPERATORS.has(operator))
      next.value = parseValue(field, operator, toInputValue(value.value));
    onChange(next);
  };
  const setValue = (raw: string) =>
    onChange({ field, operator: op, value: parseValue(field, op, raw) });

  return (
    <div className="bg-background space-y-1.5 rounded-md border p-2">
      <div className="flex items-center gap-1.5">
        <FieldSelect
          value={isCustom ? CUSTOM : field}
          disabled={disabled}
          ariaLabel={t("workflows.condition.field")}
          className="h-8 min-w-0 flex-1"
          onChange={(f) => setField(f === CUSTOM ? "custom_fields." : f)}
          options={[
            ...CONDITION_FIELDS.map((f) => ({
              value: f,
              label: t(`workflows.fields.${slug(f)}`),
            })),
            ...[...catalog.entries()].map(([path, c]) => ({
              value: path,
              label: `${t(`workflows.fields.catalog_${c.entity}`)} · ${c.definition.label}`,
            })),
            { value: CUSTOM, label: t("workflows.fields.custom") },
          ]}
        />
        {!disabled && (
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="size-8 shrink-0"
            aria-label={t("workflows.condition.remove_rule")}
            onClick={onRemove}
          >
            <Trash2 className="size-3.5" />
          </Button>
        )}
      </div>
      {isCustom && (
        <Input
          value={field}
          disabled={disabled}
          maxLength={180}
          className="h-8 font-mono text-xs"
          aria-label={t("workflows.fields.custom_key")}
          placeholder="contact.custom_fields.paket"
          onChange={(e) => onChange({ ...value, field: e.target.value.trim() })}
        />
      )}
      <div className="grid grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] gap-1.5">
        <FieldSelect
          value={op}
          disabled={disabled}
          ariaLabel={t("workflows.condition.operator")}
          className="h-8"
          onChange={setOp}
          options={OPERATORS.map((o) => ({
            value: o,
            label: t(`workflows.operators.${o}`),
          }))}
        />
        {UNARY_OPERATORS.has(op) ? (
          <span />
        ) : BOOLEAN_FIELDS.has(field) && !LIST_OPERATORS.has(op) ? (
          <FieldSelect
            value={String(value.value === true)}
            disabled={disabled}
            ariaLabel={t("workflows.condition.value")}
            className="h-8"
            onChange={setValue}
            options={[
              { value: "true", label: t("workflows.condition.yes") },
              { value: "false", label: t("workflows.condition.no") },
            ]}
          />
        ) : listField && (op === "eq" || op === "neq") ? (
          <FieldSelect
            value={toInputValue(value.value)}
            disabled={disabled}
            ariaLabel={t("workflows.condition.value")}
            className="h-8"
            onChange={(id) => onChange({ field, operator: op, value: id })}
            options={listField.definition.options.map((o) => ({
              value: o.value,
              label: o.label,
            }))}
          />
        ) : listField && LIST_OPERATORS.has(op) ? (
          <ToggleGroup
            type="multiple"
            variant="outline"
            size="sm"
            spacing={1}
            className="flex-wrap"
            disabled={disabled}
            value={Array.isArray(value.value) ? value.value.map(String) : []}
            onValueChange={(ids) =>
              onChange({ field, operator: op, value: ids })
            }
          >
            {listField.definition.options.map((o) => (
              <ToggleGroupItem key={o.value} value={o.value}>
                {o.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        ) : enumOptions && (op === "eq" || op === "neq") ? (
          <FieldSelect
            value={toInputValue(value.value)}
            disabled={disabled}
            ariaLabel={t("workflows.condition.value")}
            className="h-8"
            onChange={setValue}
            options={enumOptions}
          />
        ) : (
          <Input
            value={toInputValue(value.value)}
            disabled={disabled}
            className="h-8 text-xs"
            inputMode={
              NUMERIC_FIELDS.has(field) || NUMERIC_OPERATORS.has(op)
                ? "decimal"
                : undefined
            }
            aria-label={t("workflows.condition.value")}
            placeholder={
              LIST_OPERATORS.has(op)
                ? t("workflows.condition.list_hint")
                : undefined
            }
            onChange={(e) => setValue(e.target.value)}
          />
        )}
      </div>
    </div>
  );
}
