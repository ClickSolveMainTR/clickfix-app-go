"use client";

import { MonitorSmartphone } from "lucide-react";

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { ConversationDetail } from "../types";

/**
 * "Hangi ekrandan yazdı" (CF-197): the page and tracked events the web
 * widget sent with the latest customer message.
 */
export function ScreenContextChip({
  conversation,
}: {
  conversation: ConversationDetail;
}) {
  const { t, locale } = useLocale();
  const screen = conversation.screen_context;
  if (!screen) return null;
  const label = screen.page_path || screen.last_event;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="bg-muted text-muted-foreground hover:text-foreground flex max-w-56 items-center gap-1 rounded px-1.5 py-0.5 text-xs"
          aria-label={t("inbox.screen.label")}
        >
          <MonitorSmartphone className="size-3 shrink-0" />
          <span className="truncate">{label}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-80 space-y-2 text-xs" align="start">
        <p className="text-sm font-medium">{t("inbox.screen.title")}</p>
        {screen.page_url && (
          <p className="break-all">
            <span className="text-muted-foreground">
              {t("inbox.screen.page")}:{" "}
            </span>
            {screen.page_url}
          </p>
        )}
        {screen.events.length > 0 && (
          <div className="space-y-1">
            <p className="text-muted-foreground">{t("inbox.screen.events")}</p>
            <ul className="space-y-0.5">
              {screen.events.map((e, i) => (
                <li key={i} className="flex gap-2">
                  <code>{e.name}</code>
                  {e.properties && Object.keys(e.properties).length > 0 && (
                    <span className="text-muted-foreground truncate">
                      {JSON.stringify(e.properties)}
                    </span>
                  )}
                </li>
              ))}
            </ul>
          </div>
        )}
        {screen.at && (
          <p className="text-muted-foreground">
            {relativeDatetime(screen.at, locale)}
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}
