"use client";

import {
  useEffect,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { UserMenu } from "@/components/layout/user-menu";
import { SearchTrigger } from "@/features/search-engine";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { NotificationInbox } from "@/features/notifications";
import { cn } from "@/lib/utils";

type HeaderProps = HTMLAttributes<HTMLElement> & {
  fixed?: boolean;
  children?: ReactNode;
  /** Workspace-only buttons (page help, assistant) before notifications. */
  actions?: ReactNode;
};

/**
 * Top bar: transparent at the top of the page, a rounded frosted strip with
 * a hairline border once the page scrolls. Holds the sidebar toggle, the
 * Cmd+K search, notifications, language, theme and the user menu.
 */
export function Header({
  className,
  fixed = true,
  children,
  actions,
  ...props
}: HeaderProps) {
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => {
      const offset =
        document.body.scrollTop || document.documentElement.scrollTop;
      setScrolled(offset > 8);
    };
    onScroll();
    document.addEventListener("scroll", onScroll, { passive: true });
    return () => document.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <header
      className={cn(
        "z-50 h-16 px-2 pt-2",
        fixed && "header-fixed peer/header sticky top-0 w-[inherit]",
        className,
      )}
      {...props}
    >
      <div
        className={cn(
          "relative flex h-full items-center gap-2 rounded-xl border border-transparent px-2 transition-[background-color,border-color,box-shadow] duration-200 sm:gap-3",
          scrolled &&
            fixed &&
            "bg-background/75 border-border/70 shadow-sm shadow-black/[0.04] backdrop-blur-xl backdrop-saturate-150",
        )}
      >
        <SidebarTrigger
          variant="ghost"
          className="size-9 rounded-lg [&_svg:not([class*='size-'])]:size-[1.1rem]"
        />
        <Separator orientation="vertical" className="h-5!" />
        <div className="flex min-w-0 flex-1 items-center">{children}</div>
        <div className="ms-auto flex items-center gap-0.5">
          <SearchTrigger compact className="md:hidden" />
          {actions}
          <span data-guide="header-notifications" className="inline-flex">
            <NotificationInbox />
          </span>
          <LocaleSwitch />
          <ThemeSwitch />
          <Separator
            orientation="vertical"
            className="mx-1 h-5! max-sm:hidden"
          />
          <span data-guide="user-menu" className="inline-flex">
            <UserMenu />
          </span>
        </div>
      </div>
    </header>
  );
}
