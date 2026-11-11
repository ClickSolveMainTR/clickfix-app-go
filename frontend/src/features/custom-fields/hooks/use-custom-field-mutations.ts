"use client";

import { useQueryClient } from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  customFieldsService,
  type CustomFieldScope,
} from "../services/custom-fields.service";
import type {
  CustomFieldCreateRequest,
  CustomFieldUpdateRequest,
  CustomObjectRecordRequest,
  CustomObjectTypeRequest,
} from "../types";
import { customFieldsKeys } from "./query-keys";
import { useOrgKey } from "./use-custom-fields";

function useInvalidateDefinitions() {
  const queryClient = useQueryClient();
  const org = useOrgKey();
  // A new field closes its unknown-key warning and fills contact values.
  return () =>
    queryClient.invalidateQueries({ queryKey: customFieldsKeys.all(org) });
}

export function useDismissUnknownKey() {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const org = useOrgKey();
  return useAppMutation({
    mutationFn: (uuid: string) => customFieldsService.dismissUnknownKey(uuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: [...customFieldsKeys.all(org), "unknown-keys"],
      });
      appToast.success(t("custom_fields.unknown.dismissed"));
    },
  });
}

export function useCreateCustomField() {
  const { t } = useLocale();
  const invalidate = useInvalidateDefinitions();
  return useAppMutation({
    mutationFn: (body: CustomFieldCreateRequest) =>
      customFieldsService.createDefinition(body),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("custom_fields.toast.field_created"));
    },
  });
}

export function useUpdateCustomField() {
  const { t } = useLocale();
  const invalidate = useInvalidateDefinitions();
  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: CustomFieldUpdateRequest;
    }) => customFieldsService.updateDefinition(uuid, body),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("custom_fields.toast.field_updated"));
    },
  });
}

export function useArchiveCustomField() {
  const { t } = useLocale();
  const invalidate = useInvalidateDefinitions();
  return useAppMutation({
    mutationFn: (uuid: string) => customFieldsService.archiveDefinition(uuid),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("custom_fields.toast.field_archived"));
    },
  });
}

export function useRestoreCustomField() {
  const { t } = useLocale();
  const invalidate = useInvalidateDefinitions();
  return useAppMutation({
    mutationFn: (uuid: string) => customFieldsService.restoreDefinition(uuid),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("custom_fields.toast.field_restored"));
    },
  });
}

/** Saves a new order; the list is updated optimistically by the caller. */
export function useReorderCustomFields(scope: CustomFieldScope) {
  const invalidate = useInvalidateDefinitions();
  return useAppMutation({
    mutationFn: (uuids: string[]) =>
      customFieldsService.reorder({
        entity_type: scope.entityType,
        object_type_uuid: scope.objectTypeUuid ?? null,
        uuids,
      }),
    onSettled: () => {
      void invalidate();
    },
  });
}

export function useSaveObjectType() {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const org = useOrgKey();
  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid?: string;
      body: CustomObjectTypeRequest;
    }) =>
      uuid
        ? customFieldsService.updateObjectType(uuid, body)
        : customFieldsService.createObjectType(body),
    onSuccess: (_type, vars) => {
      void queryClient.invalidateQueries({
        queryKey: customFieldsKeys.objectTypes(org),
      });
      appToast.success(
        t(
          vars.uuid
            ? vars.body.archived === true
              ? "custom_fields.objects.toast.archived"
              : vars.body.archived === false
                ? "custom_fields.objects.toast.restored"
                : "custom_fields.objects.toast.updated"
            : "custom_fields.objects.toast.created",
        ),
      );
    },
  });
}

export function useSaveRecord(typeUuid: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const org = useOrgKey();
  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid?: string;
      body: CustomObjectRecordRequest;
    }) =>
      uuid
        ? customFieldsService.updateRecord(typeUuid, uuid, body)
        : customFieldsService.createRecord(typeUuid, body),
    onSuccess: (_record, vars) => {
      void queryClient.invalidateQueries({
        queryKey: customFieldsKeys.records(org, typeUuid),
      });
      appToast.success(
        t(
          vars.uuid
            ? "custom_fields.objects.toast.record_updated"
            : "custom_fields.objects.toast.record_created",
        ),
      );
    },
  });
}

export function useDeleteRecord(typeUuid: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const org = useOrgKey();
  return useAppMutation({
    mutationFn: (recordUuid: string) =>
      customFieldsService.deleteRecord(typeUuid, recordUuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: customFieldsKeys.records(org, typeUuid),
      });
      appToast.success(t("custom_fields.objects.toast.record_deleted"));
    },
  });
}
