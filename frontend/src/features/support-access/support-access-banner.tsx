"use client";

import { LifeBuoy } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { useActiveOrganization } from "@/features/organizations";
import {
  MODE_LABEL_KEYS,
  remaining,
  supportModeOf,
} from "@/features/support-access/support-access";
import { useNow } from "@/features/support-access/use-now";
import { useLocale } from "@/providers/locale-provider";

/**
 * Persistent workspace warning while ClickFix support staff work inside an
 * organization through a support access grant.
 */
export function SupportAccessBanner() {
  const { t } = useLocale();
  const { active } = useActiveOrganization();
  const now = useNow();
  if (!active?.support) return null;
  const mode = supportModeOf(active);
  return (
    <Alert
      className="border-amber-500/40 bg-amber-500/10"
      data-support-access="true"
    >
      <LifeBuoy className="size-4" />
      <AlertTitle>
        {t("support_access.banner.title", { name: active.name })}
      </AlertTitle>
      <AlertDescription>
        {t("support_access.banner.body", {
          mode: t(MODE_LABEL_KEYS[mode]),
          remaining: active.access_ends_at
            ? remaining(t, active.access_ends_at, now)
            : "—",
        })}
      </AlertDescription>
    </Alert>
  );
}
