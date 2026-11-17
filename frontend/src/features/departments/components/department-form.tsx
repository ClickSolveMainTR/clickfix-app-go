"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import {
  AppCombobox,
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormSection,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { useBrands } from "@/features/brands/hooks/use-brands";
import {
  createDepartmentSchema,
  departmentToForm,
  formToDepartmentInput,
  NONE,
  type DepartmentFormValues,
} from "@/features/departments/schemas/department-form";
import type { Department, DepartmentInput } from "@/features/departments/types";
import { useActiveOrganization } from "@/features/organizations";
import { slaService } from "@/features/sla/services/sla.service";
import { useTeamsList } from "@/features/teams/hooks/use-teams";
import { WeekHoursEditor } from "@/features/tenant-settings/components/week-hours-editor";
import { timeZones } from "@/features/tenant-settings/constants";
import { useLocale } from "@/providers/locale-provider";

function ColorField({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const { setValue } = useFormContext<DepartmentFormValues>();
  const color = useWatch<DepartmentFormValues, "color">({ name: "color" });
  const valid = /^#[0-9A-Fa-f]{6}$/.test(color ?? "");
  return (
    <div className="flex items-start gap-3">
      <input
        type="color"
        aria-label={t("departments.fields.color")}
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
        label={t("departments.fields.color")}
        placeholder="#2563EB"
        maxLength={7}
        className="flex-1"
        disabled={disabled}
      />
    </div>
  );
}

/** Teams, brand and SLA policy pickers. */
function ScopeFields({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const teams = useTeamsList({ limit: 100, offset: 0, sort: "name" });
  const brands = useBrands();
  const canReadSla = (org?.permissions ?? []).includes(
    Permission.TenantSlaRead,
  );
  const policies = useQuery({
    queryKey: ["tenant", org?.slug, "sla-policies"],
    queryFn: slaService.policies,
    enabled: Boolean(org) && canReadSla,
    staleTime: 60_000,
  });
  const teamOptions = (teams.data?.items ?? []).map((team) => ({
    value: team.uuid,
    label: team.name,
    hint: team.department?.name,
  }));
  const none = { value: NONE, label: t("departments.fields.none") };
  return (
    <FormSection
      title={t("departments.form.section_scope")}
      description={t("departments.form.section_scope_desc")}
      columns={2}
    >
      <AppMultiCombobox
        name="team_uuids"
        label={t("departments.fields.teams")}
        description={t("departments.fields.teams_hint")}
        options={teamOptions}
        placeholder={t("departments.fields.teams_placeholder")}
        className="sm:col-span-2"
        disabled={disabled}
      />
      <AppSelect
        name="brand_uuid"
        label={t("departments.fields.brand")}
        description={t("departments.fields.brand_hint")}
        options={[
          none,
          ...(brands.data ?? []).map((b) => ({ value: b.uuid, label: b.name })),
        ]}
        disabled={disabled}
      />
      <AppSelect
        name="sla_policy_uuid"
        label={t("departments.fields.sla_policy")}
        description={t("departments.fields.sla_policy_hint")}
        options={[
          none,
          ...(policies.data ?? []).map((p) => ({
            value: p.uuid,
            label: p.name,
          })),
        ]}
        disabled={disabled}
      />
    </FormSection>
  );
}

function HoursFields({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const inherit = useWatch<DepartmentFormValues, "inherit_hours">({
    name: "inherit_hours",
  });
  const zoneOptions = useMemo(
    () => timeZones().map((zone) => ({ value: zone, label: zone })),
    [],
  );
  return (
    <FormSection
      title={t("departments.form.section_hours")}
      description={t("departments.form.section_hours_desc")}
    >
      <AppSwitch
        name="inherit_hours"
        label={t("departments.fields.inherit_hours")}
        description={t("departments.fields.inherit_hours_hint")}
        disabled={disabled}
      />
      {inherit ? null : (
        <>
          <AppCombobox
            name="timezone"
            label={t("departments.fields.timezone")}
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
 * Department form (create and edit, CF-215): name and color, teams, brand,
 * SLA policy, widget visibility, working hours and AI settings.
 */
export function DepartmentForm({
  department,
  disabled,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  department?: Department;
  disabled?: boolean;
  submitLabel: string;
  onSubmit: (input: DepartmentInput) => Promise<unknown>;
  onCancel?: () => void;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createDepartmentSchema(t), [t]);
  return (
    <AppForm<DepartmentFormValues>
      key={department?.updated_at ?? "new"}
      className="space-y-6"
      schema={schema}
      defaultValues={departmentToForm(department)}
      onSubmit={async (values) => {
        await onSubmit(formToDepartmentInput(values));
      }}
    >
      {(form) => (
        <>
          <FormSection
            title={t("departments.form.section_general")}
            description={t("departments.form.section_general_desc")}
            columns={2}
          >
            <AppInput
              name="name"
              label={t("departments.fields.name")}
              disabled={disabled}
            />
            <ColorField disabled={disabled} />
            <AppTextarea
              name="description"
              label={t("departments.fields.description")}
              description={t("departments.fields.description_hint")}
              className="sm:col-span-2"
              rows={3}
              disabled={disabled}
            />
            <AppSwitch
              name="widget_visible"
              label={t("departments.fields.widget_visible")}
              description={t("departments.fields.widget_visible_hint")}
              disabled={disabled}
            />
            <AppInput
              name="sort_order"
              type="number"
              label={t("departments.fields.sort_order")}
              disabled={disabled}
            />
          </FormSection>
          <ScopeFields disabled={disabled} />
          <HoursFields disabled={disabled} />
          <FormSection
            title={t("departments.form.section_ai")}
            description={t("departments.form.section_ai_desc")}
          >
            <AppSwitch
              name="ai_enabled"
              label={t("departments.fields.ai_enabled")}
              description={t("departments.fields.ai_enabled_hint")}
              disabled={disabled}
            />
            <AppTextarea
              name="routing_hint"
              label={t("departments.fields.routing_hint")}
              description={t("departments.fields.routing_hint_hint")}
              rows={2}
              disabled={disabled}
            />
            <AppTextarea
              name="ai_instructions"
              label={t("departments.fields.ai_instructions")}
              description={t("departments.fields.ai_instructions_hint")}
              rows={4}
              disabled={disabled}
            />
          </FormSection>
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
                  (Boolean(department) && !form.formState.isDirty)
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
