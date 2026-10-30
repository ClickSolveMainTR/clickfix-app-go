import type { Metadata } from "next";

import { SignupPage } from "@/features/auth/components/signup-page";
import { fetchSignupMode } from "@/features/landing/lib/signup";

export const metadata: Metadata = {
  title: "Hesap açın",
  description:
    "ClickFix hesabınızı ve organizasyonunuzu açın ya da ekibinize özel bir demo isteyin.",
};

/**
 * Self-serve sign-up (CF-143). The platform sign-up mode is read on every
 * request so a change in the platform settings shows at once.
 */
export default async function Page() {
  const mode = await fetchSignupMode({ fresh: true });
  return <SignupPage mode={mode} />;
}
