"use client";

import { useMemo } from "react";
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useAppMutation } from "@/lib/query/mutation";

import {
  contactsService,
  type CompanyInput,
  type ContactInput,
  type ContactListQuery,
} from "../services/contacts.service";

export const contactsKeys = {
  all: (org: string) => ["contacts", org] as const,
  list: (org: string, q: ContactListQuery) =>
    [...contactsKeys.all(org), "list", q] as const,
  detail: (org: string, uuid: string) =>
    [...contactsKeys.all(org), "detail", uuid] as const,
  companies: (org: string, q: object) =>
    [...contactsKeys.all(org), "companies", q] as const,
};

/** Active organization and the caller's contact permissions. */
export function useContactsScope() {
  const { active } = useActiveOrganization();
  const perms = useMemo(
    () => new Set<string>(active?.permissions ?? []),
    [active?.permissions],
  );
  return {
    org: active?.slug ?? "",
    canRead: perms.has(Permission.TenantContactsRead),
    canWrite: perms.has(Permission.TenantContactsWrite),
    canDelete: perms.has(Permission.TenantContactsDelete),
  };
}

export function useContactList(q: ContactListQuery) {
  const { org, canRead } = useContactsScope();
  return useQuery({
    queryKey: contactsKeys.list(org, q),
    queryFn: () => contactsService.list(q),
    enabled: Boolean(org) && canRead,
    placeholderData: keepPreviousData,
  });
}

export function useContact(uuid: string | null) {
  const { org } = useContactsScope();
  return useQuery({
    queryKey: contactsKeys.detail(org, uuid ?? ""),
    queryFn: () => contactsService.get(uuid as string),
    enabled: Boolean(org && uuid),
  });
}

export function useCompanyList(q: {
  q?: string;
  limit: number;
  offset: number;
}) {
  const { org, canRead } = useContactsScope();
  return useQuery({
    queryKey: contactsKeys.companies(org, q),
    queryFn: () => contactsService.companies(q),
    enabled: Boolean(org) && canRead,
    placeholderData: keepPreviousData,
  });
}

function useInvalidate() {
  const { org } = useContactsScope();
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: contactsKeys.all(org) });
    void qc.invalidateQueries({ queryKey: ["inbox", org] });
  };
}

export function useContactMutations() {
  const invalidate = useInvalidate();
  return {
    create: useAppMutation({
      mutationFn: (b: ContactInput) => contactsService.create(b),
      onSuccess: invalidate,
    }),
    update: useAppMutation({
      mutationFn: (v: { uuid: string; body: ContactInput }) =>
        contactsService.update(v.uuid, v.body),
      onSuccess: invalidate,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => contactsService.remove(uuid),
      onSuccess: invalidate,
    }),
    merge: useAppMutation({
      mutationFn: (v: { into: string; from: string }) =>
        contactsService.merge(v.into, v.from),
      onSuccess: invalidate,
    }),
  };
}

export function useCompanyMutations() {
  const invalidate = useInvalidate();
  return {
    create: useAppMutation({
      mutationFn: (b: CompanyInput) => contactsService.createCompany(b),
      onSuccess: invalidate,
    }),
    update: useAppMutation({
      mutationFn: (v: { uuid: string; body: CompanyInput }) =>
        contactsService.updateCompany(v.uuid, v.body),
      onSuccess: invalidate,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => contactsService.removeCompany(uuid),
      onSuccess: invalidate,
    }),
  };
}
