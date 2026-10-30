import { z } from "zod";

import type { Team, TeamInput } from "@/features/teams/types";
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

/** Client checks mirror `POST / PATCH /v1/tenant/teams`. */
export function createTeamSchema(t: Translate) {
  const day = createWeekDaySchema(t);
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("teams.validation.name_required"))
      .max(120, t("teams.validation.name_too_long")),
    description: z
      .string()
      .max(1000, t("teams.validation.description_too_long")),
    color: z
      .string()
      .trim()
      .refine(
        (value) => value === "" || COLOR_RE.test(value),
        t("teams.validation.color"),
      ),
    skills: z.array(z.string()).max(30, t("teams.validation.too_many_skills")),
    inherit_hours: z.boolean(),
    timezone: z.string(),
    working_hours: z.object({ days: z.array(day).length(7) }),
  });
}

export type TeamFormValues = z.infer<ReturnType<typeof createTeamSchema>>;

export function teamToForm(team?: Team): TeamFormValues {
  const own = team?.working_hours ?? null;
  const days = own
    ? WEEK_DAYS.map((day) => {
        const row = own.days.find((d) => d.day === day);
        return {
          day,
          open: row?.open ?? false,
          start: row?.start || "09:00",
          end: row?.end || "18:00",
        };
      })
    : defaultWeekDays();
  return {
    name: team?.name ?? "",
    description: team?.description ?? "",
    color: team?.color ?? "",
    skills: team?.skills ?? [],
    inherit_hours: team ? own === null : true,
    timezone: team?.timezone ?? "",
    working_hours: { days },
  };
}

/** Form → request. `inherit_hours` drops the team's own hours and timezone. */
export function formToTeamInput(values: TeamFormValues): TeamInput {
  const base: TeamInput = {
    name: values.name.trim(),
    description: values.description.trim(),
    color: values.color.trim().toUpperCase(),
    skills: values.skills,
  };
  if (values.inherit_hours) return { ...base, inherit_hours: true };
  return {
    ...base,
    working_hours: values.working_hours,
    ...(values.timezone ? { timezone: values.timezone } : {}),
  };
}
