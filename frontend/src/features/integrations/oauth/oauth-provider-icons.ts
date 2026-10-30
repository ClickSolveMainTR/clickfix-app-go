import type { LucideIcon } from "lucide-react";

import type { OAuthProviderSlug } from "./services/oauth-provider.service";

import { brandNavIcon } from "@/components/brand-logos";

export const oauthProviderIcons: Record<OAuthProviderSlug, LucideIcon> = {
  google: brandNavIcon("google"),
  facebook: brandNavIcon("facebook"),
  apple: brandNavIcon("apple"),
};
