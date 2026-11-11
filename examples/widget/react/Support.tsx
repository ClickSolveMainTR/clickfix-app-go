// React (Next.js App Router) example with @getclickfix/react-widget.
// The hooks load https://<app>/widget.js once, boot it and keep typed state.
"use client";

import { useEffect, useMemo } from "react";
import { useClickFix, useClickFixEvent } from "@getclickfix/react-widget";

type User = { id: string; name: string; email: string; plan: string; userHash: string };

export function Support({ user, consent }: { user: User | null; consent: boolean }) {
  // Loading waits for consent (GDPR/KVKK); a stable object avoids re-booting.
  const options = useMemo(
    () =>
      consent
        ? { appOrigin: "https://app.getclickfix.com", widgetId: "YOUR-WIDGET-ID", hideLauncher: true, consent: true }
        : null,
    [consent],
  );
  const { ready, unread, open, sdk } = useClickFix(options);

  // Identify the signed-in user (user_hash comes from your server).
  useEffect(() => {
    if (!ready) return;
    if (user) void sdk.identify({ user_id: user.id, name: user.name, email: user.email, attributes: { plan: user.plan } }, user.userHash);
    else void sdk.reset();
  }, [ready, user, sdk]);

  useClickFixEvent("message", (m) => console.info("agent reply", m.conversation_uuid));
  useClickFixEvent("csat", (r) => console.info("rated", r.score));

  return (
    <button type="button" aria-expanded={open} onClick={() => sdk.toggle()}>
      Destek{unread > 0 ? ` (${unread})` : ""}
    </button>
  );
}
