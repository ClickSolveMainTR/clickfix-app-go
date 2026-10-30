"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";

import {
  healthService,
  type CustomerHealthSettingsInput,
} from "../services/health.service";
import { contactsKeys, useContactsScope } from "./use-contacts";

const healthKey = (org: string, uuid: string) =>
  [...contactsKeys.all(org), "health", uuid] as const;
const settingsKey = (org: string) =>
  [...contactsKeys.all(org), "health-settings"] as const;

/** Health score of one company (CF-87). */
export function useCompanyHealth(companyUuid: string | null) {
  const { org, canRead } = useContactsScope();
  return useQuery({
    queryKey: healthKey(org, companyUuid ?? ""),
    queryFn: () => healthService.get(companyUuid as string),
    enabled: Boolean(org && companyUuid) && canRead,
  });
}

export function useRecomputeHealth(companyUuid: string) {
  const { org } = useContactsScope();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: () => healthService.recompute(companyUuid),
    onSuccess: (data) => {
      qc.setQueryData(healthKey(org, companyUuid), data);
      void qc.invalidateQueries({ queryKey: contactsKeys.all(org) });
    },
  });
}

export function useHealthSettings() {
  const { org, canRead } = useContactsScope();
  return useQuery({
    queryKey: settingsKey(org),
    queryFn: () => healthService.settings(),
    enabled: Boolean(org) && canRead,
  });
}

export function useSaveHealthSettings() {
  const { org } = useContactsScope();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (body: CustomerHealthSettingsInput) =>
      healthService.saveSettings(body),
    onSuccess: (data) => qc.setQueryData(settingsKey(org), data),
  });
}
