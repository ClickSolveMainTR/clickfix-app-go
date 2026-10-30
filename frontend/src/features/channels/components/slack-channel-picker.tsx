"use client";

import { Hash, Lock, Plus, RefreshCw, X } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useSlackConversations } from "@/features/channels/hooks/use-channels";
import { useLocale } from "@/providers/locale-provider";

/** Slack channel IDs: C… (public), G… (private), D… is never offered. */
const CHANNEL_ID = /^[CG][A-Z0-9]{6,}$/;

/** `settings.channel_ids` of a Slack instance as a string list. */
export function slackChannelIds(settings: Record<string, unknown>): string[] {
  const ids = settings.channel_ids;
  return Array.isArray(ids)
    ? ids.filter((x): x is string => typeof x === "string")
    : [];
}

/**
 * Allowlist of the Slack channels routed to the inbox (CF-119). Nothing is
 * ingested until a channel is picked. Lists the channels the bot was
 * invited to; an ID can also be typed when the listing is unavailable
 * (e.g. an install from before the `channels:read` scope).
 */
export function SlackChannelPicker({
  uuid,
  value,
  onChange,
}: {
  uuid: string;
  value: string[];
  onChange: (ids: string[]) => void;
}) {
  const { t } = useLocale();
  const list = useSlackConversations(uuid);
  const [manual, setManual] = useState("");
  const selected = new Set(value);
  const known = new Set((list.data ?? []).map((c) => c.id));
  const extra = value.filter((id) => !known.has(id));

  const toggle = (id: string, on: boolean) =>
    onChange(on ? [...value, id] : value.filter((x) => x !== id));
  const addManual = () => {
    const id = manual.trim().toUpperCase();
    if (!CHANNEL_ID.test(id) || selected.has(id)) return;
    onChange([...value, id]);
    setManual("");
  };

  return (
    <div className="space-y-3">
      <div className="flex items-start justify-between gap-2">
        <div>
          <Label>{t("channels.slack.channels")}</Label>
          <p className="text-muted-foreground text-xs">
            {t("channels.slack.channels_hint")}
          </p>
        </div>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          aria-label={t("channels.slack.refresh")}
          title={t("channels.slack.refresh")}
          disabled={list.isFetching}
          onClick={() => void list.refetch()}
        >
          <RefreshCw className="size-4" />
        </Button>
      </div>

      {list.isLoading ? (
        <Skeleton className="h-20" />
      ) : list.isError ? (
        <p className="text-muted-foreground rounded-md border border-dashed p-3 text-xs">
          {t("channels.slack.list_failed")}
        </p>
      ) : list.data && list.data.length > 0 ? (
        <ul className="max-h-60 divide-y overflow-y-auto rounded-md border">
          {list.data.map((c) => (
            <li key={c.id}>
              <label className="hover:bg-muted flex cursor-pointer items-center gap-3 px-3 py-2 text-sm">
                <Checkbox
                  checked={selected.has(c.id)}
                  onCheckedChange={(v) => toggle(c.id, v === true)}
                />
                {c.is_private ? (
                  <Lock className="text-muted-foreground size-3.5" />
                ) : (
                  <Hash className="text-muted-foreground size-3.5" />
                )}
                <span className="truncate">{c.name}</span>
                {c.is_shared ? (
                  <Badge variant="secondary" className="ms-auto">
                    {t("channels.slack.shared")}
                  </Badge>
                ) : null}
              </label>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-muted-foreground rounded-md border border-dashed p-3 text-xs">
          {t("channels.slack.none")}
        </p>
      )}

      {extra.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {extra.map((id) => (
            <Badge key={id} variant="outline" className="gap-1 font-mono">
              {id}
              <button
                type="button"
                aria-label={t("channels.slack.remove")}
                onClick={() => toggle(id, false)}
              >
                <X className="size-3" />
              </button>
            </Badge>
          ))}
        </div>
      ) : null}

      <div className="flex gap-2">
        <Input
          value={manual}
          placeholder="C0123ABCD"
          aria-label={t("channels.slack.manual")}
          className="font-mono text-xs"
          onChange={(e) => setManual(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addManual();
            }
          }}
        />
        <Button
          type="button"
          variant="outline"
          disabled={!CHANNEL_ID.test(manual.trim().toUpperCase())}
          onClick={addManual}
        >
          <Plus className="size-4" />
          {t("channels.slack.add_id")}
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        {value.length === 0
          ? t("channels.slack.empty_warning")
          : t("channels.slack.selected", { count: value.length })}
      </p>
    </div>
  );
}
