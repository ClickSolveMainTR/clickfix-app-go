import type {
  IssuePriority,
  IssueStatus,
} from "@/features/engineering/services/engineering.service";

export const ISSUE_STATUSES: IssueStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "done",
  "canceled",
];

export const ISSUE_PRIORITIES: IssuePriority[] = [
  "urgent",
  "high",
  "normal",
  "low",
];

type Tone = "default" | "success" | "warning" | "danger";

export function issueStatusTone(status: IssueStatus): Tone {
  if (status === "done") return "success";
  if (status === "in_progress") return "warning";
  return "default";
}

export function issuePriorityTone(priority: IssuePriority): Tone {
  return priority === "urgent" ? "danger" : "default";
}

/** "payment-flow" → "Payment flow". */
export function humanLabel(value: string, locale: string) {
  const label = value.replace(/[-_]+/g, " ").trim();
  return label ? label[0].toLocaleUpperCase(locale) + label.slice(1) : label;
}
