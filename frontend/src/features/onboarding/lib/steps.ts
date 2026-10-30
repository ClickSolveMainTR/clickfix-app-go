import {
  Building2,
  Code2,
  MessagesSquare,
  Plug,
  UsersRound,
  type LucideIcon,
} from "lucide-react";

import type { OnboardingStepKey } from "../services/onboarding.service";

/** Checklist steps in order, with their icon (copy: onboarding.steps.*). */
export const ONBOARDING_STEPS: { key: OnboardingStepKey; icon: LucideIcon }[] =
  [
    { key: "organization", icon: Building2 },
    { key: "channel", icon: Plug },
    { key: "team", icon: UsersRound },
    { key: "widget", icon: Code2 },
    { key: "first_conversation", icon: MessagesSquare },
  ];

export function stepIcon(key: OnboardingStepKey): LucideIcon {
  return ONBOARDING_STEPS.find((s) => s.key === key)?.icon ?? Building2;
}
