"use client";

import { Check, Minus, X } from "lucide-react";

import { currency as formatCurrency, date as formatDate } from "@/lib/utils";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { isEmptyValue } from "../lib/conditions";
import { isMoneyValue } from "../lib/values";
import type { CustomFieldDefinition } from "../types";
import { OptionBadge } from "./option-badge";
import { useCustomFieldUserSource } from "./user-source";

type CustomFieldValueProps = {
  definition: Pick<CustomFieldDefinition, "type" | "options" | "config">;
  /** Stored value (as the API returns it). */
  value: unknown;
  className?: string;
  /** Shown for empty values. Default: an em dash. */
  emptyText?: string;
};

const linkClass = "text-primary underline-offset-2 hover:underline break-all";

/** Read-only display of a stored custom field value. */
export function CustomFieldValue({
  definition,
  value,
  className,
  emptyText = "—",
}: CustomFieldValueProps) {
  const { t, locale } = useLocale();
  const users = useCustomFieldUserSource();

  if (isEmptyValue(value)) {
    return (
      <span className={cn("text-muted-foreground", className)}>
        {emptyText}
      </span>
    );
  }

  const options = definition.options ?? [];
  const archivedLabel = t("custom_fields.value.archived");
  const optionFor = (v: string) =>
    options.find((o) => o.value === v) ?? {
      value: v,
      label: v,
      archived: true,
    };

  switch (definition.type) {
    case "select":
      return (
        <OptionBadge
          option={optionFor(String(value))}
          archivedLabel={archivedLabel}
          className={className}
        />
      );
    case "multiselect": {
      const list = Array.isArray(value) ? value.map(String) : [String(value)];
      return (
        <span className={cn("inline-flex flex-wrap gap-1", className)}>
          {list.map((v) => (
            <OptionBadge
              key={v}
              option={optionFor(v)}
              archivedLabel={archivedLabel}
            />
          ))}
        </span>
      );
    }
    case "boolean":
      return value === true ? (
        <span className={cn("inline-flex items-center gap-1", className)}>
          <Check className="size-3.5 text-emerald-600" aria-hidden />
          {t("custom_fields.value.yes")}
        </span>
      ) : value === false ? (
        <span
          className={cn(
            "text-muted-foreground inline-flex items-center gap-1",
            className,
          )}
        >
          <X className="size-3.5" aria-hidden />
          {t("custom_fields.value.no")}
        </span>
      ) : (
        <Minus className="text-muted-foreground size-3.5" />
      );
    case "money": {
      if (!isMoneyValue(value)) return <span className={className}>—</span>;
      let text: string;
      try {
        text = formatCurrency(value.amount_minor / 100, value.currency, locale);
      } catch {
        text = `${(value.amount_minor / 100).toFixed(2)} ${value.currency}`;
      }
      return <span className={cn("tabular-nums", className)}>{text}</span>;
    }
    case "number": {
      const n = typeof value === "number" ? value : Number(value);
      const precision = definition.config?.precision;
      const text = Number.isFinite(n)
        ? new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
            maximumFractionDigits: precision ?? 6,
          }).format(n)
        : String(value);
      return <span className={cn("tabular-nums", className)}>{text}</span>;
    }
    case "date": {
      let text = String(value);
      try {
        text = formatDate(String(value), "d MMM yyyy", locale);
      } catch {
        // keep the raw value
      }
      return (
        <time dateTime={String(value)} className={className}>
          {text}
        </time>
      );
    }
    case "url":
      return (
        <a
          href={String(value)}
          target="_blank"
          rel="noopener noreferrer"
          className={cn(linkClass, className)}
        >
          {String(value).replace(/^https?:\/\//, "")}
        </a>
      );
    case "email":
      return (
        <a
          href={`mailto:${String(value)}`}
          className={cn(linkClass, className)}
        >
          {String(value)}
        </a>
      );
    case "phone":
      return (
        <a href={`tel:${String(value)}`} className={cn(linkClass, className)}>
          {String(value)}
        </a>
      );
    case "user": {
      const uuid = String(value);
      const label = users.resolveLabel(uuid);
      return (
        <span className={className} title={uuid}>
          {label ?? (
            <code className="text-muted-foreground text-xs">
              {uuid.slice(0, 8)}
            </code>
          )}
        </span>
      );
    }
    case "textarea":
      return (
        <span className={cn("whitespace-pre-line", className)}>
          {String(value)}
        </span>
      );
    default:
      return <span className={className}>{String(value)}</span>;
  }
}
