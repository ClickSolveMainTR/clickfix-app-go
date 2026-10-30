"use client";

import { Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useCommandPalette } from "@/features/search-engine/providers/command-palette-provider";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export function SearchTrigger({
  className,
  compact = false,
}: {
  className?: string;
  compact?: boolean;
}) {
  const { setOpen } = useCommandPalette();
  const { t } = useLocale();

  if (compact) {
    return (
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={cn("size-9 shrink-0 rounded-lg", className)}
        onClick={() => setOpen(true)}
        aria-label={t("search.trigger_label")}
      >
        <Search className="size-4" />
      </Button>
    );
  }

  return (
    <Button
      type="button"
      variant="outline"
      className={cn(
        "text-muted-foreground bg-card/70 hover:bg-card hover:text-foreground hidden h-9 w-full max-w-md justify-start gap-2 rounded-lg px-3 font-normal shadow-none md:flex dark:bg-white/[0.03]",
        className,
      )}
      onClick={() => setOpen(true)}
    >
      <Search className="size-4 shrink-0 opacity-60" />
      <span className="flex-1 truncate text-left text-sm">
        {t("search.trigger_label")}
      </span>
      <kbd className="bg-muted text-muted-foreground pointer-events-none hidden items-center gap-0.5 rounded-md border px-1.5 py-0.5 font-mono text-[10px] font-medium sm:inline-flex">
        <span className="text-xs leading-none">⌘</span>K
      </kbd>
    </Button>
  );
}
