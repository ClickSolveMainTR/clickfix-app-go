import { tenantQueryRequest } from "@/features/custom-fields/lib/tenant-query-request";

import type {
  CustomFieldCreateRequest,
  CustomFieldDefinition,
  CustomFieldEntityType,
  CustomFieldMeta,
  CustomFieldReorderRequest,
  CustomFieldUpdateRequest,
  CustomObjectRecord,
  CustomObjectRecordPage,
  CustomObjectRecordRequest,
  CustomObjectType,
  CustomObjectTypeRequest,
  CustomFieldEntityValues,
  CustomFieldUnknownKey,
} from "../types";

/** Which entity's definitions: a built-in entity or one custom object type. */
export type CustomFieldScope = {
  entityType: CustomFieldEntityType;
  objectTypeUuid?: string | null;
};

export type DefinitionListParams = CustomFieldScope & {
  includeArchived?: boolean;
};

export type RecordListParams = {
  limit: number;
  offset: number;
  q?: string;
  sort?: string;
  /** `cf.*` filter parameters (see `buildFilterParams`). */
  filters?: Record<string, string>;
};

const BASE = "/v1/tenant/custom-fields";
const OBJECTS = "/v1/tenant/custom-objects";

export const customFieldsService = {
  meta() {
    return tenantQueryRequest<CustomFieldMeta>("GET", `${BASE}/meta`);
  },

  async listDefinitions(params: DefinitionListParams) {
    const data = await tenantQueryRequest<{ items: CustomFieldDefinition[] }>(
      "GET",
      BASE,
      {
        query: {
          entity_type: params.entityType,
          object_type_uuid: params.objectTypeUuid ?? undefined,
          include_archived: params.includeArchived ? "true" : undefined,
        },
      },
    );
    return data.items ?? [];
  },

  getDefinition(uuid: string) {
    return tenantQueryRequest<CustomFieldDefinition>("GET", `${BASE}/${uuid}`);
  },

  createDefinition(body: CustomFieldCreateRequest) {
    return tenantQueryRequest<CustomFieldDefinition>("POST", BASE, {
      body,
      silent: true,
    });
  },

  updateDefinition(uuid: string, body: CustomFieldUpdateRequest) {
    return tenantQueryRequest<CustomFieldDefinition>(
      "PATCH",
      `${BASE}/${uuid}`,
      {
        body,
        silent: true,
      },
    );
  },

  archiveDefinition(uuid: string) {
    return tenantQueryRequest<CustomFieldDefinition>(
      "DELETE",
      `${BASE}/${uuid}`,
    );
  },

  restoreDefinition(uuid: string) {
    return tenantQueryRequest<CustomFieldDefinition>(
      "POST",
      `${BASE}/${uuid}/restore`,
    );
  },

  async reorder(body: CustomFieldReorderRequest) {
    const data = await tenantQueryRequest<{ items?: CustomFieldDefinition[] }>(
      "PUT",
      `${BASE}/order`,
      { body },
    );
    return data?.items ?? [];
  },

  /** Undefined keys the widget sent (CF-196), optionally of one widget. */
  async listUnknownKeys(channelUuid?: string | null) {
    const data = await tenantQueryRequest<{ items: CustomFieldUnknownKey[] }>(
      "GET",
      `${BASE}/unknown-keys`,
      { query: { channel_uuid: channelUuid ?? undefined } },
    );
    return data.items ?? [];
  },

  dismissUnknownKey(uuid: string) {
    return tenantQueryRequest<{ dismissed: boolean }>(
      "POST",
      `${BASE}/unknown-keys/${uuid}/dismiss`,
    );
  },

  /** Agent view of one contact's or company's fields (CF-196). */
  values(entityType: "contact" | "company", entityUuid: string) {
    return tenantQueryRequest<CustomFieldEntityValues>(
      "GET",
      `${BASE}/values`,
      { query: { entity_type: entityType, entity_uuid: entityUuid } },
    );
  },

  async listObjectTypes() {
    const data = await tenantQueryRequest<{ items: CustomObjectType[] }>(
      "GET",
      OBJECTS,
    );
    return data.items ?? [];
  },

  getObjectType(uuid: string) {
    return tenantQueryRequest<CustomObjectType>("GET", `${OBJECTS}/${uuid}`);
  },

  createObjectType(body: CustomObjectTypeRequest) {
    return tenantQueryRequest<CustomObjectType>("POST", OBJECTS, {
      body,
      silent: true,
    });
  },

  updateObjectType(uuid: string, body: CustomObjectTypeRequest) {
    return tenantQueryRequest<CustomObjectType>("PATCH", `${OBJECTS}/${uuid}`, {
      body,
      silent: true,
    });
  },

  listRecords(typeUuid: string, params: RecordListParams) {
    return tenantQueryRequest<CustomObjectRecordPage>(
      "GET",
      `${OBJECTS}/${typeUuid}/records`,
      {
        query: {
          limit: params.limit,
          offset: params.offset,
          q: params.q,
          sort: params.sort,
          ...params.filters,
        },
      },
    );
  },

  getRecord(typeUuid: string, recordUuid: string) {
    return tenantQueryRequest<CustomObjectRecord>(
      "GET",
      `${OBJECTS}/${typeUuid}/records/${recordUuid}`,
    );
  },

  createRecord(typeUuid: string, body: CustomObjectRecordRequest) {
    return tenantQueryRequest<CustomObjectRecord>(
      "POST",
      `${OBJECTS}/${typeUuid}/records`,
      { body, silent: true },
    );
  },

  updateRecord(
    typeUuid: string,
    recordUuid: string,
    body: CustomObjectRecordRequest,
  ) {
    return tenantQueryRequest<CustomObjectRecord>(
      "PATCH",
      `${OBJECTS}/${typeUuid}/records/${recordUuid}`,
      { body, silent: true },
    );
  },

  deleteRecord(typeUuid: string, recordUuid: string) {
    return tenantQueryRequest<{ deleted: boolean }>(
      "DELETE",
      `${OBJECTS}/${typeUuid}/records/${recordUuid}`,
    );
  },
};
