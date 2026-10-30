"use client";

import { X } from "lucide-react";
import { useState, type KeyboardEvent, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Free-text chips (categories, handoff keywords). Enter or comma adds a
 * value, Backspace on an empty input removes the last one; every chip has a
 * labelled remove button so the list stays keyboard accessible.
 */
export function TagInput({
  id,
  value,
  onChange,
  placeholder,
  max,
  maxLength = 100,
  disabled,
}: {
  id?: string;
  value: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  max: number;
  maxLength?: number;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const [draft, setDraft] = useState("");
  const add = (raw: string) => {
    const next = raw.trim().slice(0, maxLength);
    if (!next || value.includes(next) || value.length >= max) return;
    onChange([...value, next]);
  };
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter" || event.key === ",") {
      event.preventDefault();
      add(draft);
      setDraft("");
    } else if (event.key === "Backspace" && draft === "" && value.length) {
      onChange(value.slice(0, -1));
    }
  };
  return (
    <div className="space-y-2">
      {value.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {value.map((item) => (
            <li key={item}>
              <Badge variant="secondary" className="gap-1 pe-1">
                <span className="max-w-[16rem] truncate">{item}</span>
                {!disabled && (
                  <button
                    type="button"
                    className="hover:bg-foreground/10 focus-visible:ring-ring rounded-sm p-0.5 focus-visible:ring-2 focus-visible:outline-none"
                    aria-label={t("ai_settings.tags.remove", { value: item })}
                    onClick={() => onChange(value.filter((v) => v !== item))}
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
        disabled={disabled || value.length >= max}
        placeholder={
          value.length >= max ? t("ai_settings.tags.full") : placeholder
        }
        maxLength={maxLength}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={onKeyDown}
        onBlur={() => {
          add(draft);
          setDraft("");
        }}
      />
      <p className="text-muted-foreground text-xs">
        {t("ai_settings.tags.hint", { count: value.length, max })}
      </p>
    </div>
  );
}

/** Label + hint + control block used across the AI settings forms. */
export function SettingRow({
  label,
  hint,
  htmlFor,
  children,
  className,
}: {
  label: string;
  hint?: ReactNode;
  htmlFor?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("space-y-2", className)}>
      <label htmlFor={htmlFor} className="text-sm leading-none font-medium">
        {label}
      </label>
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
      {children}
    </div>
  );
}

/** Loading / error placeholder for a react-query result. */
export function QueryState({
  loading,
  error,
  onRetry,
}: {
  loading: boolean;
  error: boolean;
  onRetry: () => void;
}) {
  const { t } = useLocale();
  if (loading) return <Loading label={t("common.loading")} />;
  if (error)
    return (
      <ErrorState
        title={t("ai_settings.errors.load_title")}
        description={t("ai_settings.errors.load_description")}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  return null;
}

/** Small key/value statistic (tokens, cost, latency). */
export function Stat({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="bg-muted/40 min-w-0 rounded-md border px-3 py-2">
      <dt className="text-muted-foreground truncate text-xs">{label}</dt>
      <dd className="truncate text-sm font-medium tabular-nums">{value}</dd>
    </div>
  );
}
