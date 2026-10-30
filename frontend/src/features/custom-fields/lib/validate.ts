import type { CustomFieldDefinition, CustomFieldValues } from "../types";
import { activeFieldKeys, isEmptyValue } from "./conditions";
import { toApiValue, toApiValues } from "./values";

export type CustomFieldIssueCode =
  | "required"
  | "number"
  | "amount"
  | "email"
  | "url"
  | "max_selected"
  | "min"
  | "max";

export type CustomFieldIssue = {
  key: string;
  code: CustomFieldIssueCode;
  params?: Record<string, number>;
};

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Client-side checks on form (input) values before they reach the API:
 * required (condition aware) plus cheap format checks. The API stays the
 * authority; its `custom_fields.<key>` errors are mapped onto the same
 * fields.
 */
export function validateCustomFieldValues(
  defs: readonly CustomFieldDefinition[],
  values: CustomFieldValues | null | undefined,
): CustomFieldIssue[] {
  const live = defs.filter((d) => !d.archived);
  const active = activeFieldKeys(live, toApiValues(live, values));
  const issues: CustomFieldIssue[] = [];
  for (const def of live) {
    if (!active.has(def.key)) continue;
    const raw = values?.[def.key];
    const api = toApiValue(def, raw);
    const config = def.config ?? {};

    if (isEmptyValue(api)) {
      const typedSomething =
        (def.type === "number" &&
          typeof raw === "string" &&
          raw.trim() !== "") ||
        (def.type === "money" &&
          typeof raw === "object" &&
          raw !== null &&
          String((raw as { amount?: unknown }).amount ?? "").trim() !== "");
      if (typedSomething) {
        issues.push({
          key: def.key,
          code: def.type === "money" ? "amount" : "number",
        });
      } else if (def.required) {
        issues.push({ key: def.key, code: "required" });
      }
      continue;
    }

    switch (def.type) {
      case "number":
      case "money": {
        const n =
          def.type === "money"
            ? (api as { amount_minor: number }).amount_minor / 100
            : (api as number);
        if (config.min !== undefined && config.min !== null && n < config.min) {
          issues.push({
            key: def.key,
            code: "min",
            params: { min: config.min },
          });
        } else if (
          config.max !== undefined &&
          config.max !== null &&
          n > config.max
        ) {
          issues.push({
            key: def.key,
            code: "max",
            params: { max: config.max },
          });
        }
        break;
      }
      case "email":
        if (!EMAIL_RE.test(String(api))) {
          issues.push({ key: def.key, code: "email" });
        }
        break;
      case "url":
        if (!/^https?:\/\/[^\s/]+/i.test(String(api))) {
          issues.push({ key: def.key, code: "url" });
        }
        break;
      case "multiselect":
        if (
          config.max_selected &&
          Array.isArray(api) &&
          api.length > config.max_selected
        ) {
          issues.push({
            key: def.key,
            code: "max_selected",
            params: { count: config.max_selected },
          });
        }
        break;
    }
  }
  return issues;
}
