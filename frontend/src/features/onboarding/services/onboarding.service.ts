import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type OnboardingState = components["schemas"]["OnboardingState"];
export type OnboardingStep = components["schemas"]["OnboardingStep"];
export type OnboardingStepKey = OnboardingStep["key"];
export type ChannelType = components["schemas"]["ChannelType"];
export type Organization = components["schemas"]["Organization"];

const org = (slug: string) => ({ header: { "X-Organization": slug } });

/** Self-serve sign-up and the set-up checklist (CF-13). */
export const onboardingService = {
  /** `priceUuid` (CF-42) runs the signup trial on the plan picked on the pricing page. */
  async createOrganization(name: string, priceUuid?: string) {
    const data = await unwrap<{ organization: Organization }>(
      await apiClient.POST("/v1/onboarding/organizations", {
        body: priceUuid ? { name, price_uuid: priceUuid } : { name },
      }),
      { silent: true },
    );
    return data.organization;
  },

  async getState(slug: string) {
    return unwrap<OnboardingState>(
      await apiClient.GET("/v1/tenant/onboarding", { params: org(slug) }),
      { silent: true },
    );
  },

  async completeStep(slug: string, step: "widget" | "team") {
    return unwrap<OnboardingState>(
      await apiClient.POST("/v1/tenant/onboarding/steps/{step}", {
        params: { ...org(slug), path: { step } },
      }),
    );
  },

  async finish(slug: string) {
    return unwrap<OnboardingState>(
      await apiClient.POST("/v1/tenant/onboarding/finish", {
        params: org(slug),
      }),
    );
  },

  async channelTypes(slug: string) {
    const data = await unwrap<{ items: ChannelType[] }>(
      await apiClient.GET("/v1/tenant/channels/types", { params: org(slug) }),
      { silent: true },
    );
    return data.items;
  },

  async connectChannel(
    slug: string,
    body: components["schemas"]["ChannelInstanceCreate"],
  ) {
    return unwrap<components["schemas"]["ChannelInstance"]>(
      await apiClient.POST("/v1/tenant/channels", {
        params: org(slug),
        body,
      }),
      { silent: true },
    );
  },
};
