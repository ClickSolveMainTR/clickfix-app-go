import type { ResourceMeta } from "@/features/io/types";
import type {
  ConversationDepartment,
  Department,
  DepartmentInput,
  DepartmentPage,
  DepartmentShare,
  DepartmentShareInput,
  DepartmentTransferInput,
  ListDepartmentsParams,
} from "@/features/departments/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/departments";

/** Departments of one organization (`X-Organization`, CF-215). */
export const departmentsService = {
  list(slug: string, params: ListDepartmentsParams) {
    const { mine, ...rest } = params;
    return tenantRequest<DepartmentPage>(slug, "GET", BASE, {
      query: { ...rest, ...(mine ? { mine: "true" } : {}) },
    });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Department>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: DepartmentInput) {
    return tenantRequest<Department>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: DepartmentInput) {
    return tenantRequest<Department>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },

  /** `moveTo`: `none` (organization-wide) or a department uuid for the knowledge it owns. */
  remove(slug: string, uuid: string, moveTo?: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${BASE}/${uuid}`,
      moveTo ? { query: { move_to: moveTo } } : undefined,
    );
  },

  shares(slug: string, uuid: string) {
    return tenantRequest<{ items: DepartmentShare[] }>(
      slug,
      "GET",
      `${BASE}/${uuid}/shares`,
    );
  },

  /** Shares one item with the department `target`. */
  share(slug: string, target: string, body: DepartmentShareInput) {
    return tenantRequest<DepartmentShare>(
      slug,
      "POST",
      `${BASE}/${target}/shares`,
      { body },
    );
  },

  unshare(slug: string, department: string, share: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${BASE}/${department}/shares/${share}`,
    );
  },

  conversation(slug: string, conversation: string) {
    return tenantRequest<ConversationDepartment>(
      slug,
      "GET",
      `/v1/tenant/conversations/${conversation}/department`,
    );
  },

  transfer(slug: string, conversation: string, body: DepartmentTransferInput) {
    return tenantRequest<ConversationDepartment>(
      slug,
      "POST",
      `/v1/tenant/conversations/${conversation}/department-transfer`,
      { body },
    );
  },
};
