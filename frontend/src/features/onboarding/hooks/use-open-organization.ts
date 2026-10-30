"use client";

import { useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import {
  readSignupPlan,
  writeSignupPlan,
  type StoredSignupPlan,
} from "@/features/landing/lib/pricing";
import { useActiveOrganizationStore } from "@/features/organizations";
import type { components } from "@/generated/api";
import { isApiError } from "@/lib/api";
import { platformRequest } from "@/lib/api/platform-request";
import { useAuth } from "@/providers/auth-provider";

import { writeSignupCompany } from "../lib/signup-company";
import { onboardingService } from "../services/onboarding.service";

type BillingPlan = components["schemas"]["BillingPlan"];

export type SelectedPlan = {
  name: string;
  trialDays: number;
  price: BillingPlan["versions"][number]["prices"][number];
};

const ERROR_KEYS: Record<string, string> = {
  SELF_SERVE_DISABLED: "onboarding.create.error_disabled",
  ORGANIZATION_LIMIT: "onboarding.create.error_limit",
  ORGANIZATION_SLUG_TAKEN: "onboarding.create.error_taken",
  VALIDATION_ERROR: "onboarding.create.error_name",
};

/** Message key for a failed `POST /v1/onboarding/organizations`. */
export function openOrganizationErrorKey(err: unknown): string {
  const priceRejected =
    isApiError(err) &&
    err.isValidation &&
    Object.keys(err.fieldErrors()).includes("price_uuid");
  if (priceRejected) return "onboarding.create.error_price";
  const code = isApiError(err) ? err.code : undefined;
  return ERROR_KEYS[code ?? ""] ?? "onboarding.create.error_generic";
}

/** Resolves the remembered price against the public catalog. */
function useSelectedPlan(stored: StoredSignupPlan | null): SelectedPlan | null {
  const catalog = useQuery({
    queryKey: ["public", "pricing"],
    queryFn: () =>
      platformRequest<{ items: BillingPlan[] }>("GET", "/v1/public/pricing", {
        query: { limit: 24 },
        silent: true,
      }),
    enabled: Boolean(stored),
    staleTime: 5 * 60_000,
  });
  if (!stored) return null;
  for (const plan of catalog.data?.items ?? []) {
    for (const version of plan.versions) {
      if (version.status !== "published") continue;
      const price = version.prices.find((p) => p.uuid === stored.price);
      if (price)
        return { name: plan.name, trialDays: version.trial_days ?? 0, price };
    }
  }
  return null;
}

/**
 * Opens the caller's organization (`POST /v1/onboarding/organizations`).
 * A plan picked on the pricing page (CF-42) runs the trial on that plan;
 * "pay now" (or a plan without a trial) means the billing portal's payment
 * step comes next (`payFirst`). The new organization becomes the active one
 * and the remembered plan and company name are cleared. `planFromUrl`
 * (`/register?plan=`) wins over the remembered plan on the first render, before
 * `SignupPlanCapture` has stored it.
 */
export function useOpenOrganization(planFromUrl?: string | null) {
  const { hydrateProfile } = useAuth();
  const setSlug = useActiveOrganizationStore((s) => s.setSlug);
  const [stored, setStored] = useState<StoredSignupPlan | null>(() => {
    if (planFromUrl && /^[0-9a-f-]{36}$/i.test(planFromUrl)) {
      return { price: planFromUrl, payNow: false };
    }
    return typeof window === "undefined" ? null : readSignupPlan();
  });
  const [payNow, setPayNow] = useState(stored?.payNow ?? false);
  const selected = useSelectedPlan(stored);
  const noTrial = selected !== null && selected.trialDays === 0;

  const clearPlan = useCallback(() => {
    writeSignupPlan(null);
    setStored(null);
  }, []);

  const open = useCallback(
    async (name: string) => {
      // With a trial the organization opens trialing on this plan; without
      // one it waits for the first payment (pending_payment).
      const org = await onboardingService.createOrganization(
        name.trim(),
        selected?.price.uuid,
      );
      setSlug(org.slug);
      await hydrateProfile();
      writeSignupPlan(null);
      writeSignupCompany(null);
      return {
        slug: org.slug,
        payFirst: selected !== null && (payNow || noTrial),
      };
    },
    [hydrateProfile, noTrial, payNow, selected, setSlug],
  );

  return { selected, noTrial, payNow, setPayNow, clearPlan, open };
}
