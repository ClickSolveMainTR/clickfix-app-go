"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityDetail, EntitySectionCard } from "@/components/entity";
import { AppDatePicker, AppForm } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  OrganizationAccessChip,
  OrganizationPlanCell,
  OrganizationStatusChip,
  ownerLabel,
} from "@/features/platform-organizations/components/organization-cells";
import {
  useSetOrganizationAccess,
  useSetOrganizationStatus,
} from "@/features/platform-organizations/hooks/use-organizations";
import {
  endOfDayISO,
  isoToDateInput,
  startOfDayISO,
} from "@/features/platform-organizations/lib/access-dates";
import {
  countryName,
  localeName,
} from "@/features/platform-organizations/lib/region";
import type { OrganizationSummary } from "@/features/platform-organizations/types";
import type { components } from "@/generated/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { OrganizationAttributionCard } from "@/features/platform-organizations/components/organization-attribution-card";

type Company = components["schemas"]["OrganizationCompany"];

const COMPANY_FIELDS: { key: keyof Company; label: string }[] = [
  { key: "legal_name", label: "commerce.org.company.legal_name" },
  { key: "tax_office", label: "commerce.org.company.tax_office" },
  { key: "tax_number", label: "commerce.org.company.tax_number" },
  { key: "city", label: "commerce.org.company.city" },
  { key: "address", label: "commerce.org.company.address" },
  { key: "phone", label: "commerce.org.company.phone" },
  { key: "email", label: "commerce.org.company.email" },
  { key: "website", label: "commerce.org.company.website" },
];

/** Company info saved in the organization settings (wizard / tenant profile). */
function companyOf(org: OrganizationSummary): Partial<Company> | null {
  const raw = (org.settings as { company?: unknown } | undefined)?.company;
  if (!raw || typeof raw !== "object") return null;
  const company = raw as Partial<Company>;
  return COMPANY_FIELDS.some(({ key }) => company[key]) ? company : null;
}

function createAccessSchema(t: (key: string) => string) {
  return z
    .object({ access_starts_at: z.string(), access_ends_at: z.string() })
    .superRefine((values, ctx) => {
      if (!values.access_starts_at) {
        ctx.addIssue({
          code: "custom",
          path: ["access_starts_at"],
          message: t("organizations.validation.access_start_required"),
        });
      }
      if (
        values.access_starts_at &&
        values.access_ends_at &&
        values.access_ends_at < values.access_starts_at
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["access_ends_at"],
          message: t("organizations.validation.access_end_after_start"),
        });
      }
    });
}

type AccessValues = z.infer<ReturnType<typeof createAccessSchema>>;

/** "Erişim ve durum": active / suspended and the access window. */
function AccessCard({ org }: { org: OrganizationSummary }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.organizations.write);
  const setStatus = useSetOrganizationStatus();
  const setAccess = useSetOrganizationAccess();
  const [confirm, setConfirm] = useState(false);
  const schema = useMemo(() => createAccessSchema(t), [t]);
  const suspending = org.status === "active";

  return (
    <EntitySectionCard
      title={t("commerce.org.general.access_title")}
      description={t("commerce.org.general.access_description")}
    >
      <div className="space-y-6">
        <div className="flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <OrganizationStatusChip status={org.status} />
              <OrganizationAccessChip state={org.access_state} />
            </div>
            <p className="text-muted-foreground text-sm">
              {org.status === "active"
                ? t("organizations.status_tab.active_hint")
                : t("organizations.status_tab.suspended_hint")}
            </p>
          </div>
          {canWrite ? (
            <Button
              type="button"
              size="sm"
              variant={suspending ? "outline" : "default"}
              className={suspending ? "text-destructive" : undefined}
              onClick={() => setConfirm(true)}
            >
              {suspending
                ? t("organizations.actions.suspend")
                : t("organizations.actions.activate")}
            </Button>
          ) : null}
        </div>

        <AppForm<AccessValues>
          key={`${org.access_starts_at}|${org.access_ends_at ?? ""}`}
          className="space-y-4"
          schema={schema}
          defaultValues={{
            access_starts_at: isoToDateInput(org.access_starts_at),
            access_ends_at: isoToDateInput(org.access_ends_at),
          }}
          onSubmit={async (values) => {
            await setAccess.mutateAsync({
              uuid: org.uuid,
              body: {
                access_starts_at: startOfDayISO(values.access_starts_at),
                access_ends_at: values.access_ends_at
                  ? (endOfDayISO(values.access_ends_at) ?? null)
                  : null,
              },
            });
          }}
        >
          {(form) => (
            <>
              <p className="text-sm">
                {org.access_ends_at
                  ? t("organizations.status_tab.access_until", {
                      date: datetime(org.access_ends_at, undefined, locale),
                    })
                  : t("organizations.status_tab.access_unlimited")}
              </p>
              <div className="grid gap-4 sm:grid-cols-2">
                <AppDatePicker
                  name="access_starts_at"
                  label={t("organizations.fields.access_starts_at")}
                  disabled={!canWrite}
                />
                <AppDatePicker
                  name="access_ends_at"
                  label={t("organizations.fields.access_ends_at")}
                  description={t("organizations.status_tab.access_ends_hint")}
                  disabled={!canWrite}
                />
              </div>
              {canWrite ? (
                <div className="flex flex-wrap justify-end gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    disabled={form.formState.isSubmitting}
                    onClick={() =>
                      form.setValue("access_ends_at", "", { shouldDirty: true })
                    }
                  >
                    {t("organizations.status_tab.clear_end")}
                  </Button>
                  <Button
                    type="submit"
                    disabled={
                      !form.formState.isDirty || form.formState.isSubmitting
                    }
                  >
                    {t("organizations.actions.save")}
                  </Button>
                </div>
              ) : null}
            </>
          )}
        </AppForm>
      </div>

      <ConfirmDialog
        open={confirm}
        title={
          suspending
            ? t("organizations.status_confirm.suspend_title", {
                name: org.name,
              })
            : t("organizations.status_confirm.activate_title", {
                name: org.name,
              })
        }
        description={
          suspending
            ? t("organizations.status_confirm.suspend_description")
            : t("organizations.status_confirm.activate_description")
        }
        confirmLabel={
          suspending
            ? t("organizations.actions.suspend")
            : t("organizations.actions.activate")
        }
        variant={suspending ? "destructive" : "default"}
        isPending={setStatus.isPending}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          setStatus.mutate(
            { uuid: org.uuid, status: suspending ? "suspended" : "active" },
            { onSettled: () => setConfirm(false) },
          )
        }
      />
    </EntitySectionCard>
  );
}

/**
 * "Genel": identity and company info, owner and plan overview, region and
 * the access card (organization status and access window).
 */
export function OrganizationGeneralTab({ org }: { org: OrganizationSummary }) {
  const { t, locale } = useLocale();
  const company = companyOf(org);
  return (
    <div className="grid items-start gap-6 xl:grid-cols-2">
      <div className="space-y-6">
        <EntitySectionCard title={t("organizations.detail.identity")}>
          <EntityDetail
            sections={[
              {
                id: "identity",
                fields: [
                  {
                    key: "name",
                    label: t("organizations.fields.name"),
                    value: org.name,
                  },
                  {
                    key: "slug",
                    label: t("organizations.fields.slug"),
                    value: <code className="text-xs">{org.slug}</code>,
                  },
                  {
                    key: "owner",
                    label: t("organizations.columns.owner"),
                    value: org.owner
                      ? `${ownerLabel(org.owner)} · ${org.owner.email}`
                      : org.pending_invitation_count > 0
                        ? t("organizations.owner_invited")
                        : "—",
                  },
                  {
                    key: "plan",
                    label: t("organizations.columns.plan"),
                    value: <OrganizationPlanCell org={org} />,
                  },
                  {
                    key: "members",
                    label: t("organizations.columns.member_count"),
                    value: t("commerce.org.general.members_value", {
                      members: org.member_count,
                      seats: org.billable_seats,
                    }),
                  },
                  {
                    key: "invitations",
                    label: t("organizations.detail.pending_invitations"),
                    value: org.pending_invitation_count,
                  },
                  {
                    key: "created_at",
                    label: t("organizations.columns.created_at"),
                    value: datetime(org.created_at, undefined, locale),
                  },
                  {
                    key: "last_activity",
                    label: t("organizations.columns.last_activity_at"),
                    value: org.last_activity_at
                      ? datetime(org.last_activity_at, undefined, locale)
                      : t("organizations.no_activity"),
                  },
                ],
              },
              {
                id: "region",
                title: t("commerce.org.general.region_title"),
                fields: [
                  {
                    key: "locale",
                    label: t("organizations.fields.locale"),
                    value: localeName(org.locale, locale),
                  },
                  {
                    key: "timezone",
                    label: t("organizations.fields.timezone"),
                    value: org.timezone,
                  },
                  {
                    key: "country",
                    label: t("organizations.fields.country_code"),
                    value: countryName(org.country_code, locale),
                  },
                  {
                    key: "currency",
                    label: t("organizations.fields.currency"),
                    value: org.currency,
                  },
                ],
              },
            ]}
          />
        </EntitySectionCard>
        <EntitySectionCard title={t("commerce.org.general.company_title")}>
          {company ? (
            <EntityDetail
              sections={[
                {
                  id: "company",
                  fields: COMPANY_FIELDS.filter(({ key }) => company[key]).map(
                    ({ key, label }) => ({
                      key,
                      label: t(label),
                      value: company[key],
                    }),
                  ),
                },
              ]}
            />
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("commerce.org.general.company_empty")}
            </p>
          )}
        </EntitySectionCard>
      </div>
      <AccessCard org={org} />
      <OrganizationAttributionCard uuid={org.uuid} />
    </div>
  );
}
