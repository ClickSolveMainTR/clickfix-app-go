"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Link2, Loader2, Paperclip, X } from "lucide-react";
import { useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { assistantService } from "../services/assistant.service";

const ACCEPT =
  ".pdf,application/pdf,.txt,.md,.markdown,.csv,.json,.yaml,.yml,.html,.htm,text/plain,text/markdown,text/csv,application/json,application/yaml,text/html";
const MAX_BYTES = 2 * 1024 * 1024;

function documentError(err: unknown): string {
  if (isApiError(err)) {
    switch (err.code) {
      case "ASSISTANT_DOCUMENT_TYPE":
        return "assistant.documents.errors.type";
      case "ASSISTANT_DOCUMENT_TOO_LARGE":
        return "assistant.documents.errors.size";
      case "ASSISTANT_DOCUMENT_LIMIT":
        return "assistant.documents.errors.limit";
      case "ASSISTANT_DOCUMENT_EMPTY":
        return "assistant.documents.errors.empty";
      case "ASSISTANT_DOCUMENT_FETCH":
        return "assistant.documents.errors.fetch";
      case "ASSISTANT_DOCUMENT_PDF":
        return "assistant.documents.errors.pdf";
    }
  }
  return "assistant.documents.errors.generic";
}

/**
 * Share an API document or a link with the assistant (CF-191). Only the
 * text is kept (7 days) with personal data and keys masked; the assistant
 * reads it as data and can prefill a confirmation card from it.
 */
export function DocumentAttach({
  slug,
  thread,
  ensureThread,
  disabled,
}: {
  slug: string;
  thread: string | null;
  ensureThread: () => Promise<string>;
  disabled: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);
  const key = ["assistant", slug, "documents", thread ?? ""] as const;

  const docs = useQuery({
    queryKey: key,
    queryFn: () => assistantService.documents(slug, thread ?? ""),
    enabled: Boolean(slug && thread),
  });

  const add = useMutation({
    mutationFn: async (input: { file?: File; url?: string }) => {
      const id = await ensureThread();
      if (input.file) {
        if (input.file.size > MAX_BYTES) throw new Error("size");
        return assistantService.uploadDocument(slug, id, input.file);
      }
      return assistantService.linkDocument(slug, id, input.url ?? "");
    },
    onMutate: () => setError(null),
    onSuccess: (_doc, input) => {
      if (input.url) setUrl("");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["assistant", slug, "documents"] });
    },
    onError: (err) =>
      setError(
        err instanceof Error && err.message === "size"
          ? "assistant.documents.errors.size"
          : documentError(err),
      ),
  });

  const remove = useMutation({
    mutationFn: (document: string) =>
      assistantService.removeDocument(slug, thread ?? "", document),
    onSettled: () => void qc.invalidateQueries({ queryKey: key }),
  });

  const items = docs.data?.items ?? [];
  const popover = (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          disabled={disabled || add.isPending}
          aria-label={t("assistant.documents.add")}
        >
          {add.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Paperclip className="size-4" />
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 space-y-3 text-sm">
        <p className="text-muted-foreground text-xs">
          {t("assistant.documents.hint")}
        </p>
        <input
          ref={fileRef}
          type="file"
          accept={ACCEPT}
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (file) add.mutate({ file });
          }}
        />
        <Button
          type="button"
          variant="outline"
          className="w-full"
          onClick={() => fileRef.current?.click()}
        >
          <FileText className="size-4" />
          {t("assistant.documents.file")}
        </Button>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (url.trim()) add.mutate({ url: url.trim() });
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
          <Button type="submit" disabled={!url.trim().startsWith("https://")}>
            {t("assistant.documents.add_link")}
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  );
  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-1">
        {popover}
        {items.length ? (
          <ul
            aria-label={t("assistant.documents.title")}
            className="flex flex-wrap gap-1"
          >
            {items.map((d) => (
              <li
                key={d.uuid}
                className="bg-muted flex max-w-full items-center gap-1 rounded-full px-2 py-0.5 text-xs"
                title={
                  d.masked
                    ? t("assistant.documents.masked")
                    : (d.source_url ?? d.name)
                }
              >
                {d.kind === "link" ? (
                  <Link2 className="size-3 shrink-0" />
                ) : (
                  <FileText className="size-3 shrink-0" />
                )}
                <span className="truncate">{d.name}</span>
                <button
                  type="button"
                  className="hover:text-foreground text-muted-foreground"
                  aria-label={t("assistant.documents.remove", { name: d.name })}
                  onClick={() => remove.mutate(d.uuid)}
                >
                  <X className="size-3" />
                </button>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
      {error ? (
        <p role="alert" className="text-destructive text-xs">
          {t(error)}
        </p>
      ) : null}
    </div>
  );
}
