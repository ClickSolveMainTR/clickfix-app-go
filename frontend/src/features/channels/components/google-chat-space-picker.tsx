"use client";

import { Plus, RefreshCw, Users, X } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useGoogleChatSpaces } from "@/features/channels/hooks/use-channels";
import { useLocale } from "@/providers/locale-provider";

/** Chat API space resource name (`spaces/AAAA…`). */
const SPACE_NAME = /^spaces\/[A-Za-z0-9_-]+$/;

/** A typed space id, with or without the `spaces/` prefix. */
function spaceName(raw: string): string {
  const v = raw.trim();
  return v.startsWith("spaces/") ? v : `spaces/${v}`;
}

/** `settings.space_ids` of a Google Chat instance as a string list. */
export function googleChatSpaceIds(
  settings: Record<string, unknown>,
): string[] {
  const ids = settings.space_ids;
  return Array.isArray(ids)
    ? ids.filter((x): x is string => typeof x === "string")
    : [];
}

/**
 * Allowlist of the Google Chat rooms routed to the inbox (CF-137). Direct
 * messages with the app always arrive; a room only once it is picked here.
 * Lists the rooms the app was added to; a space id can also be typed.
 */
export function GoogleChatSpacePicker({
  uuid,
  value,
  onChange,
}: {
  uuid: string;
  value: string[];
  onChange: (ids: string[]) => void;
}) {
  const { t } = useLocale();
  const list = useGoogleChatSpaces(uuid);
  const [manual, setManual] = useState("");
  const selected = new Set(value);
  const known = new Set((list.data ?? []).map((s) => s.name));
  const extra = value.filter((id) => !known.has(id));
  const typed = spaceName(manual);
  const typedValid = manual.trim() !== "" && SPACE_NAME.test(typed);

  const toggle = (id: string, on: boolean) =>
    onChange(on ? [...value, id] : value.filter((x) => x !== id));
  const addManual = () => {
    if (!typedValid || selected.has(typed)) return;
    onChange([...value, typed]);
    setManual("");
  };

  return (
    <div className="space-y-3">
      <div className="flex items-start justify-between gap-2">
        <div>
          <Label>{t("channels.google_chat.spaces")}</Label>
          <p className="text-muted-foreground text-xs">
            {t("channels.google_chat.spaces_hint")}
          </p>
        </div>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          aria-label={t("channels.google_chat.refresh")}
          title={t("channels.google_chat.refresh")}
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
          {t("channels.google_chat.list_failed")}
        </p>
      ) : list.data && list.data.length > 0 ? (
        <ul className="max-h-60 divide-y overflow-y-auto rounded-md border">
          {list.data.map((s) => (
            <li key={s.name}>
              <label className="hover:bg-muted flex cursor-pointer items-center gap-3 px-3 py-2 text-sm">
                <Checkbox
                  checked={selected.has(s.name)}
                  onCheckedChange={(v) => toggle(s.name, v === true)}
                />
                <Users className="text-muted-foreground size-3.5" />
                <span className="truncate">{s.display_name || s.name}</span>
                {s.space_type === "GROUP_CHAT" ? (
                  <Badge variant="secondary" className="ms-auto">
                    {t("channels.google_chat.group_chat")}
                  </Badge>
                ) : null}
              </label>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-muted-foreground rounded-md border border-dashed p-3 text-xs">
          {t("channels.google_chat.none")}
        </p>
      )}

      {extra.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {extra.map((id) => (
            <Badge key={id} variant="outline" className="gap-1 font-mono">
              {id}
              <button
                type="button"
                aria-label={t("channels.google_chat.remove")}
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
          placeholder="spaces/AAAAbCdEfGh"
          aria-label={t("channels.google_chat.manual")}
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
          disabled={!typedValid}
          onClick={addManual}
        >
          <Plus className="size-4" />
          {t("channels.google_chat.add_id")}
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        {value.length === 0
          ? t("channels.google_chat.empty_warning")
          : t("channels.google_chat.selected", { count: value.length })}
      </p>
    </div>
  );
}
