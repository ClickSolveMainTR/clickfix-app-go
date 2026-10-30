import type { ResourceMeta } from "@/features/io/types";
import type {
  ListTeamsParams,
  Team,
  TeamInput,
  TeamPage,
  TeamRole,
} from "@/features/teams/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/teams";

/** Teams of one organization (`X-Organization`). */
export const teamsService = {
  list(slug: string, params: ListTeamsParams) {
    return tenantRequest<TeamPage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Team>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: TeamInput) {
    return tenantRequest<Team>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: TeamInput) {
    return tenantRequest<Team>(slug, "PATCH", `${BASE}/${uuid}`, { body });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  setMember(slug: string, uuid: string, userUuid: string, role: TeamRole) {
    return tenantRequest<Team>(
      slug,
      "PUT",
      `${BASE}/${uuid}/members/${userUuid}`,
      { body: { role } },
    );
  },

  removeMember(slug: string, uuid: string, userUuid: string) {
    return tenantRequest<Team>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/members/${userUuid}`,
    );
  },

  /** Skill tags used in the organization (suggestions). */
  skills(slug: string) {
    return tenantRequest<{ items: string[] }>(slug, "GET", "/v1/tenant/skills");
  },

  /** Replaces a member's skill tags. */
  setMemberSkills(slug: string, userUuid: string, skills: string[]) {
    return tenantRequest<{ skills: string[] }>(
      slug,
      "PUT",
      `/v1/tenant/members/${userUuid}/skills`,
      { body: { skills } },
    );
  },
};
