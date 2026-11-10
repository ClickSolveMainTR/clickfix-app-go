import type { components } from "@/generated/api";

type Schemas = components["schemas"];

/** `GET / PATCH /v1/tenant/organization` (name, logo, regional, company, …). */
export type OrganizationProfile = Schemas["OrganizationProfile"];
export type OrganizationProfilePatchRequest =
  Schemas["OrganizationProfilePatchRequest"];
export type OrganizationCompany = Schemas["OrganizationCompany"];
export type OrganizationBranding = Schemas["OrganizationBranding"];
/** Weekday keys of the team working-hours editor, Monday first. */
export type WeekDay = "mon" | "tue" | "wed" | "thu" | "fri" | "sat" | "sun";
export type OrganizationLocale = OrganizationProfile["locale"];
