import { z } from "zod";

type Translate = (key: string) => string;

const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function createOrganizationEditSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("organizations.validation.name_required"))
      .max(255, t("organizations.validation.name_too_long")),
    slug: z
      .string()
      .trim()
      .min(1, t("organizations.validation.slug_invalid"))
      .max(64, t("organizations.validation.slug_invalid"))
      .regex(SLUG_RE, t("organizations.validation.slug_invalid")),
  });
}

export type OrganizationEditValues = z.infer<
  ReturnType<typeof createOrganizationEditSchema>
>;
