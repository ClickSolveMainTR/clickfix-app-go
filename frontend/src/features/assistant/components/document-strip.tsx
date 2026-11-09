"use client";

import { useAuiState } from "@assistant-ui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2 } from "lucide-react";
import { useState } from "react";

import {
  Attachment,
  AttachmentAction,
  AttachmentGroup,
  AttachmentTitle,
} from "@/components/ai-elements/attachment";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useLocale } from "@/providers/locale-provider";

import { documentErrorKey } from "../lib/errors";
import { useAssistantSurface } from "../runtime/surface";
import { assistantService } from "../services/assistant.service";

export const documentsKey = (slug: string, thread: string) =>
  ["assistant", slug, "documents", thread] as const;

/**
 * Documents the assistant can read in this thread (CF-191): files arrive
 * through the composer's attachment adapter, links through the popover
 * here. Only the text is kept (7 days) with personal data and keys masked.
 */
export function DocumentStrip({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const { slug, ensureThread } = useAssistantSurface();
  const qc = useQueryClient();
  const remoteId = useAuiState((s) => s.threadListItem.remoteId);
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);

  const docs = useQuery({
    queryKey: documentsKey(slug, remoteId ?? ""),
    queryFn: () => assistantService.documents(slug, remoteId ?? ""),
    enabled: Boolean(remoteId),
  });
  const link = useMutation({
    mutationFn: async (href: string) =>
      assistantService.linkDocument(slug, await ensureThread(), href),
    onMutate: () => setError(null),
    onSuccess: () => {
      setUrl("");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["assistant", slug, "documents"] });
    },
    onError: (err) => setError(documentErrorKey(err)),
  });
  const remove = useMutation({
    mutationFn: (document: string) =>
      assistantService.removeDocument(slug, remoteId ?? "", document),
    onSettled: () =>
      void qc.invalidateQueries({ queryKey: ["assistant", slug, "documents"] }),
  });

  const items = docs.data?.items ?? [];
  return (
    <div className="flex flex-wrap items-center gap-1">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="text-muted-foreground h-7 gap-1 px-2 text-xs"
            disabled={disabled}
          >
            <Link2 className="size-3.5" />
            {t("assistant.documents.link")}
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-80 space-y-3 text-sm">
          <p className="text-muted-foreground text-xs">
            {t("assistant.documents.hint")}
          </p>
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (url.trim()) link.mutate(url.trim());
            }}
          >
            <Input
              type="url"
              inputMode="url"
              placeholder="https://"
              aria-label={t("assistant.documents.link")}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
            <Button
              type="submit"
              disabled={link.isPending || !url.trim().startsWith("https://")}
            >
              {t("assistant.documents.add_link")}
            </Button>
          </form>
          {error ? (
            <p role="alert" className="text-destructive text-xs">
              {t(error)}
            </p>
          ) : null}
        </PopoverContent>
      </Popover>
      {items.length ? (
        <AttachmentGroup aria-label={t("assistant.documents.title")}>
          {items.map((d) => (
            <Attachment
              key={d.uuid}
              kind={d.kind === "link" ? "link" : "file"}
              title={
                d.masked
                  ? t("assistant.documents.masked")
                  : (d.source_url ?? d.name)
              }
            >
              <AttachmentTitle>{d.name}</AttachmentTitle>
              <AttachmentAction
                label={t("assistant.documents.remove", { name: d.name })}
                disabled={remove.isPending}
                onClick={() => remove.mutate(d.uuid)}
              />
            </Attachment>
          ))}
        </AttachmentGroup>
      ) : null}
    </div>
  );
}
