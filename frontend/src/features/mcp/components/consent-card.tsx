"use client";

import { Cable, ShieldCheck, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { useLocale } from "@/providers/locale-provider";

import { lifetime, writeOffered } from "../lib/consent";
import type { McpConsent } from "../services/mcp.service";

export type ConsentCardProps = {
  consent: McpConsent;
  /** E-mail of the signed-in account that would give the access. */
  account: string;
  /** Uuid of the organization picked (or the only one). */
  selected: string | undefined;
  allowWrite: boolean;
  /** A decision is on its way or was made: nothing can be pressed again. */
  busy: boolean;
  /** Message key of a refused decision. */
  errorKey: string | null;
  onSelect: (uuid: string) => void;
  onAllowWrite: (allow: boolean) => void;
  onDecide: (decision: "approve" | "deny") => void;
};

/**
 * What the member reads before deciding (CF-173): who asks, where the answer
 * goes, with which account and for how long. Everything that comes from the
 * client (its name, its redirect host) is rendered as text and marked as the
 * client's own claim. No hooks besides the locale, so it renders in a test.
 */
export function ConsentCard({
  consent,
  account,
  selected,
  allowWrite,
  busy,
  errorKey,
  onSelect,
  onAllowWrite,
  onDecide,
}: ConsentCardProps) {
  const { t } = useLocale();
  const organization = consent.organizations.find((o) => o.uuid === selected);
  const canGrantWrite = writeOffered(consent.write_requested, organization);
  const access = lifetime(consent.access_ttl_seconds);
  const renewal = lifetime(consent.refresh_ttl_seconds);

  return (
    <Card className="w-full max-w-lg">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Cable className="size-5" />
          {t("mcp.consent.title")}
        </CardTitle>
        <CardDescription>{t("mcp.consent.asks_generic")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5 text-sm">
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-3">
          <dt className="text-muted-foreground">{t("mcp.consent.app")}</dt>
          <dd className="min-w-0">
            <span className="block font-medium break-words">
              {consent.client_name || consent.client_id}
            </span>
            <span className="text-muted-foreground flex items-start gap-1 text-xs">
              <TriangleAlert className="mt-0.5 size-3 shrink-0" />
              {t("mcp.consent.unverified")}
            </span>
          </dd>
          <dt className="text-muted-foreground">
            {t("mcp.consent.returns_to")}
          </dt>
          <dd className="min-w-0">
            <span className="block font-mono text-base font-semibold break-all">
              {consent.redirect_host}
            </span>
            <span className="text-muted-foreground text-xs">
              {consent.redirect_loopback
                ? t("mcp.consent.returns_local")
                : t("mcp.consent.returns_remote")}
            </span>
          </dd>
          <dt className="text-muted-foreground">
            {t("mcp.consent.connects_to")}
          </dt>
          <dd className="font-mono text-xs break-all">
            {consent.resource_url}
          </dd>
          <dt className="text-muted-foreground">{t("mcp.consent.account")}</dt>
          <dd className="font-medium break-all">{account}</dd>
        </dl>
        <p className="text-muted-foreground text-xs">
          {t("mcp.consent.warning")}
        </p>

        <fieldset className="space-y-2">
          <legend className="font-medium">
            {t("mcp.consent.organization")}
          </legend>
          {consent.organizations.map((o) => (
            <label
              key={o.uuid}
              className="hover:bg-muted/50 flex cursor-pointer items-center gap-3 rounded-md border p-3"
            >
              <input
                type="radio"
                name="organization"
                value={o.uuid}
                checked={selected === o.uuid}
                disabled={busy}
                onChange={() => onSelect(o.uuid)}
              />
              <span className="min-w-0 flex-1">
                <span className="block font-medium break-words">{o.name}</span>
                {o.tool_count != null ? (
                  <span className="text-muted-foreground text-xs">
                    {t("mcp.consent.tool_count", { count: o.tool_count })}
                  </span>
                ) : null}
              </span>
            </label>
          ))}
        </fieldset>

        <div className="space-y-2">
          <p className="flex items-center gap-2">
            <ShieldCheck className="text-muted-foreground size-4" />
            {t("mcp.consent.scope_read")}
          </p>
          {canGrantWrite ? (
            <div className="flex items-start gap-2">
              <Checkbox
                id="mcp-allow-write"
                checked={allowWrite}
                disabled={busy}
                onCheckedChange={(v) => onAllowWrite(v === true)}
              />
              <Label htmlFor="mcp-allow-write" className="leading-snug">
                {t("mcp.consent.scope_write")}
              </Label>
            </div>
          ) : consent.write_requested && organization ? (
            <p className="text-muted-foreground text-xs">
              {t("mcp.consent.write_unavailable")}
            </p>
          ) : null}
          {access && renewal ? (
            <p className="text-muted-foreground text-xs">
              {t("mcp.consent.duration", {
                access: t(`mcp.consent.${access.unit}`, {
                  count: access.count,
                }),
                renewal: t(`mcp.consent.${renewal.unit}`, {
                  count: renewal.count,
                }),
              })}
            </p>
          ) : null}
        </div>

        {errorKey ? (
          <p role="alert" className="text-destructive">
            {t(errorKey)}
          </p>
        ) : null}

        {/* Deny has the focus; allowing takes a deliberate click. */}
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            autoFocus
            variant="outline"
            disabled={busy}
            onClick={() => onDecide("deny")}
          >
            {t("mcp.consent.deny")}
          </Button>
          <Button
            disabled={busy || !organization}
            onClick={() => onDecide("approve")}
          >
            {t("mcp.consent.approve")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
