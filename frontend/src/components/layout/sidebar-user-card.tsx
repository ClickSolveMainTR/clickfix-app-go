"use client";

import Link from "next/link";
import { UserRound } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { useMyPresence, type PresenceStatus } from "@/features/presence";
import { useWorkforceClock } from "@/features/workforce/hooks/use-workforce-clock";
import { isPlatformUser } from "@/lib/auth/types";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { useRealtimeContext } from "@/providers/realtime-provider";

function initials(fullName?: string) {
  const parts = fullName?.trim().split(/\s+/).filter(Boolean) ?? [];
  const a = parts[0]?.[0] ?? "";
  const b = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return `${a}${b}`.toLocaleUpperCase("tr-TR") || "U";
}

type Presence = "online" | "connecting" | "offline" | "away" | "busy";

const AGENT_STATUSES: PresenceStatus[] = ["online", "away", "busy", "offline"];

/**
 * Presence of the signed-in user as this browser sees it: live while the
 * realtime socket is connected (or the network is up when realtime is off),
 * connecting while it retries, offline without a network.
 */
function usePresence(): "online" | "connecting" | "offline" {
  const { status, online } = useRealtimeContext();
  if (!online) return "offline";
  if (status === "connecting") return "connecting";
  if (status === "error" || status === "disconnected") return "offline";
  return "online";
}

function PresenceDot({ presence }: { presence: Presence }) {
  return (
    <span
      aria-hidden
      data-pulse={presence === "online"}
      className={cn(
        "signal-dot border-sidebar absolute -end-1 -bottom-1 size-3.5! border-[3px]",
        (presence === "connecting" || presence === "away") &&
          "bg-warning! shadow-none!",
        presence === "busy" && "bg-destructive! shadow-none!",
        presence === "offline" && "bg-white/35! shadow-none!",
      )}
    />
  );
}

function UserAvatar({
  fullName,
  presence,
}: {
  fullName?: string;
  presence: Presence;
}) {
  return (
    <span className="relative shrink-0">
      <Avatar className="size-8 rounded-lg">
        <AvatarFallback className="font-display rounded-lg bg-white/[0.08] text-xs font-semibold text-white">
          {initials(fullName)}
        </AvatarFallback>
      </Avatar>
      <PresenceDot presence={presence} />
    </span>
  );
}

/**
 * Sidebar footer: the current user with a presence dot. In the workspace
 * shell (an active organization) the dot is the agent status teammates see
 * (CF-10) and the card opens a status menu; elsewhere it shows the
 * connection state of this browser.
 */
export function SidebarUserCard({
  agentStatus = false,
}: {
  agentStatus?: boolean;
}) {
  const { active } = useActiveOrganization();
  if (agentStatus && active) return <AgentStatusCard />;
  return <ConnectionCard />;
}

function ConnectionCard() {
  const { t } = useLocale();
  const { user } = useAuth();
  const presence = usePresence();
  if (!user) return null;

  const profileHref = isPlatformUser(user)
    ? routes.platform.profile.root
    : routes.cms.profile.root;
  const label = t(`layout.presence_${presence}`);

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          size="lg"
          asChild
          tooltip={`${user.fullName} · ${label}`}
        >
          <Link href={profileHref}>
            <UserAvatar fullName={user.fullName} presence={presence} />
            <span className="grid min-w-0 flex-1 text-left leading-tight">
              <span className="truncate text-sm font-medium">
                {user.fullName || user.email}
              </span>
              <span className="truncate text-xs opacity-60">{label}</span>
            </span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

function AgentStatusCard() {
  const { t } = useLocale();
  const { user } = useAuth();
  const connection = usePresence();
  const { status, setStatus, isSaving } = useMyPresence();
  useWorkforceClock(status);
  if (!user) return null;

  // Without a socket teammates still see the server status; show the
  // connection problem first so the agent knows live updates are paused.
  const presence: Presence =
    connection !== "online" ? connection : (status ?? "offline");
  const label = t(`layout.presence_${presence}`);

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              tooltip={`${user.fullName} · ${label}`}
            >
              <UserAvatar fullName={user.fullName} presence={presence} />
              <span className="grid min-w-0 flex-1 text-left leading-tight">
                <span className="truncate text-sm font-medium">
                  {user.fullName || user.email}
                </span>
                <span className="truncate text-xs opacity-60">{label}</span>
              </span>
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="start" className="w-56">
            <DropdownMenuLabel>{t("layout.presence_menu")}</DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={status ?? ""}
              onValueChange={(v) => setStatus(v as PresenceStatus)}
            >
              {AGENT_STATUSES.map((s) => (
                <DropdownMenuRadioItem key={s} value={s} disabled={isSaving}>
                  {t(`layout.presence_${s}`)}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href={routes.cms.profile.root}>
                <UserRound />
                {t("layout.presence_profile")}
              </Link>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
