"use client";

import { Building2, Link2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import {
  AppForm,
  AppInput,
  FormActions,
  FormSection,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useOrganization,
  useUpdateOrganization,
} from "@/features/platform-organizations/hooks/use-organizations";
import {
  createOrganizationEditSchema,
  type OrganizationEditValues,
} from "@/features/platform-organizations/schemas/organization-form";
import { useLocale } from "@/providers/locale-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("organizations.forbidden")}
    />
  );
}

/**
 * `/platform/organizations/{uuid}/edit`: name and slug. Creation is the
 * wizard in `wizard/organization-create-wizard.tsx`.
 */
export function OrganizationEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const query = useOrganization(uuid);
  const update = useUpdateOrganization();
  const schema = useMemo(() => createOrganizationEditSchema(t), [t]);
  const org = query.data;

  return (
    <EntityPage
      title={t("organizations.edit_title")}
      description={t("organizations.edit_description")}
      permission={permissions.organizations.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        ...(org
          ? [
              {
                label: org.name,
                href: routes.platform.organizations.detail(uuid),
              },
            ]
          : []),
        { label: t("organizations.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("organizations.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {org ? (
        <EntityForm>
          <AppForm<OrganizationEditValues>
            key={org.updated_at}
            schema={schema}
            defaultValues={{ name: org.name, slug: org.slug }}
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: { name: values.name.trim(), slug: values.slug },
              });
              router.push(routes.platform.organizations.detail(uuid));
            }}
          >
            {(form) => (
              <FormSection
                title={t("organizations.form.section_identity")}
                description={t("organizations.form.slug_change_hint")}
                columns={2}
              >
                <AppInput
                  name="name"
                  label={t("organizations.fields.name")}
                  startIcon={Building2}
                />
                <AppInput
                  name="slug"
                  label={t("organizations.fields.slug")}
                  startIcon={Link2}
                />
                <FormActions className="sm:col-span-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() =>
                      router.push(routes.platform.organizations.detail(uuid))
                    }
                    disabled={form.formState.isSubmitting}
                  >
                    {t("form.cancel")}
                  </Button>
                  <Button type="submit" disabled={form.formState.isSubmitting}>
                    {t("organizations.actions.save")}
                  </Button>
                </FormActions>
              </FormSection>
            )}
          </AppForm>
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}
