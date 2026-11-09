"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";

import { useOptionalTenant } from "@/features/tenant";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import { mountSupportWidget, supportWidgetInit } from "../lib/support-widget";
import { assistantService } from "../services/assistant.service";

/**
 * ClickFix's own support widget in the workspace (CF-193): the member is
 * identified with a server-side signature. It has no launcher; the
 * assistant drawer opens it for a conversation with the support team.
 * Nothing loads until the platform sets a support organization with an
 * identity-verified widget (and keeps the assistant on).
 */
export const supportWidgetKey = (slug: string) =>
  ["assistant", slug, "support-widget"] as const;

/** The support widget answer of the organization (shared query). */
export function useSupportWidget(slug: string, enabled: boolean) {
  return useQuery({
    queryKey: supportWidgetKey(slug),
    queryFn: () => assistantService.supportWidget(slug),
    enabled: Boolean(slug) && enabled,
    staleTime: Infinity,
    retry: false,
  });
}

export function SupportWidget() {
  const slug = useOptionalTenant()?.slug ?? "";
  const { user } = useAuth();
  const { locale } = useLocale();
  const { data } = useSupportWidget(slug, Boolean(user));
  const init = supportWidgetInit(
    data,
    user ? { name: user.fullName, email: user.email } : null,
    locale,
  );
  const key = init ? JSON.stringify(init) : "";
  useEffect(() => {
    if (!key) return;
    return mountSupportWidget(JSON.parse(key));
  }, [key]);
  return null;
}
