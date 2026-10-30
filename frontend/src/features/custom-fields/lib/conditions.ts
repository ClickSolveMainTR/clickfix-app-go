import type {
  CustomFieldCondition,
  CustomFieldDefinition,
  CustomFieldValues,
} from "../types";

/**
 * Client mirror of the backend condition rules
 * (`backend/internal/platform/customfields/values.go`): a conditional field
 * is shown, required and stored only while its condition holds. Empty
 * values count as "not set" because the API drops them.
 */

type ConditionDef = Pick<
  CustomFieldDefinition,
  "key" | "type" | "condition" | "archived"
>;

/** True when a value would be dropped by the API (nothing stored). */
export function isEmptyValue(value: unknown): boolean {
  if (value === null || value === undefined) return true;
  if (typeof value === "string") return value.trim() === "";
  if (Array.isArray(value)) return value.length === 0;
  if (typeof value === "number") return Number.isNaN(value);
  if (typeof value === "object") {
    const amount = (value as { amount_minor?: unknown }).amount_minor;
    if ("amount_minor" in (value as object)) {
      return amount === null || amount === undefined || amount === "";
    }
  }
  return false;
}

function normalizeScalar(type: string, value: unknown): unknown {
  if (value === null || value === undefined) return value;
  switch (type) {
    case "number": {
      const n =
        typeof value === "number"
          ? value
          : Number(String(value).replace(",", ".").trim());
      return Number.isFinite(n) ? n : undefined;
    }
    case "boolean":
      if (typeof value === "boolean") return value;
      if (typeof value === "string") {
        const s = value.trim().toLowerCase();
        if (s === "true" || s === "1") return true;
        if (s === "false" || s === "0") return false;
      }
      return undefined;
    case "money": {
      if (typeof value === "object" && value) {
        const m = value as { amount_minor?: unknown; currency?: unknown };
        return `${Number(m.amount_minor)}:${String(m.currency ?? "").toUpperCase()}`;
      }
      const n = Number(value);
      return Number.isFinite(n) ? `${Math.round(n * 100)}:` : undefined;
    }
    case "email":
      return String(value).trim().toLowerCase();
    default:
      return typeof value === "string" ? value.trim() : value;
  }
}

function sameMoney(got: unknown, want: unknown): boolean {
  const a = normalizeScalar("money", got);
  const b = normalizeScalar("money", want);
  if (typeof a !== "string" || typeof b !== "string") return false;
  // A bare amount in the condition matches any currency.
  if (b.endsWith(":")) return a.split(":")[0] === b.slice(0, -1);
  return a === b;
}

function matches(ref: ConditionDef, got: unknown, want: unknown): boolean {
  if (Array.isArray(got)) {
    // Multiselect: the wanted option(s) must all be among the picked ones.
    const picked = got.map((v) => String(v));
    if (Array.isArray(want)) {
      return want.every((w) => picked.includes(String(w)));
    }
    return typeof want === "string" && picked.includes(want.trim());
  }
  if (ref.type === "money") return sameMoney(got, want);
  const a = normalizeScalar(ref.type, got);
  const b = normalizeScalar(ref.type, want);
  return a !== undefined && b !== undefined && a === b;
}

/** Evaluates one condition against current values. */
export function conditionHolds(
  condition: CustomFieldCondition | null | undefined,
  defsByKey: Map<string, ConditionDef>,
  values: CustomFieldValues,
): boolean {
  if (!condition) return true;
  const got = values[condition.field];
  const set = !isEmptyValue(got);
  switch (condition.operator) {
    case "is_set":
      return set;
    case "is_not_set":
      return !set;
  }
  const ref = defsByKey.get(condition.field);
  if (!set || !ref) return condition.operator === "not_equals";
  switch (condition.operator) {
    case "equals":
      return matches(ref, got, condition.value);
    case "not_equals":
      return !matches(ref, got, condition.value);
    case "in": {
      const list = Array.isArray(condition.value) ? condition.value : [];
      return list.some((v) => matches(ref, got, v));
    }
  }
  return false;
}

/**
 * Keys of fields whose condition holds. Conditions may chain: a field whose
 * parent is hidden behaves as if the parent were empty (the API drops hidden
 * values), so values of hidden fields are removed until the set is stable.
 */
export function activeFieldKeys(
  defs: readonly ConditionDef[],
  values: CustomFieldValues,
): Set<string> {
  const byKey = new Map(defs.map((d) => [d.key, d]));
  const effective: CustomFieldValues = { ...values };
  const active = new Set(defs.map((d) => d.key));
  for (let i = 0; i <= defs.length; i++) {
    let changed = false;
    for (const def of defs) {
      if (!def.condition || !active.has(def.key)) continue;
      if (!conditionHolds(def.condition, byKey, effective)) {
        active.delete(def.key);
        delete effective[def.key];
        changed = true;
      }
    }
    if (!changed) break;
  }
  return active;
}

/** Whether `def` is required with the current values (condition aware). */
export function isFieldRequired(
  def: Pick<CustomFieldDefinition, "key" | "required">,
  active: Set<string>,
): boolean {
  return def.required && active.has(def.key);
}
