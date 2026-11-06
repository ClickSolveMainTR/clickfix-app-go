"use client";

import { MessagesSquare, Plus, Siren, User, UsersRound } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";

import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { shortAgo } from "@/features/inbox/lib/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useTeamChatLive,
  useTeamChatScope,
  useThreads,
} from "../hooks/use-team-chat";
import type { Thread } from "../services/team-chat.service";
import { NewThreadDialog } from "./new-thread-dialog";
import { ThreadPane } from "./thread-pane";

const KIND_ICON = { direct: User, group: UsersRound, swarm: Siren } as const;

/**
 * Team chat (CF-161): direct and group threads and the "swarm" rooms opened
 * from a conversation. Ported from the mobile screens
 * (mobile/src/features/team/threads-screen.tsx, thread-screen.tsx): thread
 * list with unread counts on the left, the open thread on the right; one
 * column on narrow screens. The open thread lives in `?thread=`.
 */
export function TeamChatPage() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const scope = useTeamChatScope();
  const threads = useThreads();
  const [creating, setCreating] = useState(false);
  useTeamChatLive();

  const activeUuid = params.get("thread");
  const select = useCallback(
    (uuid: string | null) => {
      const next = new URLSearchParams(params.toString());
      if (uuid) next.set("thread", uuid);
      else next.delete("thread");
      const qs = next.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [params, pathname, router],
  );

  if (!scope.org) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("team_chat.no_organization")}
      </p>
    );
  }
  if (!scope.canUse) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("team_chat.no_permission")}
      </p>
    );
  }

  const items = threads.data?.items ?? [];
  const active = items.find((th) => th.uuid === activeUuid);
  const titleOf = (th: Thread) => th.title || t(`team_chat.kind_${th.kind}`);

  return (
    <div className="bg-background -mx-4 -mb-6 flex h-[calc(100svh-4.5rem)] min-h-[480px] overflow-hidden border-t md:-mx-6">
      <section
        className={cn(
          "flex w-full shrink-0 flex-col border-r md:w-[320px] xl:w-[360px]",
          activeUuid && "hidden md:flex",
        )}
        aria-label={t("team_chat.threads")}
      >
        <div className="flex items-center justify-between gap-2 border-b p-3">
          <h1 className="truncate text-sm font-semibold">
            {t("team_chat.title")}
          </h1>
          <Button size="sm" onClick={() => setCreating(true)}>
            <Plus className="size-4" />
            {t("team_chat.new")}
          </Button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {threads.isLoading ? (
            <Loading />
          ) : threads.isError ? (
            <div className="flex flex-col items-center gap-2 p-8 text-center">
              <p className="text-muted-foreground text-sm">
                {t("team_chat.load_failed")}
              </p>
              <Button
                size="sm"
                variant="outline"
                onClick={() => void threads.refetch()}
              >
                {t("team_chat.retry")}
              </Button>
            </div>
          ) : items.length === 0 ? (
            <Placeholder
              title={t("team_chat.empty")}
              hint={t("team_chat.empty_hint")}
            />
          ) : (
            <ul>
              {items.map((th) => {
                const Icon = KIND_ICON[th.kind] ?? MessagesSquare;
                const unread = th.unread ?? 0;
                return (
                  <li key={th.uuid}>
                    <button
                      type="button"
                      onClick={() => select(th.uuid)}
                      aria-current={th.uuid === activeUuid ? "true" : undefined}
                      className={cn(
                        "hover:bg-muted/60 relative flex w-full items-center gap-3 border-b px-3 py-2.5 text-left",
                        th.uuid === activeUuid && "bg-muted",
                      )}
                    >
                      <span className="bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-full">
                        <Icon className="size-4" />
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                        <span
                          className={cn(
                            "truncate text-sm",
                            unread > 0 ? "font-semibold" : "font-medium",
                          )}
                        >
                          {titleOf(th)}
                        </span>
                        <span className="text-muted-foreground truncate text-xs">
                          {[
                            t(`team_chat.kind_${th.kind}`),
                            th.closed_at ? t("team_chat.closed") : null,
                            th.updated_at
                              ? shortAgo(th.updated_at, locale)
                              : null,
                          ]
                            .filter(Boolean)
                            .join(" · ")}
                        </span>
                      </span>
                      {unread > 0 && (
                        <span
                          className="bg-signal text-signal-foreground min-w-5 shrink-0 rounded-full px-1.5 py-0.5 text-center text-[11px] font-semibold tabular-nums"
                          aria-label={t("team_chat.unread", { count: unread })}
                        >
                          {unread > 99 ? "99+" : unread}
                        </span>
                      )}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </section>

      <section
        className={cn(
          "min-w-0 flex-1 flex-col",
          activeUuid ? "flex" : "hidden md:flex",
        )}
        aria-label={t("team_chat.thread")}
      >
        {activeUuid ? (
          <ThreadPane
            key={activeUuid}
            uuid={activeUuid}
            title={active ? titleOf(active) : t("team_chat.title")}
            subtitle={active ? t(`team_chat.kind_${active.kind}`) : ""}
            closed={Boolean(active?.closed_at)}
            onBack={() => select(null)}
          />
        ) : (
          <Placeholder
            title={t("team_chat.pick")}
            hint={t("team_chat.pick_hint")}
          />
        )}
      </section>

      {creating && (
        <NewThreadDialog
          onClose={() => setCreating(false)}
          onCreated={(th) => {
            setCreating(false);
            select(th.uuid);
          }}
        />
      )}
    </div>
  );
}

function Placeholder({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="flex h-full flex-1 flex-col items-center justify-center gap-2 p-8 text-center">
      <div className="bg-signal-soft text-signal-text mb-1 flex size-10 items-center justify-center rounded-full">
        <MessagesSquare className="size-5" />
      </div>
      <p className="text-sm font-medium">{title}</p>
      {hint && <p className="text-muted-foreground max-w-xs text-xs">{hint}</p>}
    </div>
  );
}
