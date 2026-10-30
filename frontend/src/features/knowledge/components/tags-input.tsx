"use client";

import { X } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useLocale } from "@/providers/locale-provider";

const MAX_TAGS = 30;
const MAX_TAG_LENGTH = 100;

/** Free tags: Enter or comma adds, Backspace on an empty field removes the last. */
export function TagsInput({
  id,
  value,
  onChange,
  disabled,
}: {
  id: string;
  value: string[];
  onChange: (tags: string[]) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const [draft, setDraft] = useState("");
  const add = (...raw: string[]) => {
    const next = [...value];
    for (const r of raw) {
      const tag = r.trim().slice(0, MAX_TAG_LENGTH);
      if (tag && !next.includes(tag) && next.length < MAX_TAGS) next.push(tag);
    }
    if (next.length !== value.length) onChange(next);
  };

  return (
    <div className="space-y-2">
      {value.length > 0 && (
        <ul
          className="flex flex-wrap gap-1.5"
          aria-label={t("knowledge.fields.tags")}
        >
          {value.map((tag) => (
            <li key={tag}>
              <Badge variant="secondary" className="gap-1 pr-1">
                {tag}
                {!disabled && (
                  <button
                    type="button"
                    className="hover:bg-foreground/10 focus-visible:ring-ring rounded-sm p-0.5 outline-none focus-visible:ring-2"
                    aria-label={t("knowledge.tags.remove", { tag })}
                    onClick={() => onChange(value.filter((v) => v !== tag))}
                  >
                    <X className="size-3" />
                  </button>
                )}
              </Badge>
            </li>
          ))}
        </ul>
      )}
      <Input
        id={id}
        value={draft}
        disabled={disabled || value.length >= MAX_TAGS}
        placeholder={t("knowledge.tags.placeholder")}
        onChange={(e) => {
          const next = e.target.value;
          if (next.includes(",")) {
            add(...next.split(",").slice(0, -1));
            setDraft(next.split(",").at(-1) ?? "");
          } else {
            setDraft(next);
          }
        }}
        onBlur={() => {
          add(draft);
          setDraft("");
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            add(draft);
            setDraft("");
          } else if (e.key === "Backspace" && draft === "" && value.length) {
            onChange(value.slice(0, -1));
          }
        }}
      />
    </div>
  );
}
