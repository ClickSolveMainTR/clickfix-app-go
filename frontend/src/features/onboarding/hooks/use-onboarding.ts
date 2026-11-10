"use client";

import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";

import { useActiveOrganization } from "@/features/organizations";

import {
  onboardingService,
  type OnboardingState,
  type OnboardingStepKey,
} from "../services/onboarding.service";

export const onboardingKeys = {
  state: (slug: string) => ["onboarding", slug] as const,
  channelTypes: (slug: string) =>
    ["onboarding", slug, "channel-types"] as const,
};

function store(qc: QueryClient, slug: string, state: OnboardingState) {
  qc.setQueryData(onboardingKeys.state(slug), state);
}

/** The active organization's checklist (null without an organization). */
export function useOnboarding() {
  const { active } = useActiveOrganization();
  const slug = active?.slug ?? "";
  const canRead = Boolean(
    active?.permissions.includes("tenant.settings.read") ||
    active?.role.key === "owner",
  );
  const query = useQuery({
    queryKey: onboardingKeys.state(slug),
    queryFn: () => onboardingService.getState(slug),
    enabled: Boolean(slug) && canRead,
    staleTime: 15_000,
    retry: false,
  });
  return { slug, organization: active, ...query };
}

export function useOnboardingActions(slug: string) {
  const qc = useQueryClient();
  const skipStep = useMutation({
    mutationFn: (step: OnboardingStepKey) =>
      onboardingService.skipStep(slug, step),
    onSuccess: (state) => store(qc, slug, state),
  });
  const finish = useMutation({
    mutationFn: () => onboardingService.finish(slug),
    onSuccess: (state) => store(qc, slug, state),
  });
  const refresh = () =>
    qc.invalidateQueries({ queryKey: onboardingKeys.state(slug) });
  return { skipStep, finish, refresh };
}

export function useChannelTypes(slug: string, enabled = true) {
  return useQuery({
    queryKey: onboardingKeys.channelTypes(slug),
    queryFn: () => onboardingService.channelTypes(slug),
    enabled: Boolean(slug) && enabled,
    staleTime: 5 * 60_000,
    retry: false,
  });
}
