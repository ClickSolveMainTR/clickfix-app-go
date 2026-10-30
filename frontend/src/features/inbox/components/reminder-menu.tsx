"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, X } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useLocale } from "@/providers/locale-provider";

import { inboxKeys, useInboxScope } from "../hooks/use-inbox";
import { shortUntil } from "../lib/format";
import { inboxService } from "../services/inbox.service";

const PRESETS = [
  { key: "in_hour", hours: 1, condition: "always" },
  { key: "tomorrow", hours: 0, condition: "always" },
  { key: "no_reply_3d", hours: 72, condition: "no_reply" },
] as const;

function dueOf(p: (typeof PRESETS)[number]): Date {
  if (p.key === "tomorrow") {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    d.setHours(9, 0, 0, 0);
    return d;
  }
  return new Date(Date.now() + p.hours * 3_600_000);
}

/** Follow-up reminders on a conversation (CF-51). */
export function ReminderMenu({ conversation }: { conversation: string }) {
  const { t, locale } = useLocale();
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const key = [...inboxKeys.all(org), "reminders", conversation];
  const list = useQuery({
    queryKey: key,
    queryFn: () => inboxService.reminders(conversation),
    enabled: Boolean(org),
  });
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const pending = list.data ?? [];
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5"
          aria-label={t("inbox.reminders.title")}
        >
          <BellRing className="size-3.5" />
          {pending.length > 0 && (
            <span className="tabular-nums">{pending.length}</span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-0" align="end">
        <Command>
          <CommandList>
            <CommandGroup heading={t("inbox.reminders.title")}>
              {PRESETS.map((p) => (
                <CommandItem
                  key={p.key}
                  onSelect={() =>
                    void inboxService
                      .addReminder(conversation, {
                        due_at: dueOf(p).toISOString(),
                        condition: p.condition,
                      })
                      .then(() => {
                        refresh();
                        toast.success(t("inbox.reminders.set"));
                      })
                  }
                >
                  {t(`inbox.reminders.presets.${p.key}`)}
                </CommandItem>
              ))}
            </CommandGroup>
            {pending.length > 0 && (
              <>
                <CommandSeparator />
                <CommandGroup heading={t("inbox.reminders.pending")}>
                  {pending.map((r) => (
                    <CommandItem
                      key={r.uuid}
                      value={r.uuid}
                      onSelect={() => undefined}
                    >
                      <span className="text-xs">
                        {shortUntil(r.due_at, locale)}
                        {r.condition === "no_reply"
                          ? ` · ${t("inbox.reminders.if_no_reply")}`
                          : ""}
                      </span>
                      <button
                        type="button"
                        aria-label={t("inbox.reminders.cancel")}
                        className="text-muted-foreground hover:text-destructive ml-auto"
                        onClick={(e) => {
                          e.stopPropagation();
                          void inboxService
                            .cancelReminder(r.uuid)
                            .then(refresh);
                        }}
                      >
                        <X className="size-3.5" />
                      </button>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
