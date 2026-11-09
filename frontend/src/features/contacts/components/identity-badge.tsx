"use client";

import { ShieldCheck, UserRound, UserRoundX } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { ContactIdentityState } from "../services/contacts.service";

const icons = {
  verified: ShieldCheck,
  anonymous: UserRoundX,
  self_named: UserRound,
} as const;

export type IdentityBadgeProps = {
  state: ContactIdentityState | undefined;
  /** Icon only (lists); the label is in the tooltip. */
  compact?: boolean;
  className?: string;
};

/**
 * CF-195: who the contact is. Verified: proved the customer's user id with
 * user_hash (widget identify). Anonymous: nothing known. Self named: told a
 * name, e-mail or phone without proof.
 */
export function IdentityBadge({
  state,
  compact,
  className,
}: IdentityBadgeProps) {
  const { t } = useLocale();
  if (!state) return null;
  const Icon = icons[state];
  const label = t(`contacts.identity.${state}`);
  const badge = (
    <Badge
      variant={state === "verified" ? "success" : "outline"}
      className={cn(
        "gap-1 font-normal",
        state === "anonymous" && "text-muted-foreground",
        compact && "size-5 justify-center rounded-full p-0",
        className,
      )}
      aria-label={label}
    >
      <Icon className="size-3" />
      {!compact && label}
    </Badge>
  );
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent className="max-w-64">
        {t(`contacts.identity.${state}_hint`)}
      </TooltipContent>
    </Tooltip>
  );
}
