"use client";

import { CallButton } from "@/features/softphone/softphone";
import {
  forwardRef,
  useImperativeHandle,
  useState,
  type ReactNode,
} from "react";
import {
  AlarmClock,
  Check,
  CheckCircle2,
  ChevronDown,
  CircleDot,
  Flag,
  Hash,
  PanelRight,
  RotateCcw,
  UserRound,
  UsersRound,
  X,
  Zap,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useApplyMacro, useMacros } from "@/features/macros/hooks/use-macros";
import { BrandBadge } from "@/features/brands/components/brand-badge";
import { StatusDot } from "@/features/conversation-statuses/components/status-dot";
import { useTeamsList } from "@/features/teams/hooks/use-teams";
import { RemoteSupportButton } from "@/features/remote-support/components/remote-support-button";
import { StartVideoButton } from "@/features/video/components/start-video-button";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useConversationTag,
  useInboxAgents,
  useInboxCustomStatuses,
  useInboxTags,
  useUpdateConversation,
} from "../hooks/use-inbox";
import {
  PRIORITY_ORDER,
  PRIORITY_TONE,
  contactLabel,
  initials,
} from "../lib/format";
import type {
  ConversationDetail,
  ConversationPriority,
  ConversationStatus,
} from "../types";
import { ScreenContextChip } from "./screen-context-chip";
import { ChannelGlyph } from "./channel-glyph";
import { ReminderMenu } from "./reminder-menu";

export type Picker =
  "status" | "assign" | "team" | "tag" | "priority" | "snooze" | "macro";

/** Base statuses in the picker; snoozed opens the snooze times. */
const BASE_STATUSES: ConversationStatus[] = [
  "open",
  "pending",
  "snoozed",
  "resolved",
  "closed",
];

export type HeaderHandle = {
  open: (picker: Picker) => void;
  toggleResolve: () => void;
  takeIt: () => void;
};

type Props = {
  conversation: ConversationDetail;
  canManage: boolean;
  canAssign: boolean;
  contextOpen: boolean;
  onToggleContext: () => void;
};

/** Conversation title bar with status, priority, assignee, tags and snooze. */
export const ConversationHeader = forwardRef<HeaderHandle, Props>(
  function ConversationHeader(
    { conversation: c, canManage, canAssign, contextOpen, onToggleContext },
    ref,
  ) {
    const { t } = useLocale();
    const { user } = useAuth();
    const [picker, setPicker] = useState<Picker | null>(null);
    const update = useUpdateConversation(c.uuid);
    const tagging = useConversationTag(c.uuid);
    const agents = useInboxAgents();
    const tags = useInboxTags();
    const customStatuses = useInboxCustomStatuses();
    const teams = useTeamsList({ limit: 100, offset: 0 }, canAssign);
    const resolved = c.status === "resolved" || c.status === "closed";
    const macros = useMacros();
    const applyMacro = useApplyMacro(c.uuid);

    const toggleResolve = () => {
      if (!canManage) return;
      update.mutate({ status: resolved ? "open" : "resolved" });
    };
    const takeIt = () => {
      if (!canAssign || !user?.uuid || c.assignee?.uuid === user.uuid) return;
      update.mutate({ assignee: user.uuid });
    };

    useImperativeHandle(ref, () => ({
      open: (p) =>
        (p === "assign" || p === "team"
          ? canAssign
          : p === "macro"
            ? true
            : canManage) && setPicker(p),
      toggleResolve,
      takeIt,
    }));

    const pickerProps = (p: Picker) => ({
      open: picker === p,
      onOpenChange: (o: boolean) => setPicker(o ? p : null),
    });

    return (
      <header className="flex min-h-14 flex-wrap items-center gap-x-2 gap-y-1.5 border-b px-4 py-2">
        <div className="flex min-w-48 flex-1 items-center gap-2.5">
          <ChannelGlyph type={c.channel_type} className="size-4 shrink-0" />
          <div className="min-w-0">
            <h2 className="truncate text-sm leading-tight font-semibold">
              {c.subject || contactLabel(c)}
            </h2>
            <p className="text-muted-foreground flex min-w-0 items-center gap-1.5 text-xs">
              <span className="truncate">
                #{c.number} · {c.inbox?.name ?? t("inbox.header.no_inbox")}
              </span>
              <BrandBadge brandId={c.brand_id} />
              <ScreenContextChip conversation={c} />
            </p>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          {/* Status (CF-249): base statuses and the organization's own. */}
          <Popover {...pickerProps("status")}>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={!canManage}
                className="h-8 gap-1.5"
                aria-label={t("inbox.header.status")}
              >
                {c.custom_status ? (
                  <StatusDot color={c.custom_status.color} />
                ) : (
                  <CircleDot className="size-3.5" />
                )}
                <span className="max-w-28 truncate">
                  {c.custom_status?.name ?? t(`inbox.status.${c.status}`)}
                </span>
                <ChevronDown className="size-3 opacity-60" />
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-60 p-0" align="end">
              <Command>
                <CommandList>
                  <CommandGroup heading={t("inbox.header.status")}>
                    {BASE_STATUSES.map((s) => (
                      <CommandItem
                        key={s}
                        value={`base ${s}`}
                        onSelect={() => {
                          if (s === "snoozed") {
                            setPicker("snooze");
                            return;
                          }
                          update.mutate({ status: s, custom_status: null });
                          setPicker(null);
                        }}
                      >
                        <CircleDot className="text-muted-foreground size-3.5" />
                        {t(`inbox.status.${s}`)}
                        {c.status === s && !c.custom_status && (
                          <Check className="ml-auto size-3.5" />
                        )}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                  {(customStatuses.data?.length ?? 0) > 0 && (
                    <CommandGroup heading={t("inbox.header.custom_statuses")}>
                      {(customStatuses.data ?? []).map((s) => (
                        <CommandItem
                          key={s.uuid}
                          value={`custom ${s.uuid} ${s.name}`}
                          onSelect={() => {
                            update.mutate({ custom_status: s.uuid });
                            setPicker(null);
                          }}
                        >
                          <StatusDot color={s.color} className="mx-0.5" />
                          <span className="truncate">{s.name}</span>
                          <span className="text-muted-foreground ml-auto text-[11px]">
                            {t(`inbox.status.${s.category}`)}
                          </span>
                          {c.custom_status?.uuid === s.uuid && (
                            <Check className="size-3.5" />
                          )}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  )}
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>

          {/* Assignee */}
          <Popover {...pickerProps("assign")}>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={!canAssign}
                className="h-8 gap-1.5"
              >
                {c.assignee ? (
                  <span className="bg-muted flex size-5 items-center justify-center rounded-full text-[9px] font-semibold">
                    {initials(c.assignee.name)}
                  </span>
                ) : (
                  <UserRound className="size-3.5" />
                )}
                <span className="max-w-28 truncate">
                  {c.assignee?.name ?? t("inbox.header.assign")}
                </span>
                <Kbd>A</Kbd>
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-64 p-0" align="end">
              <Command>
                <CommandInput placeholder={t("inbox.header.search_agent")} />
                <CommandList>
                  <CommandEmpty>{t("inbox.header.no_match")}</CommandEmpty>
                  <CommandGroup>
                    {c.assignee && (
                      <CommandItem
                        onSelect={() => {
                          update.mutate({ assignee: null });
                          setPicker(null);
                        }}
                      >
                        <X className="size-3.5" />
                        {t("inbox.header.unassign")}
                      </CommandItem>
                    )}
                    {(agents.data ?? []).map((a) => (
                      <CommandItem
                        key={a.uuid}
                        value={`${a.name} ${a.email}`}
                        onSelect={() => {
                          update.mutate({ assignee: a.uuid });
                          setPicker(null);
                        }}
                      >
                        <span className="bg-muted flex size-5 items-center justify-center rounded-full text-[9px] font-semibold">
                          {initials(a.name)}
                        </span>
                        <span className="truncate">{a.name}</span>
                        {a.uuid === user?.uuid && (
                          <span className="text-muted-foreground text-[11px]">
                            ({t("inbox.header.you")})
                          </span>
                        )}
                        {c.assignee?.uuid === a.uuid && (
                          <Check className="ml-auto size-3.5" />
                        )}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>

          {/* Team */}
          <Popover {...pickerProps("team")}>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={!canAssign}
                className="h-8 gap-1.5"
                aria-label={t("inbox.header.team")}
              >
                <UsersRound className="size-3.5" />
                <span className="hidden max-w-24 truncate xl:inline">
                  {c.team?.name ?? t("inbox.header.team")}
                </span>
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-56 p-0" align="end">
              <Command>
                <CommandInput placeholder={t("inbox.header.search_team")} />
                <CommandList>
                  <CommandEmpty>{t("inbox.header.no_match")}</CommandEmpty>
                  <CommandGroup>
                    {c.team && (
                      <CommandItem
                        onSelect={() => {
                          update.mutate({ team: null });
                          setPicker(null);
                        }}
                      >
                        <X className="size-3.5" />
                        {t("inbox.header.no_team")}
                      </CommandItem>
                    )}
                    {(teams.data?.items ?? []).map((team) => (
                      <CommandItem
                        key={team.uuid}
                        value={team.name}
                        onSelect={() => {
                          update.mutate({ team: team.uuid });
                          setPicker(null);
                        }}
                      >
                        <UsersRound className="size-3.5" />
                        <span className="truncate">{team.name}</span>
                        {c.team?.uuid === team.uuid && (
                          <Check className="ml-auto size-3.5" />
                        )}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>

          {/* Priority */}
          <Popover {...pickerProps("priority")}>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={!canManage}
                className="h-8 gap-1.5"
              >
                <Flag className={cn("size-3.5", PRIORITY_TONE[c.priority])} />
                <span className="hidden xl:inline">
                  {t(`inbox.priority.${c.priority}`)}
                </span>
                <Kbd>P</Kbd>
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-44 p-0" align="end">
              <Command>
                <CommandList>
                  <CommandGroup>
                    {PRIORITY_ORDER.map((p) => (
                      <CommandItem
                        key={p}
                        onSelect={() => {
                          update.mutate({
                            priority: p as ConversationPriority,
                          });
                          setPicker(null);
                        }}
                      >
                        <Flag className={cn("size-3.5", PRIORITY_TONE[p])} />
                        {t(`inbox.priority.${p}`)}
                        {c.priority === p && (
                          <Check className="ml-auto size-3.5" />
                        )}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>

          {/* Tags */}
          <TagPicker
            {...pickerProps("tag")}
            disabled={!canManage}
            selected={c.tags.map((x) => x.uuid)}
            options={tags.data ?? []}
            onAdd={(tag) => tagging.add.mutate(tag)}
            onRemove={(uuid) => tagging.remove.mutate(uuid)}
          />

          {/* Macros */}
          {(macros.data?.length ?? 0) > 0 && (
            <Popover {...pickerProps("macro")}>
              <PopoverTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 gap-1.5"
                  aria-label={t("inbox.header.macros")}
                >
                  <Zap className="size-3.5" />
                  <Kbd>M</Kbd>
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-72 p-0" align="end">
                <Command>
                  <CommandInput placeholder={t("inbox.header.search_macro")} />
                  <CommandList>
                    <CommandEmpty>{t("inbox.header.no_match")}</CommandEmpty>
                    <CommandGroup heading={t("inbox.header.macros")}>
                      {(macros.data ?? []).map((m) => (
                        <CommandItem
                          key={m.uuid}
                          value={m.name}
                          disabled={applyMacro.isPending}
                          onSelect={() => {
                            applyMacro.mutate(m.uuid);
                            setPicker(null);
                          }}
                        >
                          <Zap className="size-3.5 text-amber-500" />
                          <span className="truncate">{m.name}</span>
                          <span className="text-muted-foreground ml-auto text-[11px]">
                            {m.actions.length}
                          </span>
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
          )}
          <ReminderMenu conversation={c.uuid} />
          <CallButton phone={c.contact.phone} />
          <StartVideoButton
            conversation={c.uuid}
            name={c.subject || contactLabel(c)}
          />
          <RemoteSupportButton conversation={c.uuid} />

          {/* Snooze */}
          <Popover {...pickerProps("snooze")}>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={!canManage}
                className="h-8 gap-1.5"
                aria-label={t("inbox.header.snooze")}
              >
                <AlarmClock className="size-3.5" />
                <Kbd>S</Kbd>
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-56 p-0" align="end">
              <Command>
                <CommandList>
                  <CommandGroup heading={t("inbox.header.snooze")}>
                    {snoozeOptions().map((o) => (
                      <CommandItem
                        key={o.key}
                        onSelect={() => {
                          update.mutate({
                            status: "snoozed",
                            snoozed_until: o.at.toISOString(),
                            custom_status: null,
                          });
                          setPicker(null);
                        }}
                      >
                        {t(`inbox.snooze.${o.key}`)}
                        <span className="text-muted-foreground ml-auto text-[11px]">
                          {o.at.toLocaleString([], {
                            weekday: "short",
                            hour: "2-digit",
                            minute: "2-digit",
                          })}
                        </span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>

          <Button
            size="sm"
            variant={resolved ? "outline" : "default"}
            disabled={!canManage}
            onClick={toggleResolve}
            className="h-8 gap-1.5"
          >
            {resolved ? (
              <RotateCcw className="size-3.5" />
            ) : (
              <CheckCircle2 className="size-3.5" />
            )}
            <span className={cn(resolved && "hidden 2xl:inline")}>
              {resolved ? t("inbox.header.reopen") : t("inbox.header.resolve")}
            </span>
            <Kbd inverted={!resolved}>E</Kbd>
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            aria-pressed={contextOpen}
            aria-label={t("inbox.header.toggle_context")}
            onClick={onToggleContext}
          >
            <PanelRight className="size-4" />
          </Button>
        </div>
      </header>
    );
  },
);

function Kbd({
  children,
  inverted,
}: {
  children: ReactNode;
  inverted?: boolean;
}) {
  return (
    <kbd
      className={cn(
        "hidden rounded border px-1 font-mono text-[10px] leading-4 2xl:inline",
        inverted
          ? "border-primary-foreground/30 text-primary-foreground/70"
          : "text-muted-foreground",
      )}
    >
      {children}
    </kbd>
  );
}

function TagPicker({
  open,
  onOpenChange,
  disabled,
  selected,
  options,
  onAdd,
  onRemove,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  disabled: boolean;
  selected: string[];
  options: { uuid: string; name: string }[];
  onAdd: (tag: { uuid?: string; name?: string }) => void;
  onRemove: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const [search, setSearch] = useState("");
  const exact = options.some(
    (o) =>
      o.name.toLocaleLowerCase("tr") === search.trim().toLocaleLowerCase("tr"),
  );
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          disabled={disabled}
          className="h-8 gap-1.5"
        >
          <Hash className="size-3.5" />
          {selected.length > 0 ? (
            selected.length
          ) : (
            <span className="hidden xl:inline">{t("inbox.header.tags")}</span>
          )}
          <ChevronDown className="size-3 opacity-60" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-60 p-0" align="end">
        <Command>
          <CommandInput
            value={search}
            onValueChange={setSearch}
            placeholder={t("inbox.header.search_tag")}
          />
          <CommandList>
            <CommandGroup>
              {options.map((o) => {
                const on = selected.includes(o.uuid);
                return (
                  <CommandItem
                    key={o.uuid}
                    value={o.name}
                    onSelect={() =>
                      on ? onRemove(o.uuid) : onAdd({ uuid: o.uuid })
                    }
                  >
                    <Hash className="text-muted-foreground size-3.5" />
                    {o.name}
                    {on && <Check className="ml-auto size-3.5" />}
                  </CommandItem>
                );
              })}
              {search.trim() && !exact && (
                <CommandItem
                  value={`__create ${search}`}
                  onSelect={() => {
                    onAdd({ name: search.trim() });
                    setSearch("");
                  }}
                >
                  {t("inbox.header.create_tag", { name: search.trim() })}
                </CommandItem>
              )}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function snoozeOptions(): { key: string; at: Date }[] {
  const now = new Date();
  const inHours = (h: number) => new Date(now.getTime() + h * 3_600_000);
  const at = (days: number, hour: number) => {
    const d = new Date(now);
    d.setDate(d.getDate() + days);
    d.setHours(hour, 0, 0, 0);
    return d;
  };
  const monday = at((8 - now.getDay()) % 7 || 7, 9);
  return [
    { key: "one_hour", at: inHours(1) },
    { key: "three_hours", at: inHours(3) },
    { key: "tomorrow", at: at(1, 9) },
    { key: "next_week", at: monday },
  ];
}
