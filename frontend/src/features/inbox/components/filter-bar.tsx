"use client";

import { useState } from "react";
import { Bookmark, BookmarkPlus, Check, Filter, Trash2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useBrands } from "@/features/brands/hooks/use-brands";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useConversationViewCounts,
  useConversationViewMutations,
  useConversationViews,
  useInboxScope,
  useInboxTags,
} from "../hooks/use-inbox";
import { PRIORITY_ORDER } from "../lib/format";
import type { ConversationViewFilters, InboxFilters } from "../types";
import { ChannelGlyph } from "./channel-glyph";

const CHANNELS = [
  "whatsapp",
  "instagram",
  "messenger",
  "telegram",
  "email",
  "widget",
];

type Props = {
  filters: InboxFilters;
  savedView: string | null;
  setParams: (patch: Record<string, string | null>) => void;
};

const csv = (v: string[]) => (v.length ? v.join(",") : null);
const toggle = (list: string[] | undefined, v: string) =>
  list?.includes(v) ? list.filter((x) => x !== v) : [...(list ?? []), v];

/** Advanced filters and saved views of the inbox list (CF-47). */
export function FilterBar({ filters, savedView, setParams }: Props) {
  const { t } = useLocale();
  const scope = useInboxScope();
  const tags = useInboxTags();
  const brands = useBrands();
  const views = useConversationViews();
  const counts = useConversationViewCounts();
  const m = useConversationViewMutations();
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  const [shared, setShared] = useState(false);
  const active = (views.data ?? []).find((v) => v.uuid === savedView);
  const advanced =
    (filters.priorities?.length ?? 0) +
    (filters.channelTypes?.length ?? 0) +
    (filters.tags?.length ?? 0) +
    (filters.brand ? 1 : 0) +
    (filters.identity ? 1 : 0);

  const current = (): ConversationViewFilters => {
    const f: ConversationViewFilters = {};
    const statuses: Record<string, ConversationViewFilters["statuses"]> = {
      open: ["open", "pending"],
      snoozed: ["snoozed"],
      resolved: ["resolved", "closed"],
    };
    if (statuses[filters.status]) f.statuses = statuses[filters.status];
    if (filters.view === "mine") f.assignee = "me";
    if (filters.view === "unassigned") f.assignee = "unassigned";
    if (filters.view === "mentions") f.mentioned = true;
    if (filters.view === "sla_soon") f.sla_due_within_minutes = 60;
    if (filters.view === "breached") f.sla_breached = true;
    if (filters.unread) f.unread = true;
    if (filters.q) f.q = filters.q;
    if (filters.priorities?.length)
      f.priorities =
        filters.priorities as ConversationViewFilters["priorities"];
    if (filters.channelTypes?.length) f.channel_types = filters.channelTypes;
    if (filters.tags?.length) f.tags = filters.tags;
    return f;
  };

  return (
    <div className="flex items-center gap-1">
      <Popover>
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={cn(
              "h-7 gap-1 px-2 text-xs",
              advanced > 0 && "text-signal-text",
            )}
          >
            <Filter className="size-3.5" />
            {advanced > 0 ? advanced : t("inbox.filters.title")}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-64 p-0" align="end">
          <Command>
            <CommandList className="max-h-96">
              <CommandGroup heading={t("inbox.filters.priority")}>
                {PRIORITY_ORDER.map((p) => (
                  <CommandItem
                    key={p}
                    onSelect={() =>
                      setParams({ pr: csv(toggle(filters.priorities, p)) })
                    }
                  >
                    <Check
                      className={cn(
                        "size-3.5",
                        !filters.priorities?.includes(p) && "opacity-0",
                      )}
                    />
                    {t(`inbox.priority.${p}`)}
                  </CommandItem>
                ))}
              </CommandGroup>
              <CommandSeparator />
              <CommandGroup heading={t("inbox.filters.identity")}>
                {(["verified", "self_named", "anonymous"] as const).map(
                  (st) => (
                    <CommandItem
                      key={st}
                      value={`identity-${st}`}
                      onSelect={() =>
                        setParams({ id: filters.identity === st ? null : st })
                      }
                    >
                      <Check
                        className={cn(
                          "size-3.5",
                          filters.identity !== st && "opacity-0",
                        )}
                      />
                      {t(`contacts.identity.${st}`)}
                    </CommandItem>
                  ),
                )}
              </CommandGroup>
              <CommandSeparator />
              <CommandGroup heading={t("inbox.filters.channel")}>
                {CHANNELS.map((ch) => (
                  <CommandItem
                    key={ch}
                    onSelect={() =>
                      setParams({ ch: csv(toggle(filters.channelTypes, ch)) })
                    }
                  >
                    <Check
                      className={cn(
                        "size-3.5",
                        !filters.channelTypes?.includes(ch) && "opacity-0",
                      )}
                    />
                    <ChannelGlyph type={ch} className="size-3.5" />
                    {t(`channels.types.${ch}`)}
                  </CommandItem>
                ))}
              </CommandGroup>
              {(tags.data ?? []).length > 0 && (
                <>
                  <CommandSeparator />
                  <CommandGroup heading={t("inbox.filters.tag")}>
                    {(tags.data ?? []).map((tag) => (
                      <CommandItem
                        key={tag.uuid}
                        onSelect={() =>
                          setParams({ tg: csv(toggle(filters.tags, tag.uuid)) })
                        }
                      >
                        <Check
                          className={cn(
                            "size-3.5",
                            !filters.tags?.includes(tag.uuid) && "opacity-0",
                          )}
                        />
                        #{tag.name}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </>
              )}
              {(brands.data ?? []).length > 0 && (
                <>
                  <CommandSeparator />
                  <CommandGroup heading={t("brands.filter.title")}>
                    {(brands.data ?? []).map((b) => (
                      <CommandItem
                        key={b.uuid}
                        value={`brand-${b.uuid}`}
                        onSelect={() =>
                          setParams({
                            br: filters.brand === b.uuid ? null : b.uuid,
                          })
                        }
                      >
                        <Check
                          className={cn(
                            "size-3.5",
                            filters.brand !== b.uuid && "opacity-0",
                          )}
                        />
                        <span
                          aria-hidden
                          className="size-2.5 rounded-full ring-1 ring-black/10 dark:ring-white/20"
                          style={{
                            backgroundColor: b.primary_color ?? "#2563eb",
                          }}
                        />
                        <span className="truncate">{b.name}</span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </>
              )}
              {advanced > 0 && (
                <>
                  <CommandSeparator />
                  <CommandGroup>
                    <CommandItem
                      onSelect={() =>
                        setParams({
                          pr: null,
                          ch: null,
                          tg: null,
                          br: null,
                          id: null,
                        })
                      }
                    >
                      <X className="size-3.5" />
                      {t("inbox.filters.clear")}
                    </CommandItem>
                  </CommandGroup>
                </>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      <Popover onOpenChange={(o) => !o && setNaming(false)}>
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={cn(
              "h-7 max-w-36 gap-1 px-2 text-xs",
              active && "text-signal-text",
            )}
          >
            <Bookmark className="size-3.5" />
            <span className="truncate">
              {active?.name ?? t("inbox.views_saved.title")}
            </span>
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-72 p-0" align="end">
          {naming ? (
            <form
              className="flex flex-col gap-2 p-3"
              onSubmit={(e) => {
                e.preventDefault();
                m.create.mutate(
                  {
                    name,
                    filters: current(),
                    scope: shared ? "organization" : "personal",
                  },
                  {
                    onSuccess: (v) => {
                      setNaming(false);
                      setName("");
                      setParams({ sv: v.uuid });
                    },
                  },
                );
              }}
            >
              <Input
                autoFocus
                value={name}
                maxLength={60}
                placeholder={t("inbox.views_saved.name")}
                onChange={(e) => setName(e.target.value)}
              />
              {scope.canManage && (
                <label className="flex items-center gap-2 text-xs">
                  <input
                    type="checkbox"
                    checked={shared}
                    onChange={(e) => setShared(e.target.checked)}
                  />
                  {t("inbox.views_saved.share")}
                </label>
              )}
              <Button
                type="submit"
                size="sm"
                disabled={!name.trim() || m.create.isPending}
              >
                {t("inbox.views_saved.save")}
              </Button>
            </form>
          ) : (
            <Command>
              <CommandList>
                {(views.data ?? []).length > 0 && (
                  <CommandGroup heading={t("inbox.views_saved.title")}>
                    {(views.data ?? []).map((v) => (
                      <CommandItem
                        key={v.uuid}
                        value={v.uuid}
                        onSelect={() =>
                          setParams({
                            sv: v.uuid === savedView ? null : v.uuid,
                            c: null,
                          })
                        }
                      >
                        <Check
                          className={cn(
                            "size-3.5",
                            v.uuid !== savedView && "opacity-0",
                          )}
                        />
                        <span className="truncate">{v.name}</span>
                        {v.scope !== "personal" && (
                          <span className="text-muted-foreground text-[10px]">
                            {t("inbox.views_saved.shared")}
                          </span>
                        )}
                        <span className="text-muted-foreground ml-auto text-[11px] tabular-nums">
                          {counts.data?.[v.uuid] ?? ""}
                        </span>
                        {(v.mine || scope.canManage) && (
                          <button
                            type="button"
                            aria-label={t("inbox.views_saved.delete")}
                            className="text-muted-foreground hover:text-destructive"
                            onClick={(e) => {
                              e.stopPropagation();
                              m.remove.mutate(v.uuid);
                              if (v.uuid === savedView) setParams({ sv: null });
                            }}
                          >
                            <Trash2 className="size-3.5" />
                          </button>
                        )}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem onSelect={() => setNaming(true)}>
                    <BookmarkPlus className="size-3.5" />
                    {t("inbox.views_saved.save_current")}
                  </CommandItem>
                </CommandGroup>
              </CommandList>
            </Command>
          )}
        </PopoverContent>
      </Popover>
    </div>
  );
}
