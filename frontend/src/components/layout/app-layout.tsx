"use client";

import type { ReactNode } from "react";

import { AppSidebar } from "@/components/layout/app-sidebar";
import { Footer } from "@/components/layout/footer";
import { Header } from "@/components/layout/header";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import {
  CommandPalette,
  CommandPaletteProvider,
  SearchTrigger,
} from "@/features/search-engine";
import { AssistantButton } from "@/features/assistant/components/assistant-button";
import { AssistantProvider } from "@/features/assistant/components/assistant-provider";
import { GuidesProvider } from "@/features/guides/components/guides-provider";
import { PageHelpButton } from "@/features/guides/components/page-help-button";
import { ImpersonationBanner } from "@/features/auth/components/impersonation-banner";
import { SupportAccessBanner } from "@/features/support-access/support-access-banner";
import { NotificationLiveBridge } from "@/features/notifications/components/notification-live-bridge";
import { useWebPushSync } from "@/hooks/use-web-push-sync";

export type AppLayoutVariant = "platform" | "cms";

export function AppLayout({
  children,
  variant = "platform",
}: {
  children: ReactNode;
  variant?: AppLayoutVariant;
}) {
  useWebPushSync(true);

  const shell = (
    <CommandPaletteProvider>
      <SidebarProvider defaultOpen>
        <AppSidebar variant={variant} />
        <SidebarInset
          className={
            "@container/content has-data-[layout=fixed]:h-svh peer-data-[variant=inset]:has-data-[layout=fixed]:h-[calc(100svh-(var(--spacing)*4))]"
          }
        >
          <Header
            fixed
            actions={
              variant === "cms" ? (
                <>
                  <PageHelpButton />
                  <AssistantButton />
                </>
              ) : null
            }
          >
            <div
              className="flex min-w-0 flex-1 items-center gap-2"
              data-guide="header-search"
            >
              <SearchTrigger />
            </div>
          </Header>
          <div className="flex min-w-0 flex-1 flex-col gap-4 px-4 pt-2 pb-6 md:px-6">
            <ImpersonationBanner />
            {variant === "cms" ? <SupportAccessBanner /> : null}
            {children}
          </div>
          <Footer />
        </SidebarInset>
        <NotificationLiveBridge />
        <CommandPalette variant={variant} />
      </SidebarProvider>
    </CommandPaletteProvider>
  );
  if (variant !== "cms") return shell;
  // Workspace shell: module guides and ClickFix Asistan (CF-141).
  return (
    <AssistantProvider>
      <GuidesProvider>{shell}</GuidesProvider>
    </AssistantProvider>
  );
}
