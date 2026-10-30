import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

import { optionBadgeStyle } from "../lib/colors";
import type { CustomFieldOption } from "../types";

type OptionBadgeProps = {
  option: Pick<CustomFieldOption, "label" | "color" | "archived">;
  className?: string;
  archivedLabel?: string;
};

/** Colored pill for a select option; archived options are muted. */
export function OptionBadge({
  option,
  className,
  archivedLabel,
}: OptionBadgeProps) {
  const { className: colorClass, style } = optionBadgeStyle(option.color);
  return (
    <Badge
      variant="outline"
      className={cn(
        "font-medium",
        colorClass,
        option.archived && "line-through opacity-60",
        className,
      )}
      style={style}
      title={option.archived ? archivedLabel : undefined}
    >
      {option.label}
    </Badge>
  );
}
