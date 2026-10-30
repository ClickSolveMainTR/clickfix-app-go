import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Team = Schemas["Team"];
export type TeamPage = Schemas["TeamPage"];
export type TeamInput = Schemas["TeamInput"];
export type TeamMember = Schemas["TeamMember"];
export type TeamMemberUser = Schemas["TeamMemberUser"];
export type TeamRole = TeamMember["role"];

/** List query of `GET /v1/tenant/teams`. */
export type ListTeamsParams = {
  limit: number;
  offset: number;
  q?: string;
  /** `name` (default) or `created_at`; `-` prefix for descending. */
  sort?: string;
  /** One skill tag. */
  skill?: string;
  /** User uuid: teams of one member. */
  member?: string;
};

export function personName(user: {
  name: string;
  surname: string;
  email: string;
}): string {
  return `${user.name} ${user.surname}`.trim() || user.email;
}
