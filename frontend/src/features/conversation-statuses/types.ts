import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type CustomStatus = Schemas["ConversationCustomStatus"];
export type CustomStatusInput = Schemas["ConversationCustomStatusInput"];
export type CustomStatusCategory = Schemas["ConversationCustomStatusCategory"];

/** Base categories a custom status can map onto, in display order. */
export const CUSTOM_STATUS_CATEGORIES: CustomStatusCategory[] = [
  "open",
  "pending",
  "resolved",
];

/** Swatches offered in the form (any CSS color is accepted). */
export const STATUS_COLORS = [
  "#3b82f6",
  "#8b5cf6",
  "#f59e0b",
  "#f97316",
  "#10b981",
  "#ef4444",
  "#64748b",
];
