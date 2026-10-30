"use client";

import { Filter, Plus, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useCustomFieldMeta } from "../hooks/use-custom-fields";
import {
  countActiveFilters,
  fallbackOperators,
  filterValueKind,
  type CustomFieldFilterRow,
} from "../lib/filters";
import type {
  CustomFieldDefinition,
  CustomFieldOperator,
  CustomFieldType,
} from "../types";
import { CustomFieldInput } from "./custom-field-input";

type CustomFieldFilterBuilderProps = {
  /** Definitions that can be filtered on (archived ones are skipped). */
  definitions: readonly CustomFieldDefinition[];
  rows: CustomFieldFilterRow[];
  onChange: (rows: CustomFieldFilterRow[]) => void;
  /** Toolbar trigger (default) or the bare row editor. */
  variant?: "popover" | "inline";
  className?: string;
};

let rowSeq = 0;
function nextRowId() {
  rowSeq += 1;
  return `cf-filter-${rowSeq}`;
}

/** Operators of a type from the meta endpoint, else the built-in table. */
function useOperators() {
  const meta = useCustomFieldMeta();
  return useMemo(() => {
    const byType = new Map<CustomFieldType, CustomFieldOperator[]>();
    for (const ft of meta.data?.field_types ?? []) {
      byType.set(ft.type, ft.operators);
    }
    return (type: CustomFieldType) =>
      byType.get(type) ?? fallbackOperators(type);
  }, [meta.data]);
}

/**
 * Filter rows over custom fields (field → operator → value). Turn the rows
 * into `cf.*` query params with `buildFilterParams` and pass them to a list
 * endpoint; fits in a DataTable `toolbarExtra`.
 */
export function CustomFieldFilterBuilder({
  definitions,
  rows,
  onChange,
  variant = "popover",
  className,
}: CustomFieldFilterBuilderProps) {
  const { t } = useLocale();
  const live = useMemo(
    () => definitions.filter((d) => !d.archived),
    [definitions],
  );
  const activeCount = countActiveFilters(rows, live);

  if (!live.length) return null;

  const editor = (
    <FilterRowsEditor definitions={live} rows={rows} onChange={onChange} />
  );

  if (variant === "inline") {
    return <div className={className}>{editor}</div>;
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={cn("h-8 gap-1.5 shadow-none", className)}
        >
          <Filter className="size-3.5" />
          {t("custom_fields.filters.button")}
          {activeCount ? (
            <Badge variant="secondary" className="h-5 rounded-full px-1.5">
              {activeCount}
            </Badge>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-[min(36rem,calc(100vw-2rem))] p-3"
      >
        {editor}
      </PopoverContent>
    </Popover>
  );
}

function FilterRowsEditor({
  definitions,
  rows,
  onChange,
}: {
  definitions: readonly CustomFieldDefinition[];
  rows: CustomFieldFilterRow[];
  onChange: (rows: CustomFieldFilterRow[]) => void;
}) {
  const { t } = useLocale();
  const operatorsFor = useOperators();
  const byKey = useMemo(
    () => new Map(definitions.map((d) => [d.key, d])),
    [definitions],
  );

  const addRow = () => {
    const def = definitions[0];
    onChange([
      ...rows,
      {
        id: nextRowId(),
        field: def.key,
        operator: operatorsFor(def.type)[0] ?? "eq",
        value: undefined,
      },
    ]);
  };

  const updateRow = (id: string, patch: Partial<CustomFieldFilterRow>) => {
    onChange(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  };

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm font-medium">{t("custom_fields.filters.title")}</p>
      {rows.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("custom_fields.filters.empty")}
        </p>
      ) : null}
      {rows.map((row) => {
        const def = byKey.get(row.field);
        const operators = def ? operatorsFor(def.type) : [];
        return (
          <div
            key={row.id}
            className="grid grid-cols-[1fr_auto] gap-2 sm:grid-cols-[10rem_8rem_1fr_auto]"
          >
            <Select
              value={row.field}
              onValueChange={(key) => {
                const next = byKey.get(key);
                if (!next) return;
                const ops = operatorsFor(next.type);
                updateRow(row.id, {
                  field: key,
                  operator: ops.includes(row.operator) ? row.operator : ops[0],
                  value: undefined,
                });
              }}
            >
              <SelectTrigger
                size="sm"
                className="col-span-2 sm:col-span-1"
                aria-label={t("custom_fields.filters.field")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {definitions.map((d) => (
                  <SelectItem key={d.key} value={d.key}>
                    {d.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={row.operator}
              onValueChange={(op) => {
                const operator = op as CustomFieldOperator;
                const keepValue =
                  def &&
                  filterValueKind(def.type, operator) ===
                    filterValueKind(def.type, row.operator);
                updateRow(row.id, {
                  operator,
                  value: keepValue ? row.value : undefined,
                });
              }}
            >
              <SelectTrigger
                size="sm"
                className="col-span-2 sm:col-span-1"
                aria-label={t("custom_fields.filters.operator")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {operators.map((op) => (
                  <SelectItem key={op} value={op}>
                    {t(`custom_fields.operators.${op}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="min-w-0">
              {def ? (
                <FilterValueInput
                  definition={def}
                  operator={row.operator}
                  value={row.value}
                  onChange={(value) => updateRow(row.id, { value })}
                />
              ) : null}
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={t("custom_fields.filters.remove")}
              onClick={() => onChange(rows.filter((r) => r.id !== row.id))}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        );
      })}
      <div className="flex items-center justify-between gap-2 pt-1">
        <Button type="button" variant="outline" size="sm" onClick={addRow}>
          <Plus className="size-4" />
          {t("custom_fields.filters.add")}
        </Button>
        {rows.length ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onChange([])}
          >
            {t("custom_fields.filters.clear")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

function FilterValueInput({
  definition,
  operator,
  value,
  onChange,
}: {
  definition: CustomFieldDefinition;
  operator: CustomFieldOperator;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const { t } = useLocale();
  const kind = filterValueKind(definition.type, operator);

  switch (kind) {
    case "boolean": {
      const current =
        value === true || value === "true"
          ? "true"
          : value === false || value === "false"
            ? "false"
            : "";
      const isSet = operator === "set";
      return (
        <Select value={current} onValueChange={(v) => onChange(v === "true")}>
          <SelectTrigger
            size="sm"
            aria-label={t("custom_fields.filters.value")}
          >
            <SelectValue placeholder={t("custom_fields.filters.value")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="true">
              {isSet
                ? t("custom_fields.filters.is_set")
                : t("custom_fields.value.yes")}
            </SelectItem>
            <SelectItem value="false">
              {isSet
                ? t("custom_fields.filters.is_not_set")
                : t("custom_fields.value.no")}
            </SelectItem>
          </SelectContent>
        </Select>
      );
    }
    case "options":
      return (
        <Select
          value={typeof value === "string" ? value : ""}
          onValueChange={onChange}
        >
          <SelectTrigger
            size="sm"
            aria-label={t("custom_fields.filters.value")}
          >
            <SelectValue placeholder={t("custom_fields.value.pick_option")} />
          </SelectTrigger>
          <SelectContent>
            {definition.options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      );
    case "multi-options":
      return (
        <CustomFieldInput
          definition={{
            ...definition,
            type: "multiselect",
            // Archived options still match stored values.
            options: definition.options.map((o) => ({ ...o, archived: false })),
            config: {},
          }}
          value={Array.isArray(value) ? value : []}
          onChange={onChange}
          className="min-h-8 py-1"
        />
      );
    case "date":
      return (
        <DatePicker
          value={typeof value === "string" ? value : ""}
          onChange={onChange}
          className="h-8"
        />
      );
    default:
      return (
        <Input
          value={typeof value === "string" ? value : ""}
          inputMode={kind === "number" ? "decimal" : undefined}
          placeholder={
            kind === "list"
              ? t("custom_fields.filters.list_placeholder")
              : t("custom_fields.filters.value")
          }
          onChange={(e) => onChange(e.target.value)}
          className="h-8"
        />
      );
  }
}
