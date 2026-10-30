/** Error detail as the API sends it (`error.details[]`). */
export type FieldErrorDetail = {
  field?: string;
  code?: string;
  message?: string;
};

export type CustomFieldError = { code: string; message: string };

/**
 * Picks the custom field errors out of a validation error: details with
 * `field: "<prefix>.<key>"` (records use `custom_fields.<key>`).
 */
export function customFieldErrors(
  details: readonly FieldErrorDetail[] | null | undefined,
  prefix = "custom_fields",
): Record<string, CustomFieldError> {
  const out: Record<string, CustomFieldError> = {};
  const head = `${prefix}.`;
  for (const d of details ?? []) {
    if (!d.field?.startsWith(head)) continue;
    const key = d.field.slice(head.length);
    if (!key) continue;
    out[key] = { code: d.code ?? "invalid", message: d.message ?? "" };
  }
  return out;
}
