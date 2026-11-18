"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  AlarmClock,
  CheckCircle2,
  Flag,
  Hash,
  Mail,
  MailOpen,
  UserRoundCheck,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useLocale } from "@/providers/locale-provider";

import { inboxKeys, useInboxScope } from "../hooks/use-inbox";
import { PRIORITY_ORDER } from "../lib/format";
import { inboxService } from "../services/inbox.service";
import type { BulkAction } from "../types";

function snoozeAt(key: "tomorrow" | "next_week"): string {
  const d = new Date();
  d.setDate(d.getDate() + (key === "tomorrow" ? 1 : (8 - d.getDay()) % 7 || 7));
  d.setHours(9, 0, 0, 0);
  return d.toISOString();
}

/**
 * Actions on the selected conversations (CF-51). Every run can be undone
 * from the toast for 24 hours.
 */
export function BulkBar({
  selected,
  onClear,
}: {
  selected: string[];
  onClear: () => void;
}) {
  const { t } = useLocale();
  const { org, canManage, canAssign } = useInboxScope();
  const qc = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [tag, setTag] = useState("");

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: inboxKeys.all(org) });
  };

  const run = async (action: BulkAction, value = "") => {
    setBusy(true);
    try {
      const res = await inboxService.bulk(selected, action, value);
      refresh();
      onClear();
      toast.success(t("inbox.bulk.done", { count: res.applied }), {
        description:
          res.failed > 0
            ? t("inbox.bulk.failed", { count: res.failed })
            : undefined,
        action: {
          label: t("inbox.bulk.undo"),
          onClick: () =>
            void inboxService.undoBulk(res.operation).then((u) => {
              refresh();
              toast.success(t("inbox.bulk.undone", { count: u.applied }));
            }),
        },
        duration: 10_000,
      });
    } catch {
      // unwrap already toasted the API error.
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      className="bg-foreground text-background flex items-center gap-1 px-3 py-2 text-xs"
      role="toolbar"
      aria-label={t("inbox.bulk.label")}
    >
      <span className="mr-1 font-medium tabular-nums">
        {t("inbox.bulk.selected", { count: selected.length })}
      </span>
      <Button
        size="sm"
        variant="ghost"
        disabled={busy}
        className="hover:bg-background/10 h-7 px-2 text-xs"
        aria-label={t("inbox.read_state.mark_read")}
        title={t("inbox.read_state.mark_read")}
        onClick={() => void run("read")}
      >
        <MailOpen className="size-3.5" />
      </Button>
      <Button
        size="sm"
        variant="ghost"
        disabled={busy}
        className="hover:bg-background/10 h-7 px-2 text-xs"
        aria-label={t("inbox.read_state.mark_unread")}
        title={t("inbox.read_state.mark_unread")}
        onClick={() => void run("unread")}
      >
        <Mail className="size-3.5" />
      </Button>
      {canManage && (
        <Button
          size="sm"
          variant="ghost"
          disabled={busy}
          className="hover:bg-background/10 h-7 px-2 text-xs"
          onClick={() => void run("status", "resolved")}
        >
          <CheckCircle2 className="size-3.5" />
          {t("inbox.header.resolve")}
        </Button>
      )}
      {(canAssign || canManage) && (
        <Button
          size="sm"
          variant="ghost"
          disabled={busy}
          className="hover:bg-background/10 h-7 px-2 text-xs"
          onClick={() => void run("assign", "me")}
        >
          <UserRoundCheck className="size-3.5" />
          {t("inbox.bulk.assign_me")}
        </Button>
      )}
      {canManage && (
        <>
          <Popover>
            <PopoverTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                className="hover:bg-background/10 h-7 px-2 text-xs"
                aria-label={t("inbox.filters.priority")}
              >
                <Flag className="size-3.5" />
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-40 p-0">
              <Command>
                <CommandList>
                  <CommandGroup>
                    {PRIORITY_ORDER.map((p) => (
                      <CommandItem
                        key={p}
                        onSelect={() => void run("priority", p)}
                      >
                        {t(`inbox.priority.${p}`)}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
          <Popover>
            <PopoverTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                className="hover:bg-background/10 h-7 px-2 text-xs"
                aria-label={t("inbox.header.snooze")}
              >
                <AlarmClock className="size-3.5" />
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-44 p-0">
              <Command>
                <CommandList>
                  <CommandGroup>
                    <CommandItem
                      onSelect={() => void run("snooze", snoozeAt("tomorrow"))}
                    >
                      {t("inbox.snooze.tomorrow")}
                    </CommandItem>
                    <CommandItem
                      onSelect={() => void run("snooze", snoozeAt("next_week"))}
                    >
                      {t("inbox.snooze.next_week")}
                    </CommandItem>
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
          <Popover>
            <PopoverTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                className="hover:bg-background/10 h-7 px-2 text-xs"
                aria-label={t("inbox.filters.tag")}
              >
                <Hash className="size-3.5" />
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-56 p-2">
              <form
                className="flex gap-1"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (tag.trim()) void run("tag", tag.trim());
                  setTag("");
                }}
              >
                <Input
                  autoFocus
                  value={tag}
                  onChange={(e) => setTag(e.target.value)}
                  placeholder={t("inbox.bulk.tag_placeholder")}
                  className="h-8"
                />
                <Button type="submit" size="sm" disabled={!tag.trim()}>
                  {t("inbox.bulk.add")}
                </Button>
              </form>
            </PopoverContent>
          </Popover>
        </>
      )}
      <Button
        size="icon"
        variant="ghost"
        className="hover:bg-background/10 ml-auto size-7"
        aria-label={t("inbox.bulk.clear")}
        onClick={onClear}
      >
        <X className="size-3.5" />
      </Button>
    </div>
  );
}
