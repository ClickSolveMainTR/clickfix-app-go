"use client";

import { X } from "lucide-react";
import { useMemo, useState, type KeyboardEvent } from "react";

import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useLocale } from "@/providers/locale-provider";

export const MAX_SKILLS = 30;
const SKILL_RE = /^[\p{L}\p{N}][\p{L}\p{N} ._+-]{0,39}$/u;

/** Same normalization as the API: trimmed and lower-cased. */
export function normalizeSkill(value: string): string {
  return value.trim().toLocaleLowerCase("tr-TR");
}

export function isValidSkill(value: string): boolean {
  return SKILL_RE.test(value);
}

/**
 * Tag input for skill labels: Enter or a comma adds the typed tag, chips
 * remove with ✕, and tags already used in the organization are offered as
 * suggestions.
 */
export function SkillsInput({
  id,
  value,
  onChange,
  suggestions = [],
  disabled,
  invalid,
}: {
  id?: string;
  value: string[];
  onChange: (next: string[]) => void;
  suggestions?: string[];
  disabled?: boolean;
  invalid?: boolean;
}) {
  const { t } = useLocale();
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  const add = (raw: string) => {
    const tag = normalizeSkill(raw);
    if (!tag) return;
    if (!isValidSkill(tag)) {
      setError(t("teams.validation.skill"));
      return;
    }
    if (value.includes(tag)) {
      setDraft("");
      return;
    }
    if (value.length >= MAX_SKILLS) {
      setError(t("teams.validation.too_many_skills"));
      return;
    }
    setError(null);
    setDraft("");
    onChange([...value, tag].sort((a, b) => a.localeCompare(b)));
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter" || event.key === ",") {
      event.preventDefault();
      add(draft);
    } else if (event.key === "Backspace" && !draft && value.length) {
      onChange(value.slice(0, -1));
    }
  };

  const filtered = useMemo(() => {
    const q = normalizeSkill(draft);
    return suggestions
      .filter((tag) => !value.includes(tag) && (!q || tag.includes(q)))
      .slice(0, 10);
  }, [draft, suggestions, value]);

  return (
    <div className="space-y-2">
      <div
        className="border-input focus-within:ring-ring/50 aria-[invalid=true]:border-destructive flex min-h-9 flex-wrap items-center gap-1.5 rounded-md border px-2 py-1.5 focus-within:ring-[3px]"
        aria-invalid={invalid || Boolean(error)}
      >
        {value.map((tag) => (
          <Badge key={tag} variant="secondary" className="gap-1">
            {tag}
            {disabled ? null : (
              <button
                type="button"
                className="opacity-60 hover:opacity-100"
                aria-label={`${tag} ✕`}
                onClick={() => onChange(value.filter((v) => v !== tag))}
              >
                <X className="size-3" />
              </button>
            )}
          </Badge>
        ))}
        <Input
          id={id}
          value={draft}
          disabled={disabled}
          placeholder={value.length ? "" : t("teams.fields.skills_placeholder")}
          onChange={(event) => {
            setError(null);
            setDraft(event.target.value.replace(",", ""));
          }}
          onKeyDown={onKeyDown}
          onBlur={() => add(draft)}
          className="h-7 min-w-32 flex-1 border-0 px-1 shadow-none focus-visible:ring-0"
        />
      </div>
      {error ? <p className="text-destructive text-xs">{error}</p> : null}
      {!disabled && filtered.length ? (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-muted-foreground text-xs">
            {t("teams.fields.skills_suggestions")}:
          </span>
          {filtered.map((tag) => (
            <button
              key={tag}
              type="button"
              onClick={() => add(tag)}
              className="hover:bg-accent rounded-full border px-2 py-0.5 text-xs"
            >
              + {tag}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
