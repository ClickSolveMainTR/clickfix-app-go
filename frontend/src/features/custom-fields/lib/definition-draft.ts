import type {
  CustomFieldCondition,
  CustomFieldConditionOperator,
  CustomFieldConfig,
  CustomFieldCreateRequest,
  CustomFieldDefinition,
  CustomFieldEntityType,
  CustomFieldOption,
  CustomFieldType,
  CustomFieldUpdateRequest,
  CustomFieldVisibility,
  CustomFieldWidgetAccess,
} from "../types";
import { DEFAULT_VISIBILITY } from "../types";
import { isEmptyValue } from "./conditions";
import { slugifyKey, toApiValue, toInputValue } from "./values";

/** Editable option row; `saved` options can only be archived, not removed. */
export type OptionDraft = {
  value: string;
  label: string;
  color: string;
  archived: boolean;
  saved: boolean;
};

/** Config inputs as typed (strings); empty means "not set". */
export type ConfigDraft = {
  max_length: string;
  min: string;
  max: string;
  precision: string;
  currency: string;
  max_selected: string;
  placeholder: string;
};

export type ConditionDraft = {
  enabled: boolean;
  field: string;
  operator: CustomFieldConditionOperator;
  /** Input shape of the referenced field; `string[]` / text list for `in`. */
  value: unknown;
};

export type DefinitionDraft = {
  label: string;
  key: string;
  type: CustomFieldType;
  description: string;
  required: boolean;
  options: OptionDraft[];
  config: ConfigDraft;
  /** Input shape of this field (see `toInputValue`). */
  default: unknown;
  condition: ConditionDraft;
  /** CF-196: who reads the value, masking and widget write access. */
  visibility: CustomFieldVisibility;
  sensitive: boolean;
  widgetAccess: CustomFieldWidgetAccess;
  /** Received key of the "tanımsız alan" warning this field answers. */
  fromUnknownKey: string;
};

export const OPTION_TYPES: readonly CustomFieldType[] = [
  "select",
  "multiselect",
];

export function hasOptions(type: CustomFieldType): boolean {
  return OPTION_TYPES.includes(type);
}

/** Which config inputs a type uses. */
export function configKeysFor(type: CustomFieldType): (keyof ConfigDraft)[] {
  switch (type) {
    case "text":
    case "textarea":
      return ["max_length", "placeholder"];
    case "number":
      return ["min", "max", "precision", "placeholder"];
    case "money":
      return ["currency", "min", "max", "placeholder"];
    case "multiselect":
      return ["max_selected", "placeholder"];
    case "boolean":
      return [];
    default:
      return ["placeholder"];
  }
}

const EMPTY_CONFIG: ConfigDraft = {
  max_length: "",
  min: "",
  max: "",
  precision: "",
  currency: "",
  max_selected: "",
  placeholder: "",
};

function numStr(v: number | undefined | null): string {
  return v === undefined || v === null ? "" : String(v);
}

export function emptyDraft(type: CustomFieldType = "text"): DefinitionDraft {
  return {
    label: "",
    key: "",
    type,
    description: "",
    required: false,
    options: [],
    config: { ...EMPTY_CONFIG },
    default: toInputValue({ type, config: {} }, undefined),
    condition: { enabled: false, field: "", operator: "equals", value: "" },
    visibility: { ...DEFAULT_VISIBILITY },
    sensitive: false,
    widgetAccess: "verified",
    fromUnknownKey: "",
  };
}

/** Condition value (API) → input shape for the editor. */
export function conditionValueToInput(
  ref: Pick<CustomFieldDefinition, "type" | "config"> | undefined,
  operator: CustomFieldConditionOperator,
  value: unknown,
): unknown {
  if (operator === "is_set" || operator === "is_not_set") return "";
  if (operator === "in") {
    const list = Array.isArray(value) ? value : [];
    if (ref && hasOptions(ref.type)) return list.map(String);
    return list.map((v) => String(v)).join(", ");
  }
  return ref ? toInputValue(ref, value) : "";
}

/** Condition editor value → API value. */
export function conditionValueToApi(
  ref: Pick<CustomFieldDefinition, "type" | "config"> | undefined,
  operator: CustomFieldConditionOperator,
  value: unknown,
): unknown {
  if (operator === "is_set" || operator === "is_not_set") return undefined;
  if (operator === "in") {
    if (Array.isArray(value)) return value.length ? value : null;
    const parts = String(value ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (!parts.length) return null;
    return ref
      ? parts.map((p) => toApiValue({ ...ref, type: ref.type }, p) ?? p)
      : parts;
  }
  return ref ? toApiValue(ref, value) : value;
}

export function draftFromDefinition(
  def: CustomFieldDefinition,
  others: readonly CustomFieldDefinition[],
): DefinitionDraft {
  const c = def.config ?? {};
  const ref = def.condition
    ? others.find((o) => o.key === def.condition?.field)
    : undefined;
  return {
    label: def.label,
    key: def.key,
    type: def.type,
    description: def.description ?? "",
    required: def.required,
    options: (def.options ?? []).map((o) => ({
      value: o.value,
      label: o.label,
      color: o.color ?? "",
      archived: Boolean(o.archived),
      saved: true,
    })),
    config: {
      max_length: c.max_length ? String(c.max_length) : "",
      min: numStr(c.min),
      max: numStr(c.max),
      precision: numStr(c.precision),
      currency: c.currency ?? "",
      max_selected: c.max_selected ? String(c.max_selected) : "",
      placeholder: c.placeholder ?? "",
    },
    default: toInputValue(def, def.default),
    condition: def.condition
      ? {
          enabled: true,
          field: def.condition.field,
          operator: def.condition.operator,
          value: conditionValueToInput(
            ref,
            def.condition.operator,
            def.condition.value,
          ),
        }
      : { enabled: false, field: "", operator: "equals", value: "" },
    visibility: { ...DEFAULT_VISIBILITY, ...(def.visibility ?? {}) },
    sensitive: Boolean(def.sensitive),
    widgetAccess: def.widget_access ?? "verified",
    fromUnknownKey: "",
  };
}

function toNum(s: string): number | undefined {
  const t = s.trim().replace(",", ".");
  if (t === "") return undefined;
  const n = Number(t);
  return Number.isFinite(n) ? n : undefined;
}

export function configFromDraft(
  type: CustomFieldType,
  draft: ConfigDraft,
): CustomFieldConfig {
  const keys = new Set(configKeysFor(type));
  const out: CustomFieldConfig = {};
  if (keys.has("max_length")) out.max_length = toNum(draft.max_length);
  if (keys.has("min")) out.min = toNum(draft.min);
  if (keys.has("max")) out.max = toNum(draft.max);
  if (keys.has("precision")) out.precision = toNum(draft.precision);
  if (keys.has("currency") && draft.currency.trim()) {
    out.currency = draft.currency.trim().toUpperCase();
  }
  if (keys.has("max_selected")) out.max_selected = toNum(draft.max_selected);
  if (keys.has("placeholder") && draft.placeholder.trim()) {
    out.placeholder = draft.placeholder.trim();
  }
  for (const k of Object.keys(out) as (keyof CustomFieldConfig)[]) {
    if (out[k] === undefined) delete out[k];
  }
  return out;
}

export function optionsFromDraft(
  type: CustomFieldType,
  drafts: readonly OptionDraft[],
): CustomFieldOption[] {
  if (!hasOptions(type)) return [];
  return drafts
    .filter((o) => o.saved || o.label.trim())
    .map((o) => {
      const option: CustomFieldOption = {
        value: o.value.trim() || slugifyKey(o.label) || o.label.trim(),
        label: o.label.trim(),
      };
      if (o.color) option.color = o.color;
      if (o.archived) option.archived = true;
      return option;
    });
}

function conditionFromDraft(
  draft: ConditionDraft,
  others: readonly CustomFieldDefinition[],
): CustomFieldCondition | null {
  if (!draft.enabled || !draft.field) return null;
  const ref = others.find((o) => o.key === draft.field);
  const value = conditionValueToApi(ref, draft.operator, draft.value);
  const condition: CustomFieldCondition = {
    field: draft.field,
    operator: draft.operator,
  };
  if (value !== undefined) condition.value = value;
  return condition;
}

function defaultFromDraft(draft: DefinitionDraft, config: CustomFieldConfig) {
  if (draft.type === "user") return null;
  const v = toApiValue({ type: draft.type, config }, draft.default);
  return isEmptyValue(v) ? null : v;
}

export function createRequestFromDraft(
  draft: DefinitionDraft,
  scope: { entityType: CustomFieldEntityType; objectTypeUuid?: string | null },
  others: readonly CustomFieldDefinition[],
): CustomFieldCreateRequest {
  const config = configFromDraft(draft.type, draft.config);
  return {
    entity_type: scope.entityType,
    object_type_uuid: scope.objectTypeUuid ?? null,
    key: draft.key.trim(),
    label: draft.label.trim(),
    description: draft.description.trim(),
    type: draft.type,
    required: draft.required,
    options: optionsFromDraft(draft.type, draft.options),
    config,
    default: defaultFromDraft(draft, config),
    condition: conditionFromDraft(draft.condition, others),
    visibility: draft.visibility,
    sensitive: draft.sensitive,
    widget_access: draft.widgetAccess,
    from_unknown_key: draft.fromUnknownKey || undefined,
  };
}

export function updateRequestFromDraft(
  draft: DefinitionDraft,
  others: readonly CustomFieldDefinition[],
): CustomFieldUpdateRequest {
  const config = configFromDraft(draft.type, draft.config);
  return {
    label: draft.label.trim(),
    description: draft.description.trim(),
    required: draft.required,
    options: optionsFromDraft(draft.type, draft.options),
    config,
    default: defaultFromDraft(draft, config),
    condition: conditionFromDraft(draft.condition, others),
    visibility: draft.visibility,
    sensitive: draft.sensitive,
    widget_access: draft.widgetAccess,
  };
}

/** Unique option value derived from a label. */
export function uniqueOptionValue(
  label: string,
  taken: readonly string[],
): string {
  const base = slugifyKey(label) || "option";
  let candidate = base;
  let i = 2;
  while (taken.includes(candidate)) {
    candidate = `${base}_${i}`;
    i += 1;
  }
  return candidate;
}
