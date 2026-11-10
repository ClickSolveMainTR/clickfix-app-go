"use client";

import type { ColumnDef, Row, RowData } from "@tanstack/react-table";
import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

/** Id of the chevron column `renderSubRow` adds in front of the columns. */
export const EXPAND_COLUMN_ID = "__expand";

function ExpandHeader() {
  const { t } = useLocale();
  return <span className="sr-only">{t("table.expand")}</span>;
}

export function ExpandToggle<TData>({
  row,
  className,
}: {
  row: Row<TData>;
  className?: string;
}) {
  const { t } = useLocale();
  const expanded = row.getIsExpanded();
  return (
    <Button
      type="button"
      size="icon"
      variant="ghost"
      className={cn("size-7", className)}
      aria-expanded={expanded}
      aria-label={t(expanded ? "table.collapse_row" : "table.expand_row")}
      onClick={() => row.toggleExpanded()}
    >
      <ChevronRight
        className={cn(
          "size-4 transition-transform rtl:rotate-180",
          expanded && "rotate-90 rtl:rotate-90",
        )}
      />
    </Button>
  );
}

export function createExpandColumnDef<TData extends RowData>(): ColumnDef<
  TData,
  unknown
> {
  return {
    id: EXPAND_COLUMN_ID,
    size: 40,
    enableSorting: false,
    enableHiding: false,
    enableColumnFilter: false,
    enableResizing: false,
    enablePinning: true,
    meta: {
      labelKey: "table.expand",
      enableHiding: false,
    },
    header: () => <ExpandHeader />,
    cell: ({ row }) => (row.getCanExpand() ? <ExpandToggle row={row} /> : null),
  };
}

/**
 * Bottom of a card in grid view: the expand toggle and, when open, the detail
 * that the table view shows in a row under the row.
 */
export function CardSubRow<TData>({
  row,
  renderSubRow,
}: {
  row: Row<TData>;
  renderSubRow: (row: TData) => ReactNode;
}) {
  const { t } = useLocale();
  if (!row.getCanExpand()) return null;
  const expanded = row.getIsExpanded();
  return (
    <div className="border-border/60 mt-3 space-y-3 border-t pt-3">
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="-ms-2 h-7 gap-1 px-2"
        aria-expanded={expanded}
        onClick={() => row.toggleExpanded()}
      >
        <ChevronRight
          className={cn(
            "size-4 transition-transform rtl:rotate-180",
            expanded && "rotate-90 rtl:rotate-90",
          )}
        />
        {t(expanded ? "table.collapse_row" : "table.expand_row")}
      </Button>
      {expanded ? <div>{renderSubRow(row.original)}</div> : null}
    </div>
  );
}
