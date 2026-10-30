"use client";

import { useSearchParams } from "next/navigation";
import { useEffect } from "react";

import { writeSignupPlan } from "@/features/landing/lib/pricing";

/**
 * Remembers the plan picked on the pricing page (`/register?plan=`)
 * until the first organization is opened (CF-42). Only a price id is kept,
 * in this browser; the server validates it when the organization is created.
 */
export function SignupPlanCapture() {
  const params = useSearchParams();
  const plan = params.get("plan");
  useEffect(() => {
    if (plan && /^[0-9a-f-]{36}$/i.test(plan)) {
      writeSignupPlan({ price: plan, payNow: false });
    }
  }, [plan]);
  return null;
}
