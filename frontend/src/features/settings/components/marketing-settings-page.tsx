"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { BarChart3, Code2, ImageIcon, Search, ShieldCheck } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  AppForm,
  AppInput,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { FormFieldShell } from "@/components/forms/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiConfig } from "@/config/api";
import { permissions } from "@/config/permissions";
import { SettingsPageHeader } from "@/features/settings/components/settings-page-header";
import {
  marketingService,
  type MarketingConfig,
  type MarketingSettings,
} from "@/features/settings/services/marketing.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const queryKey = ["platform", "marketing"] as const;

const TAG_FIELDS = [
  { name: "gtm_id", placeholder: "GTM-XXXXXXX" },
  { name: "ga4_id", placeholder: "G-XXXXXXXXXX" },
  { name: "google_ads_id", placeholder: "AW-123456789" },
  { name: "google_ads_signup_label", placeholder: "AbCdEfGhIjK" },
  { name: "meta_pixel_id", placeholder: "123456789012345" },
  { name: "linkedin_partner_id", placeholder: "1234567" },
  { name: "tiktok_pixel_id", placeholder: "C1A2B3C4D5E6F7G8H9I0" },
  { name: "clarity_id", placeholder: "abcd1234ef" },
  { name: "hotjar_id", placeholder: "1234567" },
] as const;

const VERIFICATION_FIELDS = [
  "google_verification",
  "bing_verification",
  "yandex_verification",
] as const;

const schema = z.object({
  gtm_id: z.string(),
  ga4_id: z.string(),
  meta_pixel_id: z.string(),
  linkedin_partner_id: z.string(),
  tiktok_pixel_id: z.string(),
  google_ads_id: z.string(),
  google_ads_signup_label: z.string(),
  clarity_id: z.string(),
  hotjar_id: z.string(),
  custom_head: z.string().max(20 * 1024),
  custom_body: z.string().max(20 * 1024),
  site_title: z.string().max(70),
  site_description: z.string().max(200),
  google_verification: z.string(),
  bing_verification: z.string(),
  yandex_verification: z.string(),
});

function toValues(settings: MarketingSettings): MarketingConfig {
  return {
    gtm_id: settings.gtm_id ?? "",
    ga4_id: settings.ga4_id ?? "",
    meta_pixel_id: settings.meta_pixel_id ?? "",
    linkedin_partner_id: settings.linkedin_partner_id ?? "",
    tiktok_pixel_id: settings.tiktok_pixel_id ?? "",
    google_ads_id: settings.google_ads_id ?? "",
    google_ads_signup_label: settings.google_ads_signup_label ?? "",
    clarity_id: settings.clarity_id ?? "",
    hotjar_id: settings.hotjar_id ?? "",
    custom_head: settings.custom_head ?? "",
    custom_body: settings.custom_body ?? "",
    site_title: settings.site_title ?? "",
    site_description: settings.site_description ?? "",
    google_verification: settings.google_verification ?? "",
    bing_verification: settings.bing_verification ?? "",
    yandex_verification: settings.yandex_verification ?? "",
  };
}

/**
 * "Pazarlama & izleme" (CF-144): tag ids, verification tokens, SEO defaults,
 * custom snippets and the default share image of the marketing site. The
 * landing reads them through `GET /v1/public/marketing`; a save revalidates
 * its cache, so no redeploy is needed.
 */
export function MarketingSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.settings.read);
  const canWrite = can(permissions.settings.write);
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey,
    queryFn: () => marketingService.get(),
    enabled: canRead,
  });

  const onSaved = (data: MarketingSettings, message: string) => {
    queryClient.setQueryData(queryKey, data);
    appToast.success(message);
  };

  const save = useAppMutation({
    mutationFn: (body: MarketingConfig) => marketingService.save(body),
    onSuccess: (data) => onSaved(data, t("settings.marketing.saved")),
  });
  const upload = useAppMutation({
    mutationFn: (file: File) => marketingService.uploadOGImage(file),
    onSuccess: (data) => onSaved(data, t("settings.marketing.og_saved")),
  });
  const remove = useAppMutation({
    mutationFn: () => marketingService.deleteOGImage(),
    onSuccess: (data) => onSaved(data, t("settings.marketing.og_removed")),
  });

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "tags",
          label: t("settings.marketing.section_tags"),
          icon: BarChart3,
        },
        {
          key: "seo",
          label: t("settings.marketing.section_seo"),
          icon: Search,
        },
        {
          key: "verification",
          label: t("settings.marketing.section_verification"),
          icon: ShieldCheck,
        },
        {
          key: "og",
          label: t("settings.marketing.section_og"),
          icon: ImageIcon,
        },
        {
          key: "snippets",
          label: t("settings.marketing.section_snippets"),
          icon: Code2,
        },
      ]),
    [t],
  );

  if (!canRead) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("settings.forbidden")}
      />
    );
  }

  const settings = query.data;
  const ogSrc = settings?.og_image_url
    ? `${apiConfig.baseUrl.replace(/\/$/, "")}${settings.og_image_url}`
    : null;

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.subnav.marketing")}
        description={t("settings.marketing.description")}
      />
      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError || !settings ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <AppForm
          key={settings.updated_at}
          schema={schema}
          defaultValues={toValues(settings)}
          onSubmit={async (values) => {
            await save.mutateAsync(values);
          }}
        >
          {(form) => (
            <FormLayout navItems={sections.navItems}>
              <FormSection
                id={sections.id("tags")}
                title={t("settings.marketing.section_tags")}
                description={t("settings.marketing.section_tags_hint")}
                columns={2}
              >
                {TAG_FIELDS.map((field) => (
                  <AppInput
                    key={field.name}
                    name={field.name}
                    label={t(`settings.marketing.${field.name}`)}
                    placeholder={field.placeholder}
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono"
                    disabled={!canWrite}
                  />
                ))}
              </FormSection>

              <FormSection
                id={sections.id("seo")}
                title={t("settings.marketing.section_seo")}
                description={t("settings.marketing.section_seo_hint")}
              >
                <AppInput
                  name="site_title"
                  label={t("settings.marketing.site_title")}
                  maxLength={70}
                  disabled={!canWrite}
                />
                <AppTextarea
                  name="site_description"
                  label={t("settings.marketing.site_description")}
                  maxLength={200}
                  rows={3}
                  disabled={!canWrite}
                />
              </FormSection>

              <FormSection
                id={sections.id("verification")}
                title={t("settings.marketing.section_verification")}
                description={t("settings.marketing.section_verification_hint")}
                columns={2}
              >
                {VERIFICATION_FIELDS.map((name) => (
                  <AppInput
                    key={name}
                    name={name}
                    label={t(`settings.marketing.${name}`)}
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono"
                    disabled={!canWrite}
                  />
                ))}
              </FormSection>

              <FormSection
                id={sections.id("og")}
                title={t("settings.marketing.section_og")}
                description={t("settings.marketing.section_og_hint")}
              >
                {ogSrc ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={ogSrc}
                    alt={t("settings.marketing.og_alt")}
                    width={1200}
                    height={630}
                    className="h-auto w-full max-w-md rounded-lg border"
                  />
                ) : (
                  <p className="text-muted-foreground text-sm">
                    {t("settings.marketing.og_none")}
                  </p>
                )}
                {canWrite ? (
                  <div className="flex flex-wrap items-end gap-2">
                    <FormFieldShell
                      name="og_image"
                      label={t("settings.marketing.og_upload")}
                    >
                      <Input
                        id="og_image"
                        type="file"
                        accept="image/png,image/jpeg,image/webp"
                        className="max-w-md"
                        disabled={upload.isPending}
                        onChange={(event) => {
                          const file = event.target.files?.[0];
                          if (file) upload.mutate(file);
                          event.target.value = "";
                        }}
                      />
                    </FormFieldShell>
                    {ogSrc ? (
                      <Button
                        type="button"
                        variant="outline"
                        disabled={remove.isPending}
                        onClick={() => remove.mutate()}
                      >
                        {t("settings.marketing.og_remove")}
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </FormSection>

              <FormSection
                id={sections.id("snippets")}
                title={t("settings.marketing.section_snippets")}
                description={t("settings.marketing.section_snippets_hint")}
              >
                {(["custom_head", "custom_body"] as const).map((name) => (
                  <AppTextarea
                    key={name}
                    name={name}
                    label={t(`settings.marketing.${name}`)}
                    rows={6}
                    spellCheck={false}
                    className="font-mono text-xs"
                    disabled={!canWrite}
                  />
                ))}
              </FormSection>

              {canWrite ? (
                <FormActions>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => form.reset()}
                  >
                    {t("common.cancel")}
                  </Button>
                  <Button type="submit" disabled={save.isPending}>
                    {save.isPending ? t("common.loading") : t("settings.save")}
                  </Button>
                </FormActions>
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("settings.read_only")}
                </p>
              )}
            </FormLayout>
          )}
        </AppForm>
      )}
    </div>
  );
}
