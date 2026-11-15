"use client";

import { useMemo } from "react";
import { Controller, get, useFormContext, useWatch } from "react-hook-form";

import {
  AppCombobox,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormSection,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { useDepartmentOptions } from "@/features/departments/hooks/use-departments";
import { SkillsInput } from "@/features/teams/components/skills-input";
import { useSkillCatalog } from "@/features/teams/hooks/use-teams";
import {
  createTeamSchema,
  formToTeamInput,
  teamToForm,
  type TeamFormValues,
} from "@/features/teams/schemas/team-form";
import type { Team, TeamInput } from "@/features/teams/types";
import { WeekHoursEditor } from "@/features/tenant-settings/components/week-hours-editor";
import { timeZones } from "@/features/tenant-settings/constants";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function SkillsField({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<TeamFormValues>();
  const catalog = useSkillCatalog(!disabled);
  const error = get(errors, "skills")?.message as string | undefined;
  return (
    <div className="space-y-2 sm:col-span-2">
      <Label htmlFor="team-skills">{t("teams.fields.skills")}</Label>
      <Controller
        control={control}
        name="skills"
        render={({ field }) => (
          <SkillsInput
            id="team-skills"
            value={field.value}
            onChange={field.onChange}
            suggestions={catalog.data?.items ?? []}
            disabled={disabled}
            invalid={Boolean(error)}
          />
        )}
      />
      <p className="text-muted-foreground text-xs">
        {t("teams.fields.skills_hint")}
      </p>
      {error ? <p className="text-destructive text-xs">{error}</p> : null}
    </div>
  );
}

/** Department of the team (CF-215); hidden without departments.read. */
function DepartmentField({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { options } = useDepartmentOptions();
  if (!can(permissions.tenant.departments.read)) return null;
  return (
    <AppSelect
      name="department_uuid"
      label={t("teams.fields.department")}
      description={t("teams.fields.department_hint")}
      options={[
        { value: "none", label: t("teams.fields.no_department") },
        ...options,
      ]}
      disabled={disabled}
    />
  );
}

function ColorField({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const { setValue } = useFormContext<TeamFormValues>();
  const color = useWatch<TeamFormValues, "color">({ name: "color" });
  const valid = /^#[0-9A-Fa-f]{6}$/.test(color ?? "");
  return (
    <div className="flex items-start gap-3">
      <input
        type="color"
        aria-label={t("teams.fields.color")}
        value={valid ? color : "#2563EB"}
        disabled={disabled}
        onChange={(event) =>
          setValue("color", event.target.value.toUpperCase(), {
            shouldDirty: true,
            shouldValidate: true,
          })
        }
        className="border-input mt-6 h-9 w-14 shrink-0 cursor-pointer rounded-md border bg-transparent p-1 disabled:cursor-not-allowed disabled:opacity-50"
      />
      <AppInput
        name="color"
        label={t("teams.fields.color")}
        description={t("teams.fields.color_hint")}
        placeholder="#2563EB"
        maxLength={7}
        className="flex-1"
        disabled={disabled}
      />
    </div>
  );
}

function HoursFields({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const inherit = useWatch<TeamFormValues, "inherit_hours">({
    name: "inherit_hours",
  });
  const zoneOptions = useMemo(
    () => timeZones().map((zone) => ({ value: zone, label: zone })),
    [],
  );
  return (
    <FormSection
      title={t("teams.form.section_hours")}
      description={t("teams.form.section_hours_desc")}
    >
      <AppSwitch
        name="inherit_hours"
        label={t("teams.fields.inherit_hours")}
        description={t("teams.fields.inherit_hours_hint")}
        disabled={disabled}
      />
      {inherit ? null : (
        <>
          <AppCombobox
            name="timezone"
            label={t("teams.fields.timezone")}
            description={t("teams.fields.timezone_hint")}
            options={zoneOptions}
            placeholder={t("tenant_settings.fields.timezone_placeholder")}
            searchPlaceholder={t("tenant_settings.fields.timezone_search")}
            disabled={disabled}
          />
          <WeekHoursEditor name="working_hours.days" disabled={disabled} />
        </>
      )}
    </FormSection>
  );
}

/**
 * Team details form (create and edit): name, description, color, skills and
 * working hours (own, or the organization's). API validation errors such as
 * `working_hours.days.2.end` or `skills.1` land on the same paths.
 */
export function TeamForm({
  team,
  disabled,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  team?: Team;
  disabled?: boolean;
  submitLabel: string;
  onSubmit: (input: TeamInput) => Promise<unknown>;
  onCancel?: () => void;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createTeamSchema(t), [t]);
  return (
    <AppForm<TeamFormValues>
      key={team?.updated_at ?? "new"}
      className="space-y-6"
      schema={schema}
      defaultValues={teamToForm(team)}
      onSubmit={async (values) => {
        await onSubmit(formToTeamInput(values));
      }}
    >
      {(form) => (
        <>
          <FormSection
            title={t("teams.form.section_general")}
            description={t("teams.form.section_general_desc")}
            columns={2}
          >
            <AppInput
              name="name"
              label={t("teams.fields.name")}
              disabled={disabled}
            />
            <ColorField disabled={disabled} />
            <DepartmentField disabled={disabled} />
            <AppTextarea
              name="description"
              label={t("teams.fields.description")}
              className="sm:col-span-2"
              rows={3}
              disabled={disabled}
            />
            <SkillsField disabled={disabled} />
          </FormSection>
          <HoursFields disabled={disabled} />
          {disabled ? null : (
            <FormActions>
              {onCancel ? (
                <Button type="button" variant="outline" onClick={onCancel}>
                  {t("form.cancel")}
                </Button>
              ) : null}
              <Button
                type="submit"
                disabled={
                  form.formState.isSubmitting ||
                  (Boolean(team) && !form.formState.isDirty)
                }
              >
                {submitLabel}
              </Button>
            </FormActions>
          )}
        </>
      )}
    </AppForm>
  );
}
