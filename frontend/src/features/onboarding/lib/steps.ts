import {
  BookOpen,
  Bot,
  Building2,
  Plug,
  Workflow,
  type LucideIcon,
} from "lucide-react";

import type { OnboardingStepKey } from "../services/onboarding.service";

/**
 * Checklist steps in the set-up order (CF-198), with their icon (copy:
 * onboarding.steps.*). The API decides the order; this is only the look.
 */
export const ONBOARDING_STEPS: { key: OnboardingStepKey; icon: LucideIcon }[] =
  [
    { key: "customer", icon: Building2 },
    { key: "system", icon: Plug },
    { key: "ai", icon: Bot },
    { key: "knowledge", icon: BookOpen },
    { key: "automation", icon: Workflow },
  ];

export function stepIcon(key: OnboardingStepKey): LucideIcon {
  return ONBOARDING_STEPS.find((s) => s.key === key)?.icon ?? Building2;
}
