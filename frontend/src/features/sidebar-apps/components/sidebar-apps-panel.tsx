"use client";

import { ChevronRight } from "lucide-react";
import { useState } from "react";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { usePlacedApps } from "../hooks/use-sidebar-apps";
import { appIcon } from "../lib/protocol";
import type { SidebarAppPlacement } from "../services/sidebar-apps.service";
import { AppFrame } from "./app-frame";

/**
 * Sidebar apps for the conversation sidebar or contact panel. Each app is
 * collapsed until opened, so switching conversations does not mint context
 * tokens for apps nobody looks at. Renders nothing when no app is placed.
 */
export function SidebarAppsPanel({
  placement,
  conversationUuid,
  contactUuid,
}: {
  placement: Exclude<SidebarAppPlacement, "nav">;
  conversationUuid?: string;
  contactUuid?: string;
}) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const canReadContacts = (org?.permissions ?? []).includes(
    Permission.TenantContactsRead,
  );
  const apps = usePlacedApps(placement);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  if (apps.data.length === 0) return null;
  if (placement === "contact_panel" && (!canReadContacts || !contactUuid))
    return null;
  return (
    <section aria-label={t("apps.panel.label")}>
      <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
        {t("apps.panel.title")}
      </h3>
      <ul className="flex flex-col gap-2">
        {apps.data.map((app) => {
          const Icon = appIcon(app.icon);
          const expanded = Boolean(open[app.uuid]);
          // Contact context is passed only when the app may read it; the
          // backend would otherwise reject the whole launch.
          const withContact =
            canReadContacts && app.scopes.includes("contact.read");
          return (
            <li key={app.uuid} className="rounded-md border">
              <button
                type="button"
                aria-expanded={expanded}
                onClick={() =>
                  setOpen((s) => ({ ...s, [app.uuid]: !s[app.uuid] }))
                }
                className="hover:bg-muted flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm"
              >
                <Icon className="text-muted-foreground size-4 shrink-0" />
                <span className="truncate font-medium">{app.name}</span>
                <ChevronRight
                  className={cn(
                    "text-muted-foreground ml-auto size-4 transition-transform",
                    expanded && "rotate-90",
                  )}
                />
              </button>
              {expanded && (
                <div className="p-2 pt-0">
                  <AppFrame
                    app={app}
                    context={{
                      conversation:
                        placement === "conversation_sidebar"
                          ? conversationUuid
                          : undefined,
                      contact: withContact ? contactUuid : undefined,
                    }}
                  />
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
