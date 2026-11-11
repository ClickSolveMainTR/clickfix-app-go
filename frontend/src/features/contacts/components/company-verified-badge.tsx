"use client";

import { ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type CompanyVerifiedBadgeProps = {
  /** `verified_at` of the company; nothing is rendered while it is null. */
  verifiedAt: string | null | undefined;
  /** Icon only (lists); the label is in the tooltip. */
  compact?: boolean;
  className?: string;
};

/**
 * CF-206: the company's external id was proved by a signed-in user whose
 * user_hash signed the company id together with the user id. Companies
 * created by name or domain, by the API or by an agent carry no badge.
 */
export function CompanyVerifiedBadge({
  verifiedAt,
  compact,
  className,
}: CompanyVerifiedBadgeProps) {
  const { t } = useLocale();
  if (!verifiedAt) return null;
  const label = t("contacts.companies.verified");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="success"
          className={cn(
            "gap-1 font-normal",
            compact && "size-5 justify-center rounded-full p-0",
            className,
          )}
          aria-label={label}
        >
          <ShieldCheck className="size-3" />
          {!compact && label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent className="max-w-64">
        {t("contacts.companies.verified_hint")}{" "}
        {t("contacts.companies.verified_at", {
          date: datetime(verifiedAt, "dd.MM.yyyy HH:mm"),
        })}
      </TooltipContent>
    </Tooltip>
  );
}
