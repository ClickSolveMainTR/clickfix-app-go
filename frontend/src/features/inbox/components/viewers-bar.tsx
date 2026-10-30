"use client";

import { Eye, PencilLine } from "lucide-react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { initials } from "../lib/format";
import type { Viewer } from "../types";

/**
 * Collision warning (CF-49): who else has this conversation open and
 * whether they are writing a reply right now.
 */
export function ViewersBar({ viewers }: { viewers: Viewer[] }) {
  const { t } = useLocale();
  if (viewers.length === 0) return null;
  const typing = viewers.filter((v) => v.typing);
  const names = (list: Viewer[]) => list.map((v) => v.name || "—").join(", ");
  return (
    <div
      role="status"
      className={cn(
        "flex items-center gap-2 border-t px-4 py-1.5 text-xs",
        typing.length > 0
          ? "bg-amber-50 text-amber-900 dark:bg-amber-950/40 dark:text-amber-100"
          : "text-muted-foreground",
      )}
    >
      <div className="flex -space-x-1.5">
        {viewers.slice(0, 4).map((v) => (
          <span
            key={v.uuid}
            title={v.name}
            className={cn(
              "border-background flex size-5 items-center justify-center rounded-full border-2 text-[8px] font-semibold",
              v.typing ? "bg-amber-400 text-amber-950" : "bg-muted",
            )}
          >
            {initials(v.name)}
          </span>
        ))}
      </div>
      {typing.length > 0 ? (
        <span className="flex items-center gap-1">
          <PencilLine className="size-3.5 animate-pulse" />
          {t("inbox.presence.typing", { names: names(typing) })}
        </span>
      ) : (
        <span className="flex items-center gap-1">
          <Eye className="size-3.5" />
          {t("inbox.presence.viewing", { names: names(viewers) })}
        </span>
      )}
    </div>
  );
}
