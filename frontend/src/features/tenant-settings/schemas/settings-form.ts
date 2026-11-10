import { z } from "zod";

import { WEEK_DAYS } from "@/features/tenant-settings/constants";
import {
  isValidTCKN,
  isValidVKN,
} from "@/features/tenant-settings/lib/tax-number";
import type {
  OrganizationProfile,
  OrganizationProfilePatchRequest,
} from "@/features/tenant-settings/types";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d$/;
const COLOR_RE = /^#[0-9A-Fa-f]{6}$/;
const PHONE_RE = /^\+?[0-9 ()-]{7,20}$/;

function text(t: Translate, max: number) {
  return z
    .string()
    .trim()
    .max(max, t("tenant_settings.validation.too_long", { max }));
}

/** One `{day, open, start, end}` row of a week (business / working hours). */
export function createWeekDaySchema(t: Translate) {
  return z
    .object({
      day: z.enum(["mon", "tue", "wed", "thu", "fri", "sat", "sun"]),
      open: z.boolean(),
      start: z.string().regex(TIME_RE, t("tenant_settings.validation.time")),
      end: z.string().regex(TIME_RE, t("tenant_settings.validation.time")),
    })
    .superRefine((value, ctx) => {
      if (value.open && value.end <= value.start) {
        ctx.addIssue({
          code: "custom",
          path: ["end"],
          message: t("tenant_settings.validation.end_after_start"),
        });
      }
    });
}

/** Seven rows, Monday first (`WEEK_DAYS`) when the caller does not care. */
export function defaultWeekDays() {
  return WEEK_DAYS.map((day) => ({
    day,
    open: day !== "sat" && day !== "sun",
    start: "09:00",
    end: "18:00",
  }));
}

/**
 * Client checks mirror the API (`PATCH /v1/tenant/organization`); the API
 * stays the authority and its `details[].field` errors (e.g.
 * `business_hours.days.2.end`) land on the same form paths.
 */
export function createSettingsSchema(t: Translate) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, t("tenant_settings.profile.validation.name_required"))
        .max(255, t("tenant_settings.profile.validation.name_too_long")),
      timezone: z.string().min(1, t("tenant_settings.validation.required")),
      locale: z.enum(["tr", "en"]),
      currency: z
        .string()
        .regex(/^[A-Z]{3}$/, t("tenant_settings.validation.required")),
      country_code: z
        .string()
        .regex(/^[A-Z]{2}$/, t("tenant_settings.validation.required")),
      company: z.object({
        legal_name: text(t, 255),
        tax_office: text(t, 100),
        tax_number: text(t, 20),
        address: text(t, 500),
        city: text(t, 100),
        phone: text(t, 32).refine(
          (value) => value === "" || PHONE_RE.test(value),
          t("tenant_settings.validation.phone"),
        ),
        email: text(t, 255).refine(
          (value) =>
            value === "" || z.string().email().safeParse(value).success,
          t("tenant_settings.validation.email"),
        ),
        website: text(t, 255).refine(
          (value) => value === "" || z.string().url().safeParse(value).success,
          t("tenant_settings.validation.website"),
        ),
      }),
      branding: z.object({
        primary_color: z
          .string()
          .trim()
          .refine(
            (value) => value === "" || COLOR_RE.test(value),
            t("tenant_settings.validation.color"),
          ),
      }),
    })
    .superRefine((values, ctx) => {
      // VKN / TCKN checksums only apply to Turkish organizations.
      const taxNumber = values.company.tax_number.replace(/\s/g, "");
      if (
        values.country_code === "TR" &&
        taxNumber !== "" &&
        !isValidVKN(taxNumber) &&
        !isValidTCKN(taxNumber)
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["company", "tax_number"],
          message: t("tenant_settings.validation.tax_number"),
        });
      }
    });
}

export type SettingsFormValues = z.infer<
  ReturnType<typeof createSettingsSchema>
>;

/** API profile → form values. Working hours live in the SLA schedule (CF-198). */
export function profileToForm(
  profile: OrganizationProfile,
): SettingsFormValues {
  return {
    name: profile.name,
    timezone: profile.timezone,
    locale: profile.locale,
    currency: profile.currency,
    country_code: profile.country_code,
    company: { ...profile.company },
    branding: { primary_color: profile.branding.primary_color ?? "" },
  };
}

/** Form values → `PATCH` body (every section; objects replace whole). */
export function formToPatch(
  values: SettingsFormValues,
): OrganizationProfilePatchRequest {
  const trimmed = Object.fromEntries(
    Object.entries(values.company).map(([key, value]) => [key, value.trim()]),
  ) as SettingsFormValues["company"];
  return {
    name: values.name.trim(),
    timezone: values.timezone,
    locale: values.locale,
    currency: values.currency,
    country_code: values.country_code,
    company: {
      ...trimmed,
      tax_number: trimmed.tax_number.replace(/\s/g, ""),
    },
    branding: {
      primary_color: values.branding.primary_color.trim().toUpperCase(),
    },
  };
}
