import type {
  Category,
  CategoryAssignmentPage,
  CategoryAssignmentResult,
  CategoryInput,
  CategoryList,
  CategorySubjectType,
} from "@/features/categories/types";
import { tenantDownloadFile, tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/categories";
const ASSIGNMENTS = "/v1/tenant/category-assignments";

/** Category taxonomy of one organization (`X-Organization`, CF-218). */
export const categoriesService = {
  list(slug: string, includeArchived: boolean) {
    return tenantRequest<CategoryList>(slug, "GET", BASE, {
      query: { include_archived: includeArchived ? "true" : undefined },
    });
  },

  create(slug: string, body: CategoryInput) {
    return tenantRequest<Category>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: CategoryInput) {
    return tenantRequest<Category>(slug, "PATCH", `${BASE}/${uuid}`, { body });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  review(slug: string, params: { limit: number; offset: number }) {
    return tenantRequest<CategoryAssignmentPage>(
      slug,
      "GET",
      `${BASE}/review`,
      {
        query: params,
      },
    );
  },

  /** Confirms or corrects a record's category (stored as a label). */
  label(
    slug: string,
    subjectType: CategorySubjectType,
    subjectUuid: string,
    categoryUuid: string,
  ) {
    return tenantRequest<CategoryAssignmentResult>(
      slug,
      "PUT",
      `${ASSIGNMENTS}/${subjectType}/${subjectUuid}`,
      { body: { category_uuid: categoryUuid } },
    );
  },

  /** The label store as JSON Lines (CF-219 training set). */
  exportLabels(slug: string) {
    return tenantDownloadFile(slug, `${BASE}/labels/export`);
  },
};
