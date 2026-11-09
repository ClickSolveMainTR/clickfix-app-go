"use client";

import { Plus, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useLocale } from "@/providers/locale-provider";

import { useCustomFieldDefinitions } from "../hooks/use-custom-fields";
import {
  MATCH_ENTITIES,
  type CustomFieldDefinition,
  type CustomFieldMatch,
  type MatchEntity,
} from "../types";

/** Live list fields (select / multiselect) of each catalog entity. */
export function useCatalogListFields(
  entities: readonly MatchEntity[] = MATCH_ENTITIES,
) {
  const contact = useCustomFieldDefinitions(
    { entityType: "contact" },
    { enabled: entities.includes("contact") },
  );
  const company = useCustomFieldDefinitions(
    { entityType: "company" },
    { enabled: entities.includes("company") },
  );
  const conversation = useCustomFieldDefinitions(
    { entityType: "conversation" },
    { enabled: entities.includes("conversation") },
  );
  return useMemo(() => {
    const list = (defs: CustomFieldDefinition[] | undefined) =>
      (defs ?? []).filter(
        (d) => !d.archived && (d.type === "select" || d.type === "multiselect"),
      );
    return {
      contact: list(contact.data),
      company: list(company.data),
      conversation: list(conversation.data),
      isLoading:
        contact.isLoading || company.isLoading || conversation.isLoading,
    } satisfies Record<MatchEntity, CustomFieldDefinition[]> & {
      isLoading: boolean;
    };
  }, [
    company.data,
    company.isLoading,
    contact.data,
    contact.isLoading,
    conversation.data,
    conversation.isLoading,
  ]);
}

type CatalogMatchEditorProps = {
  value: CustomFieldMatch[];
  onChange: (next: CustomFieldMatch[]) => void;
  /** Entities offered (articles: contact and company). */
  entities?: readonly MatchEntity[];
  disabled?: boolean;
  id?: string;
};

/**
 * Picks list custom field options from the single catalog (CF-196). Rules
 * store option ids, so renaming an option label never breaks them; SLA
 * policies, article audiences and workflow conditions share this editor.
 */
export function CatalogMatchEditor({
  value,
  onChange,
  entities = MATCH_ENTITIES,
  disabled,
  id,
}: CatalogMatchEditorProps) {
  const { t } = useLocale();
  const catalog = useCatalogListFields(entities);
  const fieldsOf = (entity: string) =>
    (catalog as unknown as Record<string, CustomFieldDefinition[]>)[entity] ??
    [];
  const any = entities.some((e) => fieldsOf(e).length > 0);

  const set = (i: number, patch: Partial<CustomFieldMatch>) =>
    onChange(value.map((m, j) => (j === i ? { ...m, ...patch } : m)));
  const add = () => {
    const entity = entities.find((e) => fieldsOf(e).length > 0) ?? entities[0];
    const first = fieldsOf(entity)[0];
    onChange([...value, { entity, key: first?.key ?? "", options: [] }]);
  };

  if (!catalog.isLoading && !any && value.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("custom_fields.catalog.no_list_fields")}
      </p>
    );
  }

  return (
    <div id={id} className="flex flex-col gap-3">
      {value.map((m, i) => {
        const fields = fieldsOf(m.entity);
        const def = fields.find((f) => f.key === m.key);
        return (
          <div
            key={`${m.entity}-${m.key}-${i}`}
            className="flex flex-col gap-2 rounded-md border p-3"
          >
            <div className="flex flex-wrap items-center gap-2">
              <Select
                value={m.entity}
                disabled={disabled}
                onValueChange={(entity) =>
                  set(i, {
                    entity: entity as MatchEntity,
                    key: fieldsOf(entity)[0]?.key ?? "",
                    options: [],
                  })
                }
              >
                <SelectTrigger
                  className="w-36"
                  aria-label={t("custom_fields.catalog.entity")}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {entities.map((e) => (
                    <SelectItem key={e} value={e}>
                      {t(`custom_fields.entities.${e}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select
                value={m.key}
                disabled={disabled || !fields.length}
                onValueChange={(key) => set(i, { key, options: [] })}
              >
                <SelectTrigger
                  className="w-48"
                  aria-label={t("custom_fields.catalog.field")}
                >
                  <SelectValue
                    placeholder={t("custom_fields.catalog.pick_field")}
                  />
                </SelectTrigger>
                <SelectContent>
                  {fields.map((f) => (
                    <SelectItem key={f.key} value={f.key}>
                      {f.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                disabled={disabled}
                aria-label={t("custom_fields.catalog.remove")}
                onClick={() => onChange(value.filter((_, j) => j !== i))}
              >
                <Trash2 />
              </Button>
            </div>
            {def ? (
              <ToggleGroup
                type="multiple"
                variant="outline"
                size="sm"
                spacing={1}
                className="flex-wrap"
                disabled={disabled}
                value={m.options}
                onValueChange={(options) => set(i, { options })}
              >
                {def.options
                  .filter((o) => !o.archived || m.options.includes(o.value))
                  .map((o) => (
                    <ToggleGroupItem key={o.value} value={o.value}>
                      {o.label}
                    </ToggleGroupItem>
                  ))}
              </ToggleGroup>
            ) : m.key ? (
              <p className="text-destructive text-xs">
                {t("custom_fields.catalog.missing_field", { key: m.key })}
              </p>
            ) : null}
          </div>
        );
      })}
      <div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || !any}
          onClick={add}
        >
          <Plus />
          {t("custom_fields.catalog.add")}
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        {t("custom_fields.catalog.hint")}
      </p>
    </div>
  );
}
