import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";

export type Guide = components["schemas"]["Guide"];
export type GuideStep = components["schemas"]["GuideStep"];
export type GuideProgress = components["schemas"]["GuideProgress"];
export type GuideSetupTour = components["schemas"]["GuideSetupTour"];
export type AdminGuide = components["schemas"]["AdminGuide"];
export type AdminGuideInput = components["schemas"]["AdminGuideInput"];
export type AdminGuidePage = components["schemas"]["AdminGuidePage"];
export type GuideGenerateInput = components["schemas"]["GuideGenerateInput"];
export type GuideGenerateResult = components["schemas"]["GuideGenerateResult"];
export type ProductFeedbackInput =
  components["schemas"]["ProductFeedbackInput"];
export type ProductFeedback = components["schemas"]["ProductFeedback"];
export type ProductFeedbackPage = components["schemas"]["ProductFeedbackPage"];

const G = "/v1/tenant/guides";

/**
 * Module guides and product feedback (CF-141). Tenant routes need only
 * organization membership; platform routes `platform.guides.manage` /
 * `platform.product_feedback.read`.
 */
export const guidesService = {
  async preferences(slug: string) {
    const saved = await tenantRequest<{ auto_start: boolean }>(
      slug,
      "GET",
      `${G}/preferences`,
      { silent: true },
    );
    // Preserve an existing browser opt-out while moving the switch to the user account.
    try {
      if (window.localStorage.getItem("clickfix.guides.auto") === "off") {
        await guidesService.setPreferences(slug, false);
        window.localStorage.removeItem("clickfix.guides.auto");
        return { auto_start: false };
      }
    } catch {
      return { auto_start: false };
    }
    return saved;
  },
  setPreferences(slug: string, auto_start: boolean) {
    return tenantRequest<{ auto_start: boolean }>(
      slug,
      "PUT",
      `${G}/preferences`,
      { body: { auto_start } },
    );
  },
  claim(slug: string, key: string) {
    return tenantRequest<{ started: boolean }>(
      slug,
      "POST",
      `${G}/${encodeURIComponent(key)}/auto-start`,
      { silent: true },
    );
  },
  forRoute(slug: string, route: string, locale: string) {
    return tenantRequest<{ items: Guide[] }>(slug, "GET", G, {
      query: { surface: "web", route, locale },
      silent: true,
    });
  },
  setup(slug: string, locale: string) {
    return tenantRequest<GuideSetupTour>(slug, "GET", `${G}/setup`, {
      query: { surface: "web", locale },
      silent: true,
    });
  },
  progress(slug: string, key: string, body: GuideProgress) {
    return tenantRequest<void>(
      slug,
      "PUT",
      `${G}/${encodeURIComponent(key)}/progress`,
      { body, silent: true },
    );
  },
  reset(slug: string) {
    return tenantRequest<void>(slug, "POST", `${G}/reset`);
  },
  async uploadScreenshot(slug: string, file: Blob, name: string) {
    const form = new FormData();
    form.append("file", file, name);
    return tenantFormRequest<{ uuid: string }>(
      slug,
      "POST",
      "/v1/tenant/attachments",
      form,
    );
  },
  sendFeedback(slug: string, body: ProductFeedbackInput) {
    return tenantRequest<{ uuid: string }>(
      slug,
      "POST",
      "/v1/tenant/product-feedback",
      { body },
    );
  },
};

const P = "/v1/platform/guides";

export const platformGuidesService = {
  list(params: {
    surface?: string;
    locale?: string;
    q?: string;
    limit: number;
    offset: number;
  }) {
    return platformRequest<AdminGuidePage>("GET", P, { query: params });
  },
  create(body: AdminGuideInput) {
    return platformRequest<AdminGuide>("POST", P, { body });
  },
  update(uuid: string, body: AdminGuideInput) {
    return platformRequest<AdminGuide>("PUT", `${P}/${uuid}`, { body });
  },
  remove(uuid: string) {
    return platformRequest<void>("DELETE", `${P}/${uuid}`);
  },
  generate(body: GuideGenerateInput) {
    return platformRequest<GuideGenerateResult>("POST", `${P}/generate`, {
      body,
    });
  },
  feedback(params: {
    kind?: string;
    status?: string;
    limit: number;
    offset: number;
  }) {
    return platformRequest<ProductFeedbackPage>(
      "GET",
      "/v1/platform/product-feedback",
      { query: params },
    );
  },
  setFeedbackStatus(uuid: string, status: ProductFeedback["status"]) {
    return platformRequest<void>(
      "PATCH",
      `/v1/platform/product-feedback/${uuid}`,
      { body: { status } },
    );
  },
};
