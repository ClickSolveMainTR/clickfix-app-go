"use client";

import {
  CustomFieldValue,
  isEmptyValue,
  useCustomFieldDefinitions,
} from "@/features/custom-fields";
import { useLocale } from "@/providers/locale-provider";

import {
  useCompany,
  useContact,
  useContactsScope,
} from "../hooks/use-contacts";

type Values = Record<string, unknown> | undefined;

/**
 * CF-195: the accepted custom field values of one entity, by the labels of
 * its live definitions. Keys without a definition are not shown. With
 * `hideEmpty` only filled fields are listed (side panels).
 */
export function CustomFieldValuesList({
  entityType,
  values,
  hideEmpty,
  title,
}: {
  entityType: "contact" | "company";
  values: Values;
  hideEmpty?: boolean;
  title?: string;
}) {
  const defs = useCustomFieldDefinitions({ entityType });
  const active = (defs.data ?? []).filter(
    (d) => !d.archived && (!hideEmpty || !isEmptyValue(values?.[d.key])),
  );
  if (active.length === 0) return null;
  return (
    <div className="grid gap-1.5">
      {title && (
        <h4 className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
          {title}
        </h4>
      )}
      <dl className="grid gap-1.5 text-xs">
        {active.map((d) => (
          <div key={d.uuid} className="grid grid-cols-2 gap-2">
            <dt className="text-muted-foreground truncate">{d.label}</dt>
            <dd className="min-w-0">
              <CustomFieldValue definition={d} value={values?.[d.key]} />
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

/**
 * CF-195: the contact's and its company's external ids and accepted custom
 * field values, for the conversation side panel. Needs
 * `tenant.contacts.read`; without it nothing is shown.
 */
export function CustomerFieldsPanel({
  contactUuid,
  companyUuid,
}: {
  contactUuid: string;
  companyUuid?: string | null;
}) {
  const { t } = useLocale();
  const { canRead } = useContactsScope();
  const contact = useContact(canRead ? contactUuid : null);
  const company = useCompany(canRead ? (companyUuid ?? null) : null);
  if (!canRead) return null;
  const contactValues = contact.data?.custom_fields as Values;
  const companyValues = company.data?.custom_fields as Values;
  return (
    <div className="flex flex-col gap-3 empty:hidden">
      {company.data?.external_id && (
        <p className="text-muted-foreground text-xs">
          {t("contacts.external_id.company")}:{" "}
          <span className="text-foreground font-mono">
            {company.data.external_id}
          </span>
        </p>
      )}
      <CustomFieldValuesList
        entityType="contact"
        values={contactValues}
        hideEmpty
        title={t("contacts.custom_fields.contact")}
      />
      {companyUuid && (
        <CustomFieldValuesList
          entityType="company"
          values={companyValues}
          hideEmpty
          title={t("contacts.custom_fields.company")}
        />
      )}
    </div>
  );
}
