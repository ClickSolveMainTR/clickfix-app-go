"use client";

import { ArrowRight } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import {
  AppCombobox,
  AppInput,
  AppSelect,
  AppTextarea,
  FormSection,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  COUNTRIES,
  CURRENCIES,
  LOCALE_LABEL_KEYS,
  ORGANIZATION_LOCALES,
  displayName,
  timeZones,
} from "@/features/tenant-settings/constants";
import type { SettingsFormValues } from "@/features/tenant-settings/schemas/settings-form";
import { useLocale } from "@/providers/locale-provider";

type SectionProps = { disabled?: boolean };

/** Genel: display name (the slug in the URL never changes). */
export function GeneralSection({
  disabled,
  slug,
}: SectionProps & { slug: string }) {
  const { t } = useLocale();
  return (
    <FormSection
      title={t("tenant_settings.sections.general")}
      description={t("tenant_settings.sections.general_desc")}
    >
      <AppInput
        name="name"
        label={t("tenant_settings.profile.name")}
        description={t("tenant_settings.profile.name_hint", { slug })}
        disabled={disabled}
      />
    </FormSection>
  );
}

/** Bölgesel varsayılanlar: time zone, language, currency and country. */
export function RegionalSection({ disabled }: SectionProps) {
  const { t, locale } = useLocale();
  const zoneOptions = useMemo(
    () => timeZones().map((zone) => ({ value: zone, label: zone })),
    [],
  );
  const currencyOptions = useMemo(
    () =>
      CURRENCIES.map((code) => ({
        value: code,
        label: `${code} · ${displayName(locale, "currency", code)}`,
      })),
    [locale],
  );
  const countryOptions = useMemo(
    () =>
      COUNTRIES.map((code) => ({
        value: code,
        label: `${displayName(locale, "region", code)} (${code})`,
      })),
    [locale],
  );
  return (
    <FormSection
      title={t("tenant_settings.sections.regional")}
      description={t("tenant_settings.sections.regional_desc")}
      columns={2}
    >
      <AppCombobox
        name="timezone"
        label={t("tenant_settings.fields.timezone")}
        options={zoneOptions}
        placeholder={t("tenant_settings.fields.timezone_placeholder")}
        searchPlaceholder={t("tenant_settings.fields.timezone_search")}
        disabled={disabled}
      />
      <AppSelect
        name="locale"
        label={t("tenant_settings.fields.locale")}
        options={ORGANIZATION_LOCALES.map((value) => ({
          value,
          label: t(LOCALE_LABEL_KEYS[value]),
        }))}
        disabled={disabled}
      />
      <AppSelect
        name="currency"
        label={t("tenant_settings.fields.currency")}
        placeholder={t("tenant_settings.fields.currency_placeholder")}
        options={currencyOptions}
        disabled={disabled}
      />
      <AppCombobox
        name="country_code"
        label={t("tenant_settings.fields.country_code")}
        options={countryOptions}
        placeholder={t("tenant_settings.fields.country_placeholder")}
        searchPlaceholder={t("tenant_settings.fields.country_search")}
        disabled={disabled}
      />
    </FormSection>
  );
}

/** Şirket bilgileri: legal / tax identity and contact details. */
export function CompanySection({ disabled }: SectionProps) {
  const { t } = useLocale();
  return (
    <FormSection
      title={t("tenant_settings.sections.company")}
      description={t("tenant_settings.sections.company_desc")}
      columns={2}
    >
      <AppInput
        name="company.legal_name"
        label={t("tenant_settings.fields.legal_name")}
        className="sm:col-span-2"
        disabled={disabled}
      />
      <AppInput
        name="company.tax_office"
        label={t("tenant_settings.fields.tax_office")}
        disabled={disabled}
      />
      <AppInput
        name="company.tax_number"
        label={t("tenant_settings.fields.tax_number")}
        description={t("tenant_settings.fields.tax_number_hint")}
        inputMode="numeric"
        maxLength={20}
        disabled={disabled}
      />
      <AppTextarea
        name="company.address"
        label={t("tenant_settings.fields.address")}
        className="sm:col-span-2"
        rows={3}
        disabled={disabled}
      />
      <AppInput
        name="company.city"
        label={t("tenant_settings.fields.city")}
        disabled={disabled}
      />
      <AppInput
        name="company.phone"
        type="tel"
        autoComplete="tel"
        label={t("tenant_settings.fields.phone")}
        disabled={disabled}
      />
      <AppInput
        name="company.email"
        type="email"
        autoComplete="email"
        label={t("tenant_settings.fields.email")}
        disabled={disabled}
      />
      <AppInput
        name="company.website"
        type="url"
        placeholder="https://"
        label={t("tenant_settings.fields.website")}
        disabled={disabled}
      />
    </FormSection>
  );
}

/** Çalışma saatleri live in the SLA schedule (CF-198): one source for the widget, AI and SLA. */
export function BusinessHoursSection() {
  const { t } = useLocale();
  return (
    <FormSection
      title={t("tenant_settings.sections.business_hours")}
      description={t("tenant_settings.sections.business_hours_single_source")}
    >
      <Button asChild variant="outline" className="w-fit">
        <Link href={`${routes.cms.settings.sla}?tab=hours`}>
          {t("tenant_settings.sections.business_hours_open")}
          <ArrowRight />
        </Link>
      </Button>
    </FormSection>
  );
}

/** Marka: primary color of customer-facing pages (`""` = product default). */
export function BrandingSection({ disabled }: SectionProps) {
  const { t } = useLocale();
  const { setValue } = useFormContext<SettingsFormValues>();
  const color = useWatch<SettingsFormValues, "branding.primary_color">({
    name: "branding.primary_color",
  });
  const valid = /^#[0-9A-Fa-f]{6}$/.test(color ?? "");

  return (
    <FormSection
      title={t("tenant_settings.sections.branding")}
      description={t("tenant_settings.sections.branding_desc")}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
        <input
          type="color"
          aria-label={t("tenant_settings.fields.primary_color")}
          value={valid ? color : "#000000"}
          disabled={disabled}
          onChange={(event) =>
            setValue(
              "branding.primary_color",
              event.target.value.toUpperCase(),
              { shouldDirty: true, shouldValidate: true },
            )
          }
          className="border-input mt-6 h-9 w-14 shrink-0 cursor-pointer rounded-md border bg-transparent p-1 disabled:cursor-not-allowed disabled:opacity-50"
        />
        <AppInput
          name="branding.primary_color"
          label={t("tenant_settings.fields.primary_color")}
          description={t("tenant_settings.fields.primary_color_hint")}
          placeholder="#2563EB"
          maxLength={7}
          className="flex-1"
          disabled={disabled}
        />
        {color && !disabled ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="sm:mt-6"
            onClick={() =>
              setValue("branding.primary_color", "", {
                shouldDirty: true,
                shouldValidate: true,
              })
            }
          >
            {t("tenant_settings.fields.primary_color_reset")}
          </Button>
        ) : null}
      </div>
    </FormSection>
  );
}
