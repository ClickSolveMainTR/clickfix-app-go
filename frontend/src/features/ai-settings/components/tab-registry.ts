import {
  Bot,
  ClipboardCheck,
  Gauge,
  History,
  ListOrdered,
  ShieldCheck,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import type { ComponentType } from "react";

import { permissions } from "@/config/permissions";

import { AgentTab } from "./agent-tab";
import { EvaluationTab } from "./evaluation-tab";
import { InsightsTab } from "./insights-tab";
import { ProceduresTab } from "./procedures-tab";
import { RunsTab } from "./runs-tab";
import { SecurityTab } from "./security-tab";
import { ToolsTab } from "./tools-tab";

export type AISettingsTabProps = {
  /** Active organization slug (`X-Organization`). */
  slug: string;
  /** `tenant.settings.write`: may change settings. */
  canWrite: boolean;
  /** `tenant.conversations.reply`: may execute AI runs (test chat). */
  canRun: boolean;
  /** `tenant.ai_insights.manage`: may edit the evaluation set and run it. */
  canManage: boolean;
};

export type AISettingsTab = {
  /** `?tab=` value. */
  id: string;
  labelKey: string;
  icon: LucideIcon;
  component: ComponentType<AISettingsTabProps>;
  /** Shown only with this permission (default: the page's own). */
  permission?: string;
};

/**
 * Tabs of the "Yapay zeka" settings page, in display order. A new AI area
 * (e.g. knowledge sources, simulations, quality scoring) adds one entry here
 * plus its `ai_settings.tabs.<id>` label; the page handles routing.
 */
export const AI_SETTINGS_TABS: AISettingsTab[] = [
  {
    id: "agent",
    labelKey: "ai_settings.tabs.agent",
    icon: Bot,
    component: AgentTab,
  },
  {
    id: "procedures",
    labelKey: "ai_settings.tabs.procedures",
    icon: ListOrdered,
    component: ProceduresTab,
  },
  {
    id: "tools",
    labelKey: "ai_settings.tabs.tools",
    icon: Wrench,
    component: ToolsTab,
  },
  {
    id: "security",
    labelKey: "ai_settings.tabs.security",
    icon: ShieldCheck,
    component: SecurityTab,
  },
  {
    id: "runs",
    labelKey: "ai_settings.tabs.runs",
    icon: History,
    component: RunsTab,
  },
  {
    id: "insights",
    labelKey: "ai_settings.tabs.insights",
    icon: Gauge,
    component: InsightsTab,
    permission: permissions.tenant.aiInsights.read,
  },
  {
    id: "evaluation",
    labelKey: "ai_settings.tabs.evaluation",
    icon: ClipboardCheck,
    component: EvaluationTab,
    permission: permissions.tenant.aiInsights.read,
  },
];
