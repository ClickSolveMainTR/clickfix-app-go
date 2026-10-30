"use client";

import {
  ArrowRight,
  Check,
  Inbox,
  Loader2,
  MessageCircle,
  Plug,
  Plus,
  Send,
  Trash2,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { invitationsService } from "@/features/invitations/services/invitations.service";
import { useAllTenantRoles } from "@/features/tenant-roles/hooks/use-tenant-roles";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { isApiError } from "@/lib/api";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { useLocale } from "@/providers/locale-provider";

import {
  useOnboarding,
  useOnboardingActions,
} from "../../hooks/use-onboarding";
import { onboardingService } from "../../services/onboarding.service";
import type { SetupRun } from "../../services/setup.service";

type Row = { id: number; email: string; role: string };
type Sent = { email: string; ok: boolean; error?: string };

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Last step: invite teammates (one invitation per address through the
 * existing invitations API), connect the web widget with the plan's
 * greeting and colours in one click, then open the inbox.
 */
export function TeamStep({
  slug,
  run,
  onFinish,
}: {
  slug: string;
  run: SetupRun | null;
  onFinish: () => void;
}) {
  const { t } = useLocale();
  const reduced = usePrefersReducedMotion();
  const roles = useAllTenantRoles(true);
  const { data: checklist } = useOnboarding();
  const { refresh } = useOnboardingActions(slug);
  const roleOptions = useMemo(
    () =>
      (roles.data?.items ?? [])
        .filter((r) => r.key !== "owner")
        .map((r) => ({
          value: r.uuid,
          key: r.key,
          label: tenantRoleLabel(t, r),
        })),
    [roles.data?.items, t],
  );
  const defaultRole =
    roleOptions.find((r) => r.key === "agent")?.value ??
    roleOptions[0]?.value ??
    "";
  const [rows, setRows] = useState<Row[]>([
    { id: 1, email: "", role: "" },
    { id: 2, email: "", role: "" },
  ]);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<Sent[]>([]);
  const [widget, setWidget] = useState<"idle" | "pending" | "done" | "error">(
    "idle",
  );

  const channelDone = Boolean(
    checklist?.steps.find((s) => s.key === "channel")?.done,
  );

  const update = (id: number, patch: Partial<Row>) =>
    setRows((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)));

  const valid = rows.filter((r) => EMAIL.test(r.email.trim()));

  const send = async () => {
    setSending(true);
    const results: Sent[] = [];
    for (const row of valid) {
      const email = row.email.trim();
      try {
        await invitationsService.create(
          { kind: "tenant", slug },
          { email, role_uuids: [row.role || defaultRole] },
        );
        results.push({ email, ok: true });
      } catch (error) {
        results.push({
          email,
          ok: false,
          error: isApiError(error) ? error.message : undefined,
        });
      }
    }
    setSent((prev) => [...prev, ...results]);
    setRows((rs) =>
      rs
        .filter((r) => !results.some((x) => x.ok && x.email === r.email.trim()))
        .concat(
          rs.every((r) =>
            results.some((x) => x.ok && x.email === r.email.trim()),
          )
            ? [{ id: Date.now(), email: "", role: "" }]
            : [],
        ),
    );
    setSending(false);
    void refresh();
  };

  const connectWidget = async () => {
    setWidget("pending");
    const w = run?.plan.widget;
    try {
      await onboardingService.connectChannel(slug, {
        type: "widget",
        name: t("onboarding.setup.team.widget_name"),
        settings: {
          color: w?.color || run?.plan.branding.primary_color || undefined,
          greeting_title: w?.greeting_title || undefined,
          greeting_text: w?.greeting_text || undefined,
          locale: run?.plan.summary.locale ?? "tr",
        },
      });
      setWidget("done");
      void refresh();
    } catch {
      setWidget("error");
    }
  };

  return (
    <div className="flex flex-col gap-8">
      <header className="space-y-2">
        <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
          {t("onboarding.setup.team.title")}
        </h1>
        <p className="text-muted-foreground text-sm">
          {t("onboarding.setup.team.body")}
        </p>
      </header>

      <section className="bg-card space-y-4 rounded-2xl border p-5 shadow-sm">
        <ul className="space-y-2">
          <AnimatePresence initial={false}>
            {rows.map((row) => (
              <motion.li
                key={row.id}
                initial={reduced ? false : { opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: "auto" }}
                exit={reduced ? undefined : { opacity: 0, height: 0 }}
                transition={{ duration: 0.25, ease: BRAND_EASE }}
                className="flex flex-col gap-2 sm:flex-row"
              >
                <Input
                  type="email"
                  autoComplete="off"
                  aria-label={t("onboarding.setup.team.email")}
                  placeholder={t("onboarding.setup.team.email_placeholder")}
                  value={row.email}
                  onChange={(e) => update(row.id, { email: e.target.value })}
                  className="flex-1"
                />
                <div className="flex gap-2">
                  <Select
                    value={row.role || defaultRole}
                    onValueChange={(v) => update(row.id, { role: v })}
                    disabled={roleOptions.length === 0}
                  >
                    <SelectTrigger
                      className="w-full sm:w-44"
                      aria-label={t("onboarding.setup.team.role")}
                    >
                      <SelectValue
                        placeholder={t("onboarding.setup.team.role")}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {roleOptions.map((r) => (
                        <SelectItem key={r.value} value={r.value}>
                          {r.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    variant="ghost"
                    size="icon"
                    disabled={rows.length === 1}
                    onClick={() =>
                      setRows((rs) => rs.filter((r) => r.id !== row.id))
                    }
                  >
                    <Trash2 />
                    <span className="sr-only">
                      {t("onboarding.setup.team.remove")}
                    </span>
                  </Button>
                </div>
              </motion.li>
            ))}
          </AnimatePresence>
        </ul>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={rows.length >= 10}
            onClick={() =>
              setRows((rs) => [...rs, { id: Date.now(), email: "", role: "" }])
            }
          >
            <Plus />
            {t("onboarding.setup.team.add")}
          </Button>
          <Button
            disabled={valid.length === 0 || sending || !defaultRole}
            onClick={() => void send()}
          >
            {sending ? <Loader2 className="animate-spin" /> : <Send />}
            {t("onboarding.setup.team.send", { count: valid.length })}
          </Button>
        </div>
        {sent.length > 0 ? (
          <ul className="space-y-1 border-t pt-3 text-sm">
            {sent.map((s, i) => (
              <motion.li
                key={`${s.email}-${i}`}
                initial={reduced ? false : { opacity: 0, x: -6 }}
                animate={{ opacity: 1, x: 0 }}
                className="flex items-center gap-2"
              >
                {s.ok ? (
                  <Check className="text-signal-text size-4" aria-hidden />
                ) : (
                  <span className="text-destructive" aria-hidden>
                    ×
                  </span>
                )}
                <span className="truncate">{s.email}</span>
                <span className="text-muted-foreground text-xs">
                  {s.ok
                    ? t("onboarding.setup.team.sent")
                    : (s.error ?? t("onboarding.setup.team.failed"))}
                </span>
              </motion.li>
            ))}
          </ul>
        ) : null}
      </section>

      <section className="grid gap-3 sm:grid-cols-2">
        <div className="bg-card flex flex-col gap-3 rounded-2xl border p-5">
          <span className="flex items-center gap-2 font-medium">
            <MessageCircle className="size-4" aria-hidden />
            {t("onboarding.setup.team.widget_title")}
          </span>
          <p className="text-muted-foreground text-sm">
            {t("onboarding.setup.team.widget_body")}
          </p>
          {widget === "done" || channelDone ? (
            <p className="text-signal-text flex items-center gap-2 text-sm font-medium">
              <Check className="size-4" aria-hidden />
              {t("onboarding.setup.team.widget_done")}
            </p>
          ) : (
            <Button
              variant="outline"
              className="w-fit"
              onClick={() => void connectWidget()}
              disabled={widget === "pending"}
            >
              {widget === "pending" ? (
                <Loader2 className="animate-spin" />
              ) : (
                <Plug />
              )}
              {t("onboarding.setup.team.widget_connect")}
            </Button>
          )}
          {widget === "error" ? (
            <p className="text-destructive text-xs">
              {t("onboarding.setup.team.widget_error")}
            </p>
          ) : null}
          <Link
            href={`${routes.cms.onboarding}?adim=channel`}
            className="text-muted-foreground text-xs hover:underline"
          >
            {t("onboarding.setup.team.other_channels")}
          </Link>
        </div>
        <div className="bg-card flex flex-col gap-3 rounded-2xl border p-5">
          <span className="flex items-center gap-2 font-medium">
            <Inbox className="size-4" aria-hidden />
            {t("onboarding.setup.team.inbox_title")}
          </span>
          <p className="text-muted-foreground text-sm">
            {t("onboarding.setup.team.inbox_body")}
          </p>
          <Button
            className="mt-auto w-fit text-white"
            style={{ background: "var(--setup-accent)" }}
            onClick={onFinish}
          >
            {t("onboarding.setup.team.open_inbox")}
            <ArrowRight />
          </Button>
        </div>
      </section>
    </div>
  );
}
