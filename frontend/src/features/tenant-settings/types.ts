import type { components } from "@/generated/api";

type Schemas = components["schemas"];

/** `GET / PATCH /v1/tenant/organization` (name, logo, regional, company, …). */
export type OrganizationProfile = Schemas["OrganizationProfile"];
export type OrganizationProfilePatchRequest =
  Schemas["OrganizationProfilePatchRequest"];
export type OrganizationCompany = Schemas["OrganizationCompany"];
export type OrganizationBusinessHours = Schemas["OrganizationBusinessHours"];
export type OrganizationBusinessDay = Schemas["OrganizationBusinessDay"];
export type OrganizationBranding = Schemas["OrganizationBranding"];
export type WeekDay = OrganizationBusinessDay["day"];
export type OrganizationLocale = OrganizationProfile["locale"];
