"use client";

import { useQuery } from "@tanstack/react-query";

import { useActiveOrganization } from "@/features/organizations/lib/use-active-organization";

import {
  customFieldsService,
  type DefinitionListParams,
  type RecordListParams,
} from "../services/custom-fields.service";
import { customFieldsKeys } from "./query-keys";

/** Slug of the active organization (part of every query key). */
export function useOrgKey(): string | null {
  return useActiveOrganization().active?.slug ?? null;
}

/** Field types, their filter operators and limits. */
export function useCustomFieldMeta() {
  const org = useOrgKey();
  return useQuery({
    queryKey: customFieldsKeys.meta(org),
    queryFn: () => customFieldsService.meta(),
    enabled: Boolean(org),
    staleTime: 10 * 60_000,
  });
}

/**
 * Definitions of one entity (ordered by position). Archived ones are left
 * out unless `includeArchived`.
 */
export function useCustomFieldDefinitions(
  params: DefinitionListParams,
  options: { enabled?: boolean } = {},
) {
  const org = useOrgKey();
  const isObject = params.entityType === "object";
  return useQuery({
    queryKey: customFieldsKeys.definitionList(org, params),
    queryFn: () => customFieldsService.listDefinitions(params),
    enabled:
      Boolean(org) &&
      (options.enabled ?? true) &&
      (!isObject || Boolean(params.objectTypeUuid)),
    staleTime: 60_000,
  });
}

export function useCustomObjectTypes(options: { enabled?: boolean } = {}) {
  const org = useOrgKey();
  return useQuery({
    queryKey: customFieldsKeys.objectTypes(org),
    queryFn: () => customFieldsService.listObjectTypes(),
    enabled: Boolean(org) && (options.enabled ?? true),
    staleTime: 60_000,
  });
}

export function useCustomObjectType(uuid: string) {
  const org = useOrgKey();
  return useQuery({
    queryKey: customFieldsKeys.objectType(org, uuid),
    queryFn: () => customFieldsService.getObjectType(uuid),
    enabled: Boolean(org) && Boolean(uuid),
  });
}

export function useCustomObjectRecords(
  typeUuid: string,
  params: RecordListParams,
) {
  const org = useOrgKey();
  return useQuery({
    queryKey: customFieldsKeys.recordList(org, typeUuid, params),
    queryFn: () => customFieldsService.listRecords(typeUuid, params),
    enabled: Boolean(org) && Boolean(typeUuid),
    placeholderData: (previous) => previous,
  });
}

/** Open "tanımsız alan geldi" warnings, optionally of one widget (CF-196). */
export function useUnknownCustomFieldKeys(
  channelUuid?: string | null,
  options: { enabled?: boolean } = {},
) {
  const org = useOrgKey();
  return useQuery({
    queryKey: customFieldsKeys.unknownKeys(org, channelUuid),
    queryFn: () => customFieldsService.listUnknownKeys(channelUuid),
    enabled: Boolean(org) && (options.enabled ?? true),
    staleTime: 30_000,
  });
}

/**
 * Agent view of a contact's or company's custom fields: definitions with
 * visibility flags, values (sensitive ones masked) and undefined keys.
 */
export function useCustomFieldValues(
  entityType: "contact" | "company",
  entityUuid: string | null | undefined,
  options: { enabled?: boolean } = {},
) {
  const org = useOrgKey();
  return useQuery({
    queryKey: customFieldsKeys.values(org, entityType, entityUuid ?? ""),
    queryFn: () => customFieldsService.values(entityType, entityUuid ?? ""),
    enabled: Boolean(org) && Boolean(entityUuid) && (options.enabled ?? true),
    staleTime: 30_000,
  });
}
