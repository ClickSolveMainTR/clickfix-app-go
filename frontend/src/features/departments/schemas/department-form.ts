import { z } from "zod";

import type { Department, DepartmentInput } from "@/features/departments/types";
import { WEEK_DAYS } from "@/features/tenant-settings/constants";
import {
  createWeekDaySchema,
  defaultWeekDays,
} from "@/features/tenant-settings/schemas/settings-form";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

const COLOR_RE = /^#[0-9A-Fa-f]{6}$/;

/** "none" stands for an empty select value (no brand / no SLA policy). */
export const NONE = "none";

/** Client checks mirror `POST / PATCH /v1/tenant/departments` (CF-215). */
export function createDepartmentSchema(t: Translate) {
  const day = createWeekDaySchema(t);
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("departments.validation.name_required"))
      .max(120, t("departments.validation.name_too_long")),
    description: z
      .string()
      .max(1000, t("departments.validation.description_too_long")),
    color: z
      .string()
      .trim()
      .refine(
        (value) => value === "" || COLOR_RE.test(value),
        t("departments.validation.color"),
      ),
    brand_uuid: z.string(),
    sla_policy_uuid: z.string(),
    widget_visible: z.boolean(),
    sort_order: z.coerce.number().int().min(-1000).max(1000),
    team_uuids: z.array(z.string()),
    inherit_hours: z.boolean(),
    timezone: z.string(),
    working_hours: z.object({ days: z.array(day).length(7) }),
    ai_enabled: z.boolean(),
    ai_instructions: z
      .string()
      .max(4000, t("departments.validation.instructions_too_long")),
    routing_hint: z
      .string()
      .max(1000, t("departments.validation.routing_hint_too_long")),
  });
}

export type DepartmentFormValues = z.infer<
  ReturnType<typeof createDepartmentSchema>
>;

export function departmentToForm(d?: Department): DepartmentFormValues {
  const own = d?.working_hours ?? null;
  const days = own
    ? WEEK_DAYS.map((day) => {
        const row = own.days.find((x) => x.day === day);
        return {
          day,
          open: row?.open ?? false,
          start: row?.start || "09:00",
          end: row?.end || "18:00",
        };
      })
    : defaultWeekDays();
  return {
    name: d?.name ?? "",
    description: d?.description ?? "",
    color: d?.color ?? "",
    brand_uuid: d?.brand_uuid ?? NONE,
    sla_policy_uuid: d?.sla_policy_uuid ?? NONE,
    widget_visible: d?.widget_visible ?? true,
    sort_order: d?.sort_order ?? 0,
    team_uuids: (d?.teams ?? []).map((team) => team.uuid),
    inherit_hours: d ? own === null : true,
    timezone: d?.timezone ?? "",
    working_hours: { days },
    ai_enabled: d?.ai.enabled ?? true,
    ai_instructions: d?.ai.instructions ?? "",
    routing_hint: d?.ai.routing_hint ?? "",
  };
}

/** Form → request. `inherit_hours` drops the department's own hours. */
export function formToDepartmentInput(
  values: DepartmentFormValues,
): DepartmentInput {
  const base: DepartmentInput = {
    name: values.name.trim(),
    description: values.description.trim(),
    color: values.color.trim().toUpperCase(),
    brand_uuid: values.brand_uuid === NONE ? null : values.brand_uuid,
    sla_policy_uuid:
      values.sla_policy_uuid === NONE ? null : values.sla_policy_uuid,
    widget_visible: values.widget_visible,
    sort_order: values.sort_order,
    team_uuids: values.team_uuids,
    ai: {
      enabled: values.ai_enabled,
      instructions: values.ai_instructions.trim(),
      routing_hint: values.routing_hint.trim(),
    },
  };
  if (values.inherit_hours) return { ...base, inherit_hours: true };
  return {
    ...base,
    working_hours: values.working_hours,
    ...(values.timezone ? { timezone: values.timezone } : {}),
  };
}
