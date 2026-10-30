import { z } from "zod";

import { hasOptions, type DefinitionDraft } from "../lib/definition-draft";
import { KEY_PATTERN } from "../lib/values";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Client checks of the field definition dialog (the API re-validates). */
export function fieldDefinitionSchema(t: Translate) {
  return z.custom<DefinitionDraft>().superRefine((draft, ctx) => {
    const label = draft.label.trim();
    if (!label) {
      ctx.addIssue({
        code: "custom",
        path: ["label"],
        message: t("custom_fields.validation.label_required"),
      });
    } else if (label.length > 100) {
      ctx.addIssue({
        code: "custom",
        path: ["label"],
        message: t("custom_fields.validation.label_length"),
      });
    }
    if (!KEY_PATTERN.test(draft.key.trim())) {
      ctx.addIssue({
        code: "custom",
        path: ["key"],
        message: t("custom_fields.validation.key_format"),
      });
    }
    if (hasOptions(draft.type)) {
      const used = draft.options.filter((o) => o.saved || o.label.trim());
      if (!used.length) {
        ctx.addIssue({
          code: "custom",
          path: ["options"],
          message: t("custom_fields.validation.options_required"),
        });
      }
      const seen = new Set<string>();
      draft.options.forEach((o, i) => {
        if (!o.saved && !o.label.trim()) return;
        if (!o.label.trim()) {
          ctx.addIssue({
            code: "custom",
            path: ["options", i],
            message: t("custom_fields.validation.option_label"),
          });
        }
        if (seen.has(o.value)) {
          ctx.addIssue({
            code: "custom",
            path: ["options", i],
            message: t("custom_fields.validation.option_duplicate"),
          });
        }
        seen.add(o.value);
      });
    }
    const c = draft.config;
    const min = Number(c.min.replace(",", "."));
    const max = Number(c.max.replace(",", "."));
    if (c.min.trim() && c.max.trim() && min > max) {
      ctx.addIssue({
        code: "custom",
        path: ["config", "min"],
        message: t("custom_fields.validation.min_max"),
      });
    }
    if (c.currency.trim() && !/^[A-Za-z]{3}$/.test(c.currency.trim())) {
      ctx.addIssue({
        code: "custom",
        path: ["config", "currency"],
        message: t("custom_fields.validation.currency"),
      });
    }
    if (draft.condition.enabled && !draft.condition.field) {
      ctx.addIssue({
        code: "custom",
        path: ["condition", "field"],
        message: t("custom_fields.validation.condition_field"),
      });
    }
  });
}
