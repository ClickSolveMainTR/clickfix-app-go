import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";

type AppSidebarLogoProps = {
  className?: string;
  /** Second line under the name (workspace / console label). */
  subtitle?: string;
};

/** Sidebar header lockup: mark + name + subtitle, on the ink sidebar card. */
export function AppSidebarLogo({ className, subtitle }: AppSidebarLogoProps) {
  return (
    <>
      <span
        className={cn(
          "flex aspect-square size-8 shrink-0 items-center justify-center",
          className,
        )}
      >
        <AppMark tone="dark" className="size-8" />
      </span>
      <span className="grid flex-1 text-left leading-tight">
        <span className="font-display truncate text-[15px] font-semibold tracking-[-0.03em]">
          {brand.wordmark}
        </span>
        <span className="truncate text-xs opacity-60">
          {subtitle ?? brand.tagline}
        </span>
      </span>
    </>
  );
}
