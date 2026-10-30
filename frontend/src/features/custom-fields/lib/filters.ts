import type {
  CustomFieldDefinition,
  CustomFieldOperator,
  CustomFieldType,
} from "../types";

/** Query prefix of custom field filters and sorts (`cf.<key>`). */
export const CF_PREFIX = "cf.";

/** One row of the filter builder. `value` shape depends on the operator. */
export type CustomFieldFilterRow = {
  id: string;
  field: string;
  operator: CustomFieldOperator;
  value: unknown;
};

/** Operators per type when the meta endpoint has not answered yet. */
export function fallbackOperators(
  type: CustomFieldType,
): CustomFieldOperator[] {
  switch (type) {
    case "number":
    case "money":
    case "date":
      return ["eq", "neq", "in", "gt", "gte", "lt", "lte", "set"];
    case "text":
    case "textarea":
    case "url":
    case "email":
    case "phone":
      return ["eq", "neq", "in", "contains", "set"];
    case "multiselect":
      return ["eq", "in", "set"];
    case "boolean":
      return ["eq", "set"];
    default:
      return ["eq", "neq", "in", "set"];
  }
}

/** How the value input of a filter row behaves for (type, operator). */
export type FilterValueKind =
  | "boolean" // yes / no
  | "options" // one option
  | "multi-options" // several options
  | "date"
  | "number"
  | "list" // comma separated text
  | "text";

export function filterValueKind(
  type: CustomFieldType,
  operator: CustomFieldOperator,
): FilterValueKind {
  if (operator === "set" || type === "boolean") return "boolean";
  if (operator === "contains") return "text";
  if (type === "select" || type === "multiselect") {
    return operator === "in" || type === "multiselect"
      ? "multi-options"
      : "options";
  }
  if (operator === "in") return "list";
  if (type === "date") return "date";
  if (type === "number" || type === "money") return "number";
  return "text";
}

function serializeValue(kind: FilterValueKind, value: unknown): string | null {
  switch (kind) {
    case "boolean":
      if (value === true || value === "true") return "true";
      if (value === false || value === "false") return "false";
      return null;
    case "multi-options": {
      const list = Array.isArray(value) ? value : [];
      const parts = list.map((v) => String(v).trim()).filter(Boolean);
      return parts.length ? parts.join(",") : null;
    }
    case "list": {
      const raw = Array.isArray(value) ? value.join(",") : String(value ?? "");
      const parts = raw
        .split(",")
        .map((v) => v.trim())
        .filter(Boolean);
      return parts.length ? parts.join(",") : null;
    }
    case "number": {
      const s = String(value ?? "")
        .trim()
        .replace(",", ".");
      return s !== "" && Number.isFinite(Number(s)) ? s : null;
    }
    default: {
      const s = String(value ?? "").trim();
      return s === "" ? null : s;
    }
  }
}

/** Parameter name of a row: `cf.<key>` for eq, else `cf.<key>.<op>`. */
export function filterParamName(
  key: string,
  operator: CustomFieldOperator,
): string {
  return operator === "eq"
    ? `${CF_PREFIX}${key}`
    : `${CF_PREFIX}${key}.${operator}`;
}

/**
 * Turns filter rows into `cf.*` query parameters. Rows with an unknown or
 * archived field, or without a usable value, are skipped. When two rows
 * share a parameter the last one wins (the API takes one value per param,
 * lists go through `in`).
 */
export function buildFilterParams(
  rows: readonly CustomFieldFilterRow[],
  defs: readonly Pick<CustomFieldDefinition, "key" | "type">[],
): Record<string, string> {
  const byKey = new Map(defs.map((d) => [d.key, d]));
  const out: Record<string, string> = {};
  for (const row of rows) {
    const def = byKey.get(row.field);
    if (!def) continue;
    const kind = filterValueKind(def.type, row.operator);
    const value = serializeValue(kind, row.value);
    if (value === null) continue;
    out[filterParamName(def.key, row.operator)] = value;
  }
  return out;
}

/** Number of rows that produce a parameter. */
export function countActiveFilters(
  rows: readonly CustomFieldFilterRow[],
  defs: readonly Pick<CustomFieldDefinition, "key" | "type">[],
): number {
  return Object.keys(buildFilterParams(rows, defs)).length;
}

/** Sort parameter for a custom field column (`cf.<key>` / `-cf.<key>`). */
export function customFieldSortParam(key: string, desc: boolean): string {
  return `${desc ? "-" : ""}${CF_PREFIX}${key}`;
}
