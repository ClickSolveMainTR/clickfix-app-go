"use client";

import { useQuery } from "@tanstack/react-query";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { inboxService } from "@/features/inbox/services/inbox.service";
import { useLocale } from "@/providers/locale-provider";

import { contactsKeys, useContactsScope } from "../hooks/use-contacts";

const NONE = "__none__";

/** Account owner (CSM) of a company: one of the organization's agents (CF-84). */
export function OwnerPicker({
  value,
  onChange,
}: {
  value: string | null;
  onChange: (uuid: string | null) => void;
}) {
  const { t } = useLocale();
  const { org } = useContactsScope();
  const agents = useQuery({
    queryKey: [...contactsKeys.all(org), "owners"],
    queryFn: () => inboxService.agents(),
    enabled: Boolean(org),
    staleTime: 5 * 60_000,
  });
  return (
    <Select
      value={value ?? NONE}
      onValueChange={(v) => onChange(v === NONE ? null : v)}
    >
      <SelectTrigger className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NONE}>{t("contacts.companies.no_owner")}</SelectItem>
        {(agents.data ?? []).map((a) => (
          <SelectItem key={a.uuid} value={a.uuid}>
            {a.name}
            <span className="text-muted-foreground ml-1 text-xs">
              {a.email}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
