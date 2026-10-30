import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

type Schemas = components["schemas"];

export type SetupTemplate = Schemas["OnboardingSetupTemplate"];
export type SetupRun = Schemas["OnboardingSetupRun"];
export type SetupItem = Schemas["OnboardingSetupItem"];
export type SetupItemKind = Schemas["OnboardingSetupItemKind"];
export type SetupItemStatus = SetupItem["status"];
export type SetupPlan = Schemas["OnboardingSetupPlan"];
export type SetupAnswers = Schemas["OnboardingSetupAnswers"];
export type SetupDiscoverRequest = Schemas["OnboardingSetupDiscoverRequest"];

/** Payload of `onboarding.setup.progress` on the organization channel. */
export type SetupProgressEvent = {
  run_uuid: string;
  status: SetupRun["status"];
  done: number;
  failed: number;
  total: number;
  item?: SetupItem;
};

const base = "/v1/tenant/onboarding/setup";

/** AI-assisted set-up and the use-case template gallery (CF-140). */
export const setupService = {
  async templates(slug: string) {
    const data = await tenantRequest<{ items: SetupTemplate[] }>(
      slug,
      "GET",
      `${base}/templates`,
      { silent: true },
    );
    return data.items;
  },

  useTemplate(slug: string, key: string) {
    return tenantRequest<SetupRun>(
      slug,
      "POST",
      `${base}/templates/${encodeURIComponent(key)}`,
      { silent: true },
    );
  },

  discover(slug: string, body: SetupDiscoverRequest) {
    return tenantRequest<SetupRun>(slug, "POST", `${base}/discover`, {
      body,
      silent: true,
    });
  },

  async latest(slug: string) {
    const data = await tenantRequest<{ run: SetupRun | null }>(
      slug,
      "GET",
      `${base}/latest`,
      { silent: true },
    );
    return data.run;
  },

  run(slug: string, uuid: string) {
    return tenantRequest<SetupRun>(slug, "GET", `${base}/runs/${uuid}`, {
      silent: true,
    });
  },

  apply(slug: string, uuid: string, selection: string[]) {
    return tenantRequest<SetupRun>(slug, "POST", `${base}/runs/${uuid}/apply`, {
      body: { selection },
      silent: true,
    });
  },

  undo(slug: string, uuid: string) {
    return tenantRequest<SetupRun>(slug, "POST", `${base}/runs/${uuid}/undo`);
  },
};
