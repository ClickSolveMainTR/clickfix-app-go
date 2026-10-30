import { z } from "zod";

import { validateCustomFieldValues } from "../lib/validate";
import type { CustomFieldDefinition } from "../types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/**
 * Zod schema for the `custom_fields` object of a form. Compose it into the
 * entity schema (`custom_fields: customFieldsSchema(defs, t)`); issues land
 * on `custom_fields.<key>` so `CustomFieldsForm` shows them under the field.
 */
export function customFieldsSchema(
  defs: readonly CustomFieldDefinition[],
  t: Translate,
) {
  return z.record(z.string(), z.unknown()).superRefine((values, ctx) => {
    for (const issue of validateCustomFieldValues(defs, values)) {
      ctx.addIssue({
        code: "custom",
        path: [issue.key],
        message: t(`custom_fields.validation.${issue.code}`, issue.params),
      });
    }
  });
}
