/**
 * Custom fields (CF-7): reusable inputs, display and filters for any entity
 * with `custom_fields`, plus the settings and custom object screens.
 */
export { CustomFieldInput } from "./components/custom-field-input";
export type { CustomFieldInputProps } from "./components/custom-field-input";
export { CustomFieldValue } from "./components/custom-field-value";
export {
  CustomFieldsForm,
  applyCustomFieldApiErrors,
} from "./components/custom-fields-form";
export { CustomFieldFilterBuilder } from "./components/custom-field-filter-builder";
export { OptionBadge } from "./components/option-badge";
export {
  CustomFieldUserSourceProvider,
  useCustomFieldUserSource,
  type CustomFieldUserSource,
} from "./components/user-source";

export { CustomFieldsSettingsPage } from "./components/settings/custom-fields-settings-page";
export { CustomObjectsSettingsPage } from "./components/settings/custom-objects-settings-page";
export { ObjectRecordsPage } from "./components/records/object-records-page";

export {
  useCustomFieldDefinitions,
  useCustomFieldMeta,
  useCustomObjectTypes,
  useCustomObjectType,
  useCustomObjectRecords,
} from "./hooks/use-custom-fields";
export { customFieldsKeys } from "./hooks/query-keys";
export {
  customFieldsService,
  type CustomFieldScope,
} from "./services/custom-fields.service";
export { customFieldsSchema } from "./schemas/custom-fields-schema";

export {
  activeFieldKeys,
  conditionHolds,
  isEmptyValue,
} from "./lib/conditions";
export {
  buildFilterParams,
  customFieldSortParam,
  type CustomFieldFilterRow,
} from "./lib/filters";
export {
  buildCustomFieldsPayload,
  toInputValues,
  toApiValues,
} from "./lib/values";
export { customFieldErrors } from "./lib/errors";
export { validateCustomFieldValues } from "./lib/validate";
export * from "./types";
