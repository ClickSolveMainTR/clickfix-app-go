"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, Trash2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { contactsService } from "@/features/contacts/services/contacts.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { slaService, type SlaPolicy } from "../services/sla.service";

/**
 * B2B contracts (CF-46): a company's contracted SLA policy applies to its
 * conversations before any condition match.
 */
export function ContractedCompanies({
  orgSlug,
  policies,
  canWrite,
}: {
  orgSlug: string;
  policies: SlaPolicy[];
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const key = ["tenant", orgSlug, "sla", "companies"] as const;
  const list = useQuery({
    queryKey: key,
    queryFn: slaService.contractedCompanies,
  });
  const [company, setCompany] = useState("");
  const [policy, setPolicy] = useState("");

  const set = useMutation({
    mutationFn: (p: { company: string; policy: string | null }) =>
      slaService.setCompanyPolicy(p.company, p.policy),
    onSuccess: async () => {
      setCompany("");
      await Promise.all([
        qc.invalidateQueries({ queryKey: key }),
        qc.invalidateQueries({
          queryKey: ["tenant", orgSlug, "sla", "policies"],
        }),
      ]);
      appToast.success(t("sla.toast.saved"));
    },
    onError: (error: unknown) =>
      appToast.error(isApiError(error) ? error.message : t("sla.toast.failed")),
  });

  const searchCompanies = async (q: string): Promise<ComboboxOption[]> => {
    const page = await contactsService.companies({ q, limit: 20, offset: 0 });
    return page.items.map((c) => ({
      value: c.uuid,
      label: c.name,
      description: c.tier || c.domain || undefined,
    }));
  };

  const items = list.data ?? [];
  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <Building2 className="size-4" />
            {t("sla.contracts.title")}
          </h3>
          <p className="text-muted-foreground text-xs">
            {t("sla.contracts.hint")}
          </p>
        </div>
        {canWrite && policies.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            <AsyncCombobox
              className="min-w-56 flex-1"
              value={company}
              onValueChange={setCompany}
              loadOptions={searchCompanies}
              placeholder={t("sla.contracts.company")}
              searchPlaceholder={t("sla.contracts.search")}
              emptyText={t("sla.contracts.no_company")}
            />
            <Select value={policy} onValueChange={setPolicy}>
              <SelectTrigger className="w-56">
                <SelectValue placeholder={t("sla.contracts.policy")} />
              </SelectTrigger>
              <SelectContent>
                {policies.map((p) => (
                  <SelectItem key={p.uuid} value={p.uuid}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              size="sm"
              disabled={!company || !policy || set.isPending}
              onClick={() => set.mutate({ company, policy })}
            >
              {t("sla.contracts.assign")}
            </Button>
          </div>
        )}
        {items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("sla.contracts.empty")}
          </p>
        ) : (
          <ul className="divide-y rounded-md border">
            {items.map((c) => (
              <li
                key={c.company_uuid}
                className="flex items-center gap-3 p-2.5 text-sm"
              >
                <span className="flex-1 truncate">
                  {c.name}
                  {c.tier && (
                    <span className="text-muted-foreground ml-2 text-xs">
                      {c.tier}
                    </span>
                  )}
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {c.policy.name}
                </span>
                {canWrite && (
                  <Button
                    disabled={set.isPending}
                    size="icon"
                    variant="ghost"
                    aria-label={t("sla.contracts.remove")}
                    onClick={() =>
                      set.mutate({ company: c.company_uuid, policy: null })
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
