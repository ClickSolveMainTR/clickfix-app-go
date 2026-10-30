import { z } from "zod";

type Translate = (key: string) => string;

const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const TAX_NO_RE = /^\d{10,11}$/;
const COUNT_RE = /^\d*$/;
/** "firma.com", "www.firma.com" or a full http(s) URL. */
const WEBSITE_RE = /^(https?:\/\/)?[^\s/.]+(\.[^\s/.]+)+(\/\S*)?$/i;

/** The API stores websites as http(s) URLs: "firma.com" → "https://firma.com". */
export function normalizeWebsite(value: string) {
  const v = value.trim();
  if (!v) return "";
  return /^https?:\/\//i.test(v) ? v : `https://${v}`;
}
/** Same ceiling the wallet accepts per entry (money.MaxMinor). */
const MAX_OPENING = 10_000_000;

export const WIZARD_STEPS = [
  "company",
  "owner",
  "plan",
  "credits",
  "region",
  "review",
] as const;
export type WizardStep = (typeof WIZARD_STEPS)[number];

export function createOrganizationWizardSchema(t: Translate) {
  const optional = (max: number) =>
    z.string().trim().max(max, t("commerce.wizard.validation.too_long"));
  const count = z
    .string()
    .trim()
    .refine(
      (v) => COUNT_RE.test(v) && Number(v || 0) <= MAX_OPENING,
      t("commerce.wizard.validation.count"),
    );
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, t("organizations.validation.name_required"))
        .max(255, t("organizations.validation.name_too_long")),
      slug: z
        .string()
        .trim()
        .max(64, t("organizations.validation.slug_invalid"))
        .refine(
          (value) => value === "" || SLUG_RE.test(value),
          t("organizations.validation.slug_invalid"),
        ),
      company: z.object({
        legal_name: optional(255),
        tax_office: optional(120),
        tax_number: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || TAX_NO_RE.test(v),
            t("commerce.wizard.validation.tax_number"),
          ),
        address: optional(500),
        city: optional(120),
        phone: optional(40),
        email: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || z.string().email().safeParse(v).success,
            t("organizations.validation.owner_email_invalid"),
          ),
        website: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || WEBSITE_RE.test(v),
            t("commerce.wizard.validation.website"),
          ),
      }),
      owner_mode: z.enum(["existing", "invite"]),
      owner_user_uuid: z.string(),
      owner_email: z.string().trim(),
      plan_mode: z.enum(["default", "choose"]),
      price_uuid: z.string(),
      ai_credits: count,
      video_minutes: count,
      credit_reason: z
        .string()
        .trim()
        .max(500, t("commerce.wizard.validation.too_long")),
      locale: z.enum(["tr", "en"]),
      timezone: z.string().min(1, t("commerce.wizard.validation.timezone")),
      country_code: z
        .string()
        .regex(/^[A-Z]{2}$/, t("commerce.wizard.validation.country")),
    })
    .superRefine((values, ctx) => {
      if (values.owner_mode === "existing" && !values.owner_user_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["owner_user_uuid"],
          message: t("organizations.validation.owner_required"),
        });
      }
      if (
        values.owner_mode === "invite" &&
        !z.string().email().safeParse(values.owner_email).success
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["owner_email"],
          message: t("organizations.validation.owner_email_invalid"),
        });
      }
      if (values.plan_mode === "choose" && !values.price_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["price_uuid"],
          message: t("commerce.wizard.validation.price"),
        });
      }
      const credits = Number(values.ai_credits || 0);
      const minutes = Number(values.video_minutes || 0);
      if ((credits > 0 || minutes > 0) && !values.credit_reason) {
        ctx.addIssue({
          code: "custom",
          path: ["credit_reason"],
          message: t("commerce.wizard.validation.reason"),
        });
      }
    });
}

export type OrganizationWizardValues = z.infer<
  ReturnType<typeof createOrganizationWizardSchema>
>;

export const WIZARD_DEFAULTS: OrganizationWizardValues = {
  name: "",
  slug: "",
  company: {
    legal_name: "",
    tax_office: "",
    tax_number: "",
    address: "",
    city: "",
    phone: "",
    email: "",
    website: "",
  },
  owner_mode: "existing",
  owner_user_uuid: "",
  owner_email: "",
  plan_mode: "default",
  price_uuid: "",
  ai_credits: "",
  video_minutes: "",
  credit_reason: "",
  locale: "tr",
  timezone: "Europe/Istanbul",
  country_code: "TR",
};

/** Fields validated before leaving each step. */
export const STEP_FIELDS: Record<WizardStep, string[]> = {
  company: [
    "name",
    "slug",
    "company.legal_name",
    "company.tax_office",
    "company.tax_number",
    "company.address",
    "company.city",
    "company.phone",
    "company.email",
    "company.website",
  ],
  owner: ["owner_mode", "owner_user_uuid", "owner_email"],
  plan: ["plan_mode", "price_uuid"],
  credits: ["ai_credits", "video_minutes", "credit_reason"],
  region: ["locale", "timezone", "country_code"],
  review: [],
};

/**
 * API error field → wizard field and step. The create endpoint reports
 * `email` for invitation problems and `company.*` / profile fields with the
 * request names.
 */
export function stepOfApiField(
  field: string,
): { step: WizardStep; field: string } | null {
  if (field === "email") return { step: "owner", field: "owner_email" };
  if (field === "trial_price_uuid")
    return { step: "plan", field: "price_uuid" };
  for (const step of WIZARD_STEPS) {
    if (STEP_FIELDS[step].includes(field)) return { step, field };
  }
  return null;
}

/** Human labels of profile fields named in `profile_errors`. */
export const PROFILE_FIELD_KEYS: Record<string, string> = {
  timezone: "organizations.fields.timezone",
  locale: "organizations.fields.locale",
  country_code: "organizations.fields.country_code",
  company: "commerce.org.general.company_title",
  "company.legal_name": "commerce.org.company.legal_name",
  "company.tax_office": "commerce.org.company.tax_office",
  "company.tax_number": "commerce.org.company.tax_number",
  "company.address": "commerce.org.company.address",
  "company.city": "commerce.org.company.city",
  "company.phone": "commerce.org.company.phone",
  "company.email": "commerce.org.company.email",
  "company.website": "commerce.org.company.website",
};
