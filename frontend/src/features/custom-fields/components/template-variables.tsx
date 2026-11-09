"use client";

import { useMemo } from "react";

import { useLocale } from "@/providers/locale-provider";

import { useCustomFieldDefinitions } from "../hooks/use-custom-fields";
import type { CustomFieldDefinition } from "../types";

type Audience = "agent" | "ai";

/** `{{contact.<key>}}` / `{{company.<key>}}` of the fields an audience reads. */
export function useTemplateVariables(audience: Audience) {
  const contact = useCustomFieldDefinitions({ entityType: "contact" });
  const company = useCustomFieldDefinitions({ entityType: "company" });
  return useMemo(() => {
    const visible = (d: CustomFieldDefinition) =>
      !d.archived && !d.sensitive && d.visibility?.[audience] !== false;
    return [
      ...(contact.data ?? []).filter(visible).map((d) => ({
        token: `{{contact.${d.key}}}`,
        label: d.label,
      })),
      ...(company.data ?? []).filter(visible).map((d) => ({
        token: `{{company.${d.key}}}`,
        label: d.label,
      })),
    ];
  }, [audience, company.data, contact.data]);
}

/**
 * Custom field variables as clickable chips (CF-196). List fields render
 * their option label; sensitive fields are never offered.
 */
export function TemplateVariableChips({
  audience,
  onPick,
}: {
  audience: Audience;
  onPick: (token: string) => void;
}) {
  const { t } = useLocale();
  const vars = useTemplateVariables(audience);
  if (!vars.length) return null;
  return (
    <div className="flex flex-wrap items-center gap-1">
      <span className="text-muted-foreground text-xs">
        {t("custom_fields.catalog.variables")}
      </span>
      {vars.map((v) => (
        <button
          type="button"
          key={v.token}
          title={v.label}
          className="bg-muted hover:bg-muted/70 rounded px-1.5 py-0.5 font-mono text-[11px]"
          onClick={() => onPick(v.token)}
        >
          {v.token}
        </button>
      ))}
    </div>
  );
}
