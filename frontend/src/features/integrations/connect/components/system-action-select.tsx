"use client";

import { useQuery } from "@tanstack/react-query";

import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { connectKeys, connectService } from "../services/connect.service";

/**
 * Picks a connected system action for a workflow step (CF-197). Only
 * actions open to workflows that need no approval are listed: the API
 * refuses the others for workflows.
 */
export function SystemActionSelect({
  value,
  onChange,
  readOnly,
  label,
}: {
  value: string;
  onChange: (value: string) => void;
  readOnly?: boolean;
  label: string;
}) {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const canRead = permissions.includes(Permission.TenantIntegrationsRead);
  const overview = useQuery({
    queryKey: connectKeys.overview(slug ?? ""),
    queryFn: () => connectService.overview(slug ?? ""),
    enabled: Boolean(slug) && canRead,
  });
  const actions = (overview.data?.actions ?? []).filter(
    (a) =>
      a.enabled && !a.requires_approval && a.allowed_for.includes("workflow"),
  );
  const options =
    value && !actions.some((a) => a.key === value)
      ? [{ key: value, title: value }, ...actions]
      : actions;
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      <Select value={value} onValueChange={onChange} disabled={readOnly}>
        <SelectTrigger className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((a) => (
            <SelectItem key={a.key} value={a.key}>
              {a.title}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {actions.length === 0 && (
        <p className="text-muted-foreground text-xs">
          {t("workflows.system_action.none")}
        </p>
      )}
    </div>
  );
}
