"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useLocale } from "@/providers/locale-provider";

import { hasOptions, type ConditionDraft } from "../../lib/definition-draft";
import { toInputValue } from "../../lib/values";
import {
  CONDITION_OPERATORS,
  type CustomFieldConditionOperator,
  type CustomFieldDefinition,
} from "../../types";
import { CustomFieldInput } from "../custom-field-input";

type ConditionEditorProps = {
  value: ConditionDraft;
  onChange: (value: ConditionDraft) => void;
  /** Fields the condition may refer to (same entity, not this one). */
  candidates: readonly CustomFieldDefinition[];
  errors?: { field?: string; operator?: string; value?: string };
  disabled?: boolean;
};

/** "Show this field only when …" editor. */
export function ConditionEditor({
  value,
  onChange,
  candidates,
  errors,
  disabled,
}: ConditionEditorProps) {
  const { t } = useLocale();
  const ref = candidates.find((c) => c.key === value.field);
  const needsValue =
    value.operator !== "is_set" && value.operator !== "is_not_set";

  const resetValue = (
    nextRef: CustomFieldDefinition | undefined,
    operator: CustomFieldConditionOperator,
  ): unknown => {
    if (!nextRef || operator === "is_set" || operator === "is_not_set") {
      return "";
    }
    if (operator === "in") return hasOptions(nextRef.type) ? [] : "";
    return toInputValue(nextRef, undefined);
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Switch
          id="cf-condition-enabled"
          checked={value.enabled}
          disabled={disabled || candidates.length === 0}
          onCheckedChange={(enabled) => {
            const first = value.field ? ref : candidates[0];
            onChange({
              ...value,
              enabled,
              field: value.field || first?.key || "",
              value: value.field
                ? value.value
                : resetValue(first, value.operator),
            });
          }}
        />
        <Label htmlFor="cf-condition-enabled" className="font-normal">
          {t("custom_fields.condition.enable")}
        </Label>
      </div>
      {candidates.length === 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("custom_fields.condition.no_candidates")}
        </p>
      ) : null}
      {value.enabled ? (
        <div className="grid gap-2 sm:grid-cols-[1fr_10rem]">
          <div className="flex flex-col gap-1">
            <Select
              value={value.field}
              disabled={disabled}
              onValueChange={(field) => {
                const nextRef = candidates.find((c) => c.key === field);
                onChange({
                  ...value,
                  field,
                  value: resetValue(nextRef, value.operator),
                });
              }}
            >
              <SelectTrigger
                aria-label={t("custom_fields.condition.field")}
                aria-invalid={Boolean(errors?.field)}
              >
                <SelectValue placeholder={t("custom_fields.condition.field")} />
              </SelectTrigger>
              <SelectContent>
                {candidates.map((c) => (
                  <SelectItem key={c.key} value={c.key}>
                    {c.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {errors?.field ? (
              <p className="text-destructive text-xs">{errors.field}</p>
            ) : null}
          </div>
          <Select
            value={value.operator}
            disabled={disabled}
            onValueChange={(op) => {
              const operator = op as CustomFieldConditionOperator;
              const sameShape =
                (operator === "in") === (value.operator === "in") &&
                needsValue ===
                  (operator !== "is_set" && operator !== "is_not_set");
              onChange({
                ...value,
                operator,
                value: sameShape ? value.value : resetValue(ref, operator),
              });
            }}
          >
            <SelectTrigger aria-label={t("custom_fields.condition.operator")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CONDITION_OPERATORS.map((op) => (
                <SelectItem key={op} value={op}>
                  {t(`custom_fields.condition.operators.${op}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {needsValue && ref ? (
            <div className="flex flex-col gap-1 sm:col-span-2">
              {value.operator === "in" && !hasOptions(ref.type) ? (
                <Input
                  value={typeof value.value === "string" ? value.value : ""}
                  placeholder={t("custom_fields.filters.list_placeholder")}
                  disabled={disabled}
                  aria-invalid={Boolean(errors?.value)}
                  onChange={(e) =>
                    onChange({ ...value, value: e.target.value })
                  }
                />
              ) : (
                <CustomFieldInput
                  definition={
                    value.operator === "in"
                      ? { ...ref, type: "multiselect", config: {} }
                      : { ...ref, config: { ...ref.config, placeholder: "" } }
                  }
                  value={value.value}
                  onChange={(next) => onChange({ ...value, value: next })}
                  disabled={disabled}
                  invalid={Boolean(errors?.value)}
                  id="cf-condition-value"
                />
              )}
              {errors?.value ? (
                <p className="text-destructive text-xs">{errors.value}</p>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
