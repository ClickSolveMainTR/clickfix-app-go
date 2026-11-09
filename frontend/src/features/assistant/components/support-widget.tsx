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
 * identified with a server-side signature and gets the assistant tab
 * (widget surface: navigation, help, reads, small confirmed set-ups) next
 * to the support team's chat. Nothing loads until the platform sets a
 * support organization with an identity-verified widget.
 */
export function SupportWidget() {
  const slug = useOptionalTenant()?.slug ?? "";
  const { user } = useAuth();
  const { locale } = useLocale();
  const { data } = useQuery({
    queryKey: ["assistant", slug, "support-widget"],
    queryFn: () => assistantService.supportWidget(slug),
    enabled: Boolean(slug && user),
    staleTime: Infinity,
    retry: false,
  });
  const init = supportWidgetInit(
    data,
    user ? { name: user.fullName, email: user.email } : null,
    slug,
    locale,
  );
  const key = init ? JSON.stringify(init) : "";
  useEffect(() => {
    if (!key) return;
    return mountSupportWidget(JSON.parse(key));
  }, [key]);
  return null;
}
