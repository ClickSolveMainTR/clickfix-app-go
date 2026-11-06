"use client";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { useLocale } from "@/providers/locale-provider";

import type { StatusIncident } from "../services/oncall.service";

/**
 * State of one incident, shown the same way on the on-call alerts tab, the
 * agents' incidents page and the mobile app: "Resolved" once it is over,
 * the incident's own status while it is going on.
 */
export function IncidentStatusChip({ incident }: { incident: StatusIncident }) {
  const { t } = useLocale();
  if (incident.resolved_at)
    return (
      <StatusChip label={t("oncall.incidents.state_resolved")} tone="success" />
    );
  return <StatusChip label={incident.status ?? "—"} tone="danger" />;
}

export function IncidentSeverityBadge({
  incident,
}: {
  incident: StatusIncident;
}) {
  if (!incident.severity) return null;
  return <Badge variant="outline">{incident.severity}</Badge>;
}

/** "Public" when the incident is on the status page, "Private" when not. */
export function IncidentVisibilityBadge({
  incident,
}: {
  incident: StatusIncident;
}) {
  const { t } = useLocale();
  return (
    <Badge variant={incident.public ? "secondary" : "outline"}>
      {incident.public
        ? t("oncall.incidents.public")
        : t("oncall.incidents.private")}
    </Badge>
  );
}
