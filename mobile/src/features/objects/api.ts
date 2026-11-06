import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

/**
 * Custom object records, read-only (CF-172), the panel's `/objects/[typeUuid]`
 * page (`frontend/src/features/custom-fields`). Object types and their field
 * definitions need `tenant.custom_fields.read`, records
 * `tenant.custom_objects.read` (`backend/internal/modules/customfields/
 * routes.go`); the screens need both and a hook never calls an endpoint the
 * member cannot read.
 */
export type ObjectType = components['schemas']['CustomObjectType'];
export type ObjectRecord = components['schemas']['CustomObjectRecord'];
export type FieldDefinition = components['schemas']['CustomFieldDefinition'];
type RecordPage = components['schemas']['CustomObjectRecordPage'];

export function useObjectsScope() {
  const can = useCan();
  return { canRecords: can(P.customFieldsRead) && can(P.customObjectsRead) };
}

export const RECORDS_PAGE = 30;

const keys = {
  all: (org: string) => orgKey(org, 'objects'),
  types: (org: string) => [...keys.all(org), 'types'] as const,
  definitions: (org: string, type: string) => [...keys.all(org), 'definitions', type] as const,
  records: (org: string, type: string, q: string) => [...keys.all(org), 'records', type, q] as const,
  record: (org: string, type: string, record: string) => [...keys.all(org), 'record', type, record] as const,
};

/** Object types of the organization; the API returns all of them (no paging), archived ones left out. */
export function useObjectTypes() {
  const org = useOrgSlug();
  const { canRecords } = useObjectsScope();
  return useQuery({
    queryKey: keys.types(org),
    enabled: !!org && canRecords,
    staleTime: 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/custom-objects', { params: { header: orgHeader() } })),
  });
}

/** Field definitions of one object type (the API returns all of them, no paging). */
export function useObjectDefinitions(type: string) {
  const org = useOrgSlug();
  const { canRecords } = useObjectsScope();
  return useQuery({
    queryKey: keys.definitions(org, type),
    enabled: !!org && !!type && canRecords,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/custom-fields', {
          params: { header: orgHeader(), query: { entity_type: 'object', object_type_uuid: type } },
        }),
      ),
  });
}

/** Records of one type, newest first like the panel, searched by `q`. */
export function useObjectRecords(type: string, q: string) {
  const org = useOrgSlug();
  const { canRecords } = useObjectsScope();
  const query = q.trim();
  return useInfiniteQuery({
    queryKey: keys.records(org, type, query),
    enabled: !!org && !!type && canRecords,
    initialPageParam: 0,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    queryFn: ({ pageParam }): Promise<RecordPage> =>
      unwrap(
        api.GET('/v1/tenant/custom-objects/{uuid}/records', {
          params: {
            header: orgHeader(),
            path: { uuid: type },
            query: { limit: RECORDS_PAGE, offset: pageParam, sort: '-created_at', ...(query ? { q: query } : {}) },
          },
        }),
      ),
    getNextPageParam: (last: RecordPage) =>
      last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
  });
}

export function useObjectRecord(type: string, record: string) {
  const org = useOrgSlug();
  const { canRecords } = useObjectsScope();
  return useQuery({
    queryKey: keys.record(org, type, record),
    enabled: !!org && !!type && !!record && canRecords,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/custom-objects/{uuid}/records/{record_uuid}', {
          params: { header: orgHeader(), path: { uuid: type, record_uuid: record } },
        }),
      ),
  });
}
