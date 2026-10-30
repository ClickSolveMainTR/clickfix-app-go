import {
  Activity,
  Bell,
  Building2,
  Contact,
  Download,
  Inbox,
  MessageSquareText,
  StickyNote,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  ScrollText,
  Settings2,
  Shield,
  Upload,
  Users,
  type LucideIcon,
} from "lucide-react";

import { brandNavIcon } from "@/components/brand-logos";

const ICONS: Record<string, LucideIcon> = {
  users: Users,
  shield: Shield,
  pages: LayoutDashboard,
  bell: Bell,
  activity: Activity,
  logs: ScrollText,
  storage: HardDrive,
  access: KeyRound,
  imports: Upload,
  exports: Download,
  settings: Settings2,
  home: LayoutDashboard,
  github: brandNavIcon("github"),
  // Organization records (CF-45).
  inbox: Inbox,
  message: MessageSquareText,
  note: StickyNote,
  contact: Contact,
  building: Building2,
  conversations: Inbox,
  messages: MessageSquareText,
  contacts: Contact,
  companies: Building2,
};

export function resolveSearchIcon(name?: string): LucideIcon {
  if (!name) return LayoutDashboard;
  return ICONS[name.toLowerCase()] ?? LayoutDashboard;
}
