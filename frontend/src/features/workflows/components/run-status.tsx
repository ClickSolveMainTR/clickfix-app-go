"use client";

import { Badge } from "@/components/ui/badge";
import { useLocale } from "@/providers/locale-provider";

const TONE: Record<
  string,
  "success" | "warning" | "danger" | "secondary" | "outline"
> = {
  succeeded: "success",
  would_execute: "success",
  simulated: "secondary",
  running: "warning",
  started: "warning",
  pending: "secondary",
  waiting: "warning",
  would_wait: "warning",
  failed: "danger",
  blocked: "danger",
  uncertain: "warning",
};

/** Run, step and dry-run trace statuses share one badge. */
export function RunStatus({ status }: { status: string }) {
  const { t } = useLocale();
  return (
    <Badge variant={TONE[status] ?? "outline"}>
      {t(`workflows.run_status.${status}`)}
    </Badge>
  );
}
