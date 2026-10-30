import type {
  DefinitionListParams,
  RecordListParams,
} from "../services/custom-fields.service";

/** Every key starts with the organization slug so a switch refetches. */
export const customFieldsKeys = {
  all: (org: string | null) => ["tenant", org, "custom-fields"] as const,
  meta: (org: string | null) => [...customFieldsKeys.all(org), "meta"] as const,
  definitions: (org: string | null) =>
    [...customFieldsKeys.all(org), "definitions"] as const,
  definitionList: (org: string | null, params: DefinitionListParams) =>
    [
      ...customFieldsKeys.definitions(org),
      params.entityType,
      params.objectTypeUuid ?? null,
      Boolean(params.includeArchived),
    ] as const,
  objectTypes: (org: string | null) =>
    ["tenant", org, "custom-objects"] as const,
  objectType: (org: string | null, uuid: string) =>
    [...customFieldsKeys.objectTypes(org), "detail", uuid] as const,
  records: (org: string | null, typeUuid: string) =>
    [...customFieldsKeys.objectTypes(org), "records", typeUuid] as const,
  recordList: (
    org: string | null,
    typeUuid: string,
    params: RecordListParams,
  ) => [...customFieldsKeys.records(org, typeUuid), "list", params] as const,
};
