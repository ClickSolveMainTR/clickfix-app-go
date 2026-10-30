import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type CustomFieldEntityType = Schemas["CustomFieldEntityType"];
export type CustomFieldType = Schemas["CustomFieldType"];
export type CustomFieldOperator = Schemas["CustomFieldOperator"];
export type CustomFieldOption = Schemas["CustomFieldOption"];
export type CustomFieldConfig = Schemas["CustomFieldConfig"];
export type CustomFieldCondition = Schemas["CustomFieldCondition"];
export type CustomFieldConditionOperator = CustomFieldCondition["operator"];
export type CustomFieldDefinition = Schemas["CustomFieldDefinition"];
export type CustomFieldCreateRequest = Schemas["CustomFieldCreateRequest"];
export type CustomFieldUpdateRequest = Schemas["CustomFieldUpdateRequest"];
export type CustomFieldReorderRequest = Schemas["CustomFieldReorderRequest"];
export type CustomFieldTypeMeta = Schemas["CustomFieldTypeMeta"];
export type CustomFieldMeta = Schemas["CustomFieldMeta"];
export type CustomObjectType = Schemas["CustomObjectType"];
export type CustomObjectTypeRequest = Schemas["CustomObjectTypeRequest"];
export type CustomObjectRecord = Schemas["CustomObjectRecord"];
export type CustomObjectRecordRequest = Schemas["CustomObjectRecordRequest"];
export type CustomObjectRecordPage = Schemas["CustomObjectRecordPage"];

/** Stored money value. */
export type MoneyValue = { amount_minor: number; currency: string };

/** Values keyed by field key, as stored in `custom_fields`. */
export type CustomFieldValues = Record<string, unknown>;

/** Built-in entity types that carry custom fields (custom objects excluded). */
export const BUILTIN_ENTITY_TYPES = [
  "contact",
  "company",
  "conversation",
  "ticket",
] as const satisfies readonly CustomFieldEntityType[];

export const CUSTOM_FIELD_TYPES = [
  "text",
  "textarea",
  "number",
  "money",
  "date",
  "boolean",
  "select",
  "multiselect",
  "url",
  "email",
  "phone",
  "user",
] as const satisfies readonly CustomFieldType[];

export const CONDITION_OPERATORS = [
  "equals",
  "not_equals",
  "in",
  "is_set",
  "is_not_set",
] as const satisfies readonly CustomFieldConditionOperator[];
