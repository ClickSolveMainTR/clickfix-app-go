"use client";

import { CircleAlert, Plus, X } from "lucide-react";
import { useMemo, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

import { useDismissUnknownKey } from "../hooks/use-custom-field-mutations";
import {
  useCustomFieldDefinitions,
  useCustomFieldValues,
  useUnknownCustomFieldKeys,
} from "../hooks/use-custom-fields";
import type { DefinitionDraft } from "../lib/definition-draft";
import type { CustomFieldType } from "../types";
import { FieldDefinitionDialog } from "./settings/field-definition-dialog";

/** Field type suggested for a received value kind. */
const KIND_TYPES: Record<string, CustomFieldType> = {
  text: "text",
  number: "number",
  boolean: "boolean",
  date: "date",
};

type Pending = { key: string; suggestedKey: string; kind: string };

function labelOf(key: string) {
  const words = key.replace(/[_-]+/g, " ").trim();
  return words ? words[0].toLocaleUpperCase("tr") + words.slice(1) : key;
}

/** Opens the field dialog prefilled from one undefined key. */
function useDefineFromKey() {
  const [pending, setPending] = useState<Pending | null>(null);
  const siblings = useCustomFieldDefinitions(
    { entityType: "contact", includeArchived: true },
    { enabled: Boolean(pending) },
  );
  const initial = useMemo<Partial<DefinitionDraft> | undefined>(
    () =>
      pending
        ? {
            key: pending.suggestedKey,
            label: labelOf(pending.suggestedKey || pending.key),
            type: KIND_TYPES[pending.kind] ?? "text",
            fromUnknownKey: pending.key,
          }
        : undefined,
    [pending],
  );
  const dialog = (
    <FieldDefinitionDialog
      open={Boolean(pending)}
      onOpenChange={(open) => !open && setPending(null)}
      scope={{ entityType: "contact" }}
      siblings={siblings.data ?? []}
      initial={initial}
    />
  );
  return { define: setPending, dialog };
}

type UnknownFieldsAlertProps = {
  /** Only the keys one widget sent (channel setup check). */
  channelUuid?: string | null;
  className?: string;
};

/**
 * "Tanımsız alan geldi" warnings of the organization or one widget (CF-196):
 * keys the widget sent that no contact field defines. One click opens the
 * field dialog prefilled with the key; creating the field closes the warning
 * and applies the values contacts already sent.
 */
export function UnknownFieldsAlert({
  channelUuid,
  className,
}: UnknownFieldsAlertProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.customFields.manage);
  const canRead = can(permissions.tenant.customFields.read);
  const keys = useUnknownCustomFieldKeys(channelUuid, { enabled: canRead });
  const dismiss = useDismissUnknownKey();
  const { define, dialog } = useDefineFromKey();
  const items = keys.data ?? [];
  if (!items.length) return null;

  return (
    <>
      <Alert className={className}>
        <CircleAlert />
        <AlertTitle>
          {t("custom_fields.unknown.title", { count: items.length })}
        </AlertTitle>
        <AlertDescription>
          <p>{t("custom_fields.unknown.description")}</p>
          <ul className="mt-2 flex w-full flex-col gap-2">
            {items.map((k) => (
              <li
                key={k.uuid}
                className="flex flex-wrap items-center justify-between gap-2"
              >
                <span className="flex flex-wrap items-center gap-2">
                  <span>{t("custom_fields.unknown.received")}</span>
                  <code className="bg-muted rounded px-1.5 py-0.5 font-mono text-xs">
                    {k.key}
                  </code>
                  <Badge variant="outline">
                    {t(`custom_fields.unknown.kind.${k.value_kind}`)}
                  </Badge>
                  <span className="text-muted-foreground text-xs">
                    {t("custom_fields.unknown.hits", { count: k.hits })}
                    {k.channel_name ? ` · ${k.channel_name}` : ""}
                  </span>
                </span>
                {canManage ? (
                  <span className="flex gap-1">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        define({
                          key: k.key,
                          suggestedKey: k.suggested_key,
                          kind: k.value_kind,
                        })
                      }
                    >
                      <Plus />
                      {t("custom_fields.unknown.define")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={dismiss.isPending}
                      onClick={() => dismiss.mutate(k.uuid)}
                      aria-label={t("custom_fields.unknown.dismiss")}
                    >
                      <X />
                    </Button>
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        </AlertDescription>
      </Alert>
      {dialog}
    </>
  );
}

type ContactUnknownFieldsProps = {
  contactUuid: string;
  className?: string;
};

/**
 * Contact card warning (CF-196): the undefined keys this contact received,
 * each with a one-click definition. Mount it on the contact card / the
 * conversation panel; it renders nothing when there are none.
 */
export function ContactUnknownFields({
  contactUuid,
  className,
}: ContactUnknownFieldsProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.customFields.manage);
  const values = useCustomFieldValues("contact", contactUuid, {
    enabled: can(permissions.tenant.customFields.read),
  });
  const { define, dialog } = useDefineFromKey();
  const unknown = values.data?.unknown ?? [];
  if (!unknown.length) return null;

  return (
    <>
      <Alert className={className}>
        <CircleAlert />
        <AlertTitle>
          {t("custom_fields.unknown.contact_title", { count: unknown.length })}
        </AlertTitle>
        <AlertDescription>
          <ul className="flex w-full flex-col gap-1.5">
            {unknown.map((u) => (
              <li
                key={u.key}
                className="flex flex-wrap items-center justify-between gap-2"
              >
                <span className="flex flex-wrap items-center gap-1.5">
                  <span>{t("custom_fields.unknown.received")}</span>
                  <code className="bg-muted rounded px-1.5 py-0.5 font-mono text-xs">
                    {u.key}
                  </code>
                  <span className="text-muted-foreground text-xs">
                    {String(u.value ?? "")}
                  </span>
                </span>
                {canManage ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      define({
                        key: u.key,
                        suggestedKey: u.suggested_key,
                        kind:
                          typeof u.value === "number"
                            ? "number"
                            : typeof u.value === "boolean"
                              ? "boolean"
                              : /^\d{4}-\d{2}-\d{2}/.test(String(u.value))
                                ? "date"
                                : "text",
                      })
                    }
                  >
                    <Plus />
                    {t("custom_fields.unknown.define")}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        </AlertDescription>
      </Alert>
      {dialog}
    </>
  );
}
