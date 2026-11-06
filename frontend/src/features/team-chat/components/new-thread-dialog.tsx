"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTeamPresence } from "@/features/presence/hooks/use-presence";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { useCreateThread } from "../hooks/use-team-chat";
import type { Thread } from "../services/team-chat.service";

/**
 * New thread: pick teammates from the presence list every member may read.
 * One teammate starts (or reopens) the direct thread; more make a group,
 * which the API requires a name for.
 */
export function NewThreadDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (thread: Thread) => void;
}) {
  const { t, locale } = useLocale();
  const { user } = useAuth();
  const presence = useTeamPresence();
  const create = useCreateThread();
  const [members, setMembers] = useState<string[]>([]);
  const [title, setTitle] = useState("");
  const [search, setSearch] = useState("");

  const direct = members.length === 1;
  const q = search.trim().toLocaleLowerCase(locale);
  const options = (presence.data ?? []).filter(
    (m) => m.user_uuid !== user?.uuid,
  );
  const shown = options.filter((m) =>
    m.name.toLocaleLowerCase(locale).includes(q),
  );
  const ready =
    members.length > 0 &&
    (direct || Boolean(title.trim())) &&
    !create.isPending;

  const toggle = (uuid: string) =>
    setMembers((cur) =>
      cur.includes(uuid) ? cur.filter((x) => x !== uuid) : [...cur, uuid],
    );
  const start = () => {
    if (!ready) return;
    create.mutate(
      {
        kind: direct ? "direct" : "group",
        members,
        ...(direct ? {} : { title: title.trim() }),
      },
      {
        onSuccess: onCreated,
        onError: (e) =>
          appToast.error(isApiError(e) ? e.message : t("team_chat.failed")),
      },
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("team_chat.new")}</DialogTitle>
          <DialogDescription>{t("team_chat.new_hint")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("team_chat.search_members")}
            aria-label={t("team_chat.search_members")}
          />
          <ul
            className="max-h-64 overflow-y-auto rounded-md border"
            aria-label={t("team_chat.to")}
          >
            {shown.length === 0 ? (
              <li className="text-muted-foreground p-3 text-center text-sm">
                {presence.isLoading
                  ? t("team_chat.loading")
                  : t("team_chat.no_members")}
              </li>
            ) : (
              shown.map((m) => (
                <li key={m.user_uuid}>
                  <label className="hover:bg-muted/60 flex cursor-pointer items-center gap-3 px-3 py-2 text-sm">
                    <Checkbox
                      checked={members.includes(m.user_uuid)}
                      onCheckedChange={() => toggle(m.user_uuid)}
                    />
                    <span className="truncate">{m.name}</span>
                  </label>
                </li>
              ))
            )}
          </ul>
          {members.length > 1 && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="team-chat-title">
                {t("team_chat.thread_title")}
              </Label>
              <Input
                id="team-chat-title"
                value={title}
                maxLength={200}
                onChange={(e) => setTitle(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && start()}
              />
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("team_chat.cancel")}
          </Button>
          <Button onClick={start} disabled={!ready}>
            {t("team_chat.start")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
