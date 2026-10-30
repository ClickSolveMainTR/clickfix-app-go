"use client";

import { format, isSameDay, subDays } from "date-fns";
import { createElement } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  COLOR_CLASSES,
  type CalendarViewItem,
} from "@/features/calendar/components/views/types";
import { LAYER_DEFS } from "@/features/calendar/lib/layers";
import type { CalendarFeedItem } from "@/features/calendar/types";
import { cn } from "@/lib/utils";
import { dateFnsLocale } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Read-only summary of a shift or time off shown on the calendar (ported
 * from the otopoly-go calendar item dialog).
 */
export function ItemDialog({
  item,
  view,
  onOpenChange,
}: {
  item: CalendarFeedItem | null;
  view: CalendarViewItem | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  if (!item || !view) return null;
  const def = LAYER_DEFS[item.layer];
  const dfLocale = dateFnsLocale(locale);
  const lastDay = subDays(view.end, 1);
  const when = view.allDay
    ? isSameDay(view.start, lastDay)
      ? `${format(view.start, "PPPP", { locale: dfLocale })} · ${t("calendar.all_day")}`
      : `${format(view.start, "PPP", { locale: dfLocale })} – ${format(lastDay, "PPP", { locale: dfLocale })}`
    : `${format(view.start, "PPPP · HH:mm", { locale: dfLocale })} – ${format(view.end, "HH:mm")}`;

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <span
              aria-hidden
              className={cn(
                "flex size-6 items-center justify-center rounded-md",
                COLOR_CLASSES[view.color].chip,
              )}
            >
              {createElement(def.icon, {
                "aria-hidden": true,
                className: "size-3.5",
              })}
            </span>
            <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              {t(def.labelKey)}
            </span>
          </div>
          <DialogTitle>{view.title}</DialogTitle>
          <DialogDescription className="first-letter:uppercase">
            {when}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
