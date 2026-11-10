"use client";

import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { AppForm, FormActions } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AutoRepliesSection } from "@/features/tenant-settings/components/auto-replies-section";
import { OrganizationLogoCard } from "@/features/tenant-settings/components/organization-logo-card";
import {
  BrandingSection,
  BusinessHoursSection,
  CompanySection,
  GeneralSection,
  RegionalSection,
} from "@/features/tenant-settings/components/settings-sections";
import {
  useOrganizationProfile,
  useUpdateOrganizationProfile,
} from "@/features/tenant-settings/hooks/use-tenant-settings";
import {
  createSettingsSchema,
  formToPatch,
  profileToForm,
  type SettingsFormValues,
} from "@/features/tenant-settings/schemas/settings-form";
import { LeaveOrganizationCard } from "@/features/tenant/components/leave-organization-card";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Organization settings (`/v1/tenant/organization`): logo, then one form for
 * name, regional defaults, company identity and branding with a save bar,
 * then the automatic customer replies (their own form, CF-198). Working
 * hours live in the SLA schedule. API validation errors (`details[].field`) are shown on
 * the matching fields by `AppForm`.
 */
export function TenantSettingsPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const query = useOrganizationProfile();
  const save = useUpdateOrganizationProfile();
  const schema = useMemo(() => createSettingsSchema(t), [t]);
  const profile = query.data;
  const defaults = useMemo(
    () => (profile ? profileToForm(profile) : undefined),
    [profile],
  );
  const disabled = !canWrite;

  return (
    <EntityPage
      title={t("tenant_settings.title")}
      description={t("tenant_settings.description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tenant_settings.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("tenant_settings.title") },
      ]}
    >
      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError || !profile || !defaults ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("tenant_settings.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <div className="space-y-6">
          <OrganizationLogoCard profile={profile} />
          <AppForm<SettingsFormValues>
            key={profile.updated_at}
            className="space-y-6"
            schema={schema}
            defaultValues={defaults}
            onSubmit={async (values) => {
              await save.mutateAsync(formToPatch(values));
            }}
          >
            {(form) => {
              const { isDirty, isSubmitting } = form.formState;
              return (
                <>
                  <GeneralSection disabled={disabled} slug={profile.slug} />
                  <RegionalSection disabled={disabled} />
                  <CompanySection disabled={disabled} />
                  <BusinessHoursSection />
                  <BrandingSection disabled={disabled} />
                  {canWrite ? (
                    <FormActions className="justify-between">
                      <p className="text-muted-foreground text-sm">
                        {isDirty
                          ? t("tenant_settings.save_bar.dirty")
                          : t("tenant_settings.save_bar.saved", {
                              date: datetime(
                                profile.updated_at,
                                undefined,
                                locale,
                              ),
                            })}
                      </p>
                      <div className="flex gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          disabled={!isDirty || isSubmitting}
                          onClick={() => form.reset()}
                        >
                          {t("tenant_settings.save_bar.discard")}
                        </Button>
                        <Button
                          type="submit"
                          disabled={!isDirty || isSubmitting}
                        >
                          {t("tenant_settings.save_bar.save")}
                        </Button>
                      </div>
                    </FormActions>
                  ) : null}
                </>
              );
            }}
          </AppForm>
          <AutoRepliesSection disabled={disabled} />
          <LeaveOrganizationCard />
        </div>
      )}
    </EntityPage>
  );
}
