import type {
  CustomFieldDefinition,
  CustomFieldValues,
  MoneyValue,
} from "../types";
import { activeFieldKeys, isEmptyValue } from "./conditions";

export const DEFAULT_CURRENCY = "TRY";

/** Max key length enforced by the API (`^[a-z][a-z0-9_]{0,62}$`). */
export const KEY_PATTERN = /^[a-z][a-z0-9_]{0,62}$/;

const TURKISH_MAP: Record<string, string> = {
  ç: "c",
  ğ: "g",
  ı: "i",
  İ: "i",
  ö: "o",
  ş: "s",
  ü: "u",
};

/** Field key from a label: lowercase ASCII, `_` separated, starts with a letter. */
export function slugifyKey(label: string): string {
  const ascii = label
    .replace(
      /[çğıİöşü]/gi,
      (c) => TURKISH_MAP[c] ?? TURKISH_MAP[c.toLowerCase()] ?? c,
    )
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
  const withLetter = /^[a-z]/.test(ascii) ? ascii : ascii ? `f_${ascii}` : "";
  return withLetter.slice(0, 63).replace(/_+$/, "");
}

export function isMoneyValue(value: unknown): value is MoneyValue {
  return (
    typeof value === "object" &&
    value !== null &&
    "amount_minor" in value &&
    typeof (value as MoneyValue).amount_minor === "number"
  );
}

/**
 * Form-side money value: amount in major units as typed (string), plus
 * currency. Converted to `amount_minor` on submit.
 */
export type MoneyInputValue = { amount: string; currency: string };

export function moneyToInput(
  value: unknown,
  fallbackCurrency = DEFAULT_CURRENCY,
): MoneyInputValue {
  if (isMoneyValue(value)) {
    return {
      amount: String(value.amount_minor / 100),
      currency: value.currency || fallbackCurrency,
    };
  }
  return { amount: "", currency: fallbackCurrency };
}

function parseNumber(raw: unknown): number | null {
  if (typeof raw === "number") return Number.isFinite(raw) ? raw : null;
  if (typeof raw !== "string") return null;
  const s = raw.trim().replace(",", ".");
  if (s === "") return null;
  const n = Number(s);
  return Number.isFinite(n) ? n : null;
}

/** Stored value → value the input controls work with. */
export function toInputValue(
  def: Pick<CustomFieldDefinition, "type" | "config">,
  value: unknown,
): unknown {
  switch (def.type) {
    case "number":
      return typeof value === "number" ? String(value) : "";
    case "money":
      return moneyToInput(value, def.config?.currency || DEFAULT_CURRENCY);
    case "boolean":
      return typeof value === "boolean" ? value : null;
    case "multiselect":
      return Array.isArray(value) ? value.map(String) : [];
    default:
      return typeof value === "string" ? value : "";
  }
}

/** Input value → API value (`null` clears the field). */
export function toApiValue(
  def: Pick<CustomFieldDefinition, "type" | "config">,
  value: unknown,
): unknown {
  switch (def.type) {
    case "number": {
      return parseNumber(value);
    }
    case "money": {
      const m = (value ?? {}) as Partial<MoneyInputValue>;
      const amount = parseNumber(m.amount);
      if (amount === null) return null;
      return {
        amount_minor: Math.round(amount * 100),
        currency: (m.currency || def.config?.currency || DEFAULT_CURRENCY)
          .trim()
          .toUpperCase(),
      };
    }
    case "boolean":
      return typeof value === "boolean" ? value : null;
    case "multiselect":
      return Array.isArray(value) && value.length ? value : null;
    default: {
      const s = typeof value === "string" ? value.trim() : "";
      return s === "" ? null : s;
    }
  }
}

/** Stored values → form values for every definition. */
export function toInputValues(
  defs: readonly CustomFieldDefinition[],
  stored: CustomFieldValues | null | undefined,
  options: { applyDefaults?: boolean } = {},
): CustomFieldValues {
  const out: CustomFieldValues = {};
  for (const def of defs) {
    let value = stored?.[def.key];
    if (
      options.applyDefaults &&
      isEmptyValue(value) &&
      def.default !== null &&
      def.default !== undefined
    ) {
      value = def.default;
    }
    out[def.key] = toInputValue(def, value);
  }
  return out;
}

/** Form values → API values, for the API side of condition evaluation. */
export function toApiValues(
  defs: readonly CustomFieldDefinition[],
  values: CustomFieldValues | null | undefined,
): CustomFieldValues {
  const out: CustomFieldValues = {};
  for (const def of defs) {
    out[def.key] = toApiValue(def, values?.[def.key]);
  }
  return out;
}

/**
 * Payload of `custom_fields` for create/update: active (non archived)
 * definitions only; hidden conditional fields are sent as `null` so the API
 * clears them. `null` also clears emptied fields.
 */
export function buildCustomFieldsPayload(
  defs: readonly CustomFieldDefinition[],
  values: CustomFieldValues | null | undefined,
  options: { omitEmpty?: boolean } = {},
): CustomFieldValues {
  const live = defs.filter((d) => !d.archived);
  const apiValues = toApiValues(live, values);
  const active = activeFieldKeys(live, apiValues);
  const out: CustomFieldValues = {};
  for (const def of live) {
    const value = active.has(def.key) ? apiValues[def.key] : null;
    if (options.omitEmpty && value === null) continue;
    out[def.key] = value;
  }
  return out;
}
