"use client";

import {
  useDesktopBadge,
  useDesktopSignOut,
} from "@/features/desktop/hooks/use-desktop-bridge";
import { useAuth } from "@/providers/auth-provider";

/**
 * Workspace shell glue for ClickFix Masaüstü (CF-236): badge count and the
 * tray's sign-out. Renders nothing and does nothing in a browser.
 */
export function DesktopBridgeHost() {
  const { logout } = useAuth();
  useDesktopBadge();
  useDesktopSignOut(logout);
  return null;
}
