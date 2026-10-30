"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";

import {
  revenueService,
  type CompanyRevenueInput,
} from "../services/revenue.service";
import { contactsKeys, useContactsScope } from "./use-contacts";

const revenueKey = (org: string, uuid: string) =>
  [...contactsKeys.all(org), "revenue", uuid] as const;

/** Revenue and history of one company (CF-86). */
export function useCompanyRevenue(companyUuid: string | null) {
  const { org, canRead } = useContactsScope();
  return useQuery({
    queryKey: revenueKey(org, companyUuid ?? ""),
    queryFn: () => revenueService.get(companyUuid as string),
    enabled: Boolean(org && companyUuid) && canRead,
  });
}

export function useCompanyRevenueMutations(companyUuid: string) {
  const { org } = useContactsScope();
  const qc = useQueryClient();
  // Company lists embed `revenue`, so refresh everything under contacts.
  const invalidate = () =>
    void qc.invalidateQueries({ queryKey: contactsKeys.all(org) });
  return {
    save: useAppMutation({
      mutationFn: (body: CompanyRevenueInput) =>
        revenueService.put(companyUuid, body),
      onSuccess: invalidate,
    }),
    clear: useAppMutation({
      mutationFn: () => revenueService.clear(companyUuid),
      onSuccess: invalidate,
    }),
  };
}
