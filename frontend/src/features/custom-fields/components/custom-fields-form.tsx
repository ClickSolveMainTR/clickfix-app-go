"use client";

import get from "lodash/get";
import { useMemo } from "react";
import {
  Controller,
  useFormContext,
  useWatch,
  type FieldValues,
  type UseFormSetError,
} from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { activeFieldKeys } from "../lib/conditions";
import { customFieldErrors } from "../lib/errors";
import { toApiValues } from "../lib/values";
import type { CustomFieldDefinition, CustomFieldValues } from "../types";
import { CustomFieldInput } from "./custom-field-input";

type CustomFieldsFormProps = {
  /** Definitions of the entity; archived ones are skipped. */
  definitions: readonly CustomFieldDefinition[];
  /** Form path holding the values object. Default `custom_fields`. */
  name?: string;
  disabled?: boolean;
  /** Grid columns on wide screens. Default 1. */
  columns?: 1 | 2;
  className?: string;
};

/**
 * Renders every active definition of an entity inside a react-hook-form
 * form (`FormProvider` / `AppForm`). Values live under `name.<key>` in input
 * shape (see `toInputValues`); conditional fields are hidden while their
 * condition does not hold and required ones get a marker. Convert with
 * `buildCustomFieldsPayload` on submit.
 */
export function CustomFieldsForm({
  definitions,
  name = "custom_fields",
  disabled,
  columns = 1,
  className,
}: CustomFieldsFormProps) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const watched = useWatch({ control, name }) as CustomFieldValues | undefined;

  const live = useMemo(
    () =>
      definitions
        .filter((d) => !d.archived)
        .sort((a, b) => a.position - b.position),
    [definitions],
  );
  const active = useMemo(
    () => activeFieldKeys(live, toApiValues(live, watched)),
    [live, watched],
  );

  if (!live.length) return null;

  return (
    <div
      className={cn("grid gap-4", columns === 2 && "sm:grid-cols-2", className)}
    >
      {live.map((def) => {
        if (!active.has(def.key)) return null;
        const path = `${name}.${def.key}`;
        const error = get(errors, path)?.message as string | undefined;
        const wide =
          columns === 2 &&
          (def.type === "textarea" || def.type === "multiselect");
        return (
          <FormFieldShell
            key={def.uuid}
            name={`cf-${def.key}`}
            label={
              <>
                {def.label}
                {def.required ? (
                  <span
                    className="text-destructive"
                    aria-label={t("custom_fields.required")}
                  >
                    *
                  </span>
                ) : null}
              </>
            }
            description={def.description || undefined}
            error={error}
            className={cn(wide && "sm:col-span-2")}
          >
            <Controller
              control={control}
              name={path}
              render={({ field }) => (
                <CustomFieldInput
                  definition={def}
                  value={field.value}
                  onChange={field.onChange}
                  onBlur={field.onBlur}
                  id={`cf-${def.key}`}
                  disabled={disabled}
                  invalid={Boolean(error)}
                />
              )}
            />
          </FormFieldShell>
        );
      })}
    </div>
  );
}

/**
 * Maps `custom_fields.<key>` errors of an API validation error onto form
 * fields under `name.<key>`, with localized messages. Returns true when at
 * least one field error was set.
 */
export function applyCustomFieldApiErrors<T extends FieldValues>(
  error: unknown,
  setError: UseFormSetError<T>,
  t: (key: string, params?: Record<string, string | number>) => string,
  options: { name?: string; apiPrefix?: string } = {},
): boolean {
  if (!(error instanceof ApiError) || !error.isValidation) return false;
  const name = options.name ?? "custom_fields";
  const mapped = customFieldErrors(
    error.details,
    options.apiPrefix ?? "custom_fields",
  );
  const entries = Object.entries(mapped);
  for (const [key, detail] of entries) {
    const translated =
      detail.code === "required" ||
      detail.code === "archived" ||
      detail.code === "unknown"
        ? t(`custom_fields.api_errors.${detail.code}`)
        : detail.message
          ? t("custom_fields.api_errors.invalid_detail", {
              message: detail.message,
            })
          : t("custom_fields.api_errors.invalid");
    setError(`${name}.${key}` as never, {
      type: "server",
      message: translated,
    });
  }
  return entries.length > 0;
}
