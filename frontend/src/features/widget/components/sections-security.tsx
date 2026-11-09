"use client";

import { useQuery } from "@tanstack/react-query";
import { Copy, KeyRound, Play, RefreshCw, TriangleAlert } from "lucide-react";
import { useState } from "react";

import { BrandLogo, isBrandSlug } from "@/components/brand-logos";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { UnknownFieldsAlert } from "@/features/custom-fields";
import {
  Group,
  LocalizedRow,
  NumberRow,
  Row,
  SelectRow,
  SwitchRow,
  TextRow,
  useEditor,
} from "@/features/widget/components/fields";
import {
  generateSecret,
  hashSnippet,
  identityWarning,
  installFreshness,
  invalidSelector,
  parseDomains,
  parseSelectors,
  PLATFORMS,
  REPLAY_LIMITS,
  snippet,
} from "@/features/widget/lib/settings";
import { widgetConfigService } from "@/features/widget/services/widget.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export async function copyText(text: string, done: string) {
  try {
    await navigator.clipboard.writeText(text);
    appToast.success(done);
  } catch {
    appToast.error(text);
  }
}

/** The install check of a widget (shared by the sections that show it). */
function useInstalls(widgetId: string) {
  return useQuery({
    queryKey: ["widget-installs", widgetId],
    queryFn: () => widgetConfigService.installs(widgetId),
  });
}

/**
 * Tells the integrator why signed-in users stay anonymous (CF-176): the
 * widget's last identify call was rejected. Shows nothing otherwise.
 */
function IdentityWarning({
  widgetId,
  hasSecret,
  onOpenSecurity,
}: {
  widgetId: string;
  hasSecret: boolean;
  onOpenSecurity?: () => void;
}) {
  const { t, locale } = useLocale();
  const rejection = useInstalls(widgetId).data?.identity_rejection;
  const kind = identityWarning(rejection, hasSecret);
  if (!kind || !rejection) return null;
  return (
    <Alert data-testid={`identity-warning-${kind}`}>
      <TriangleAlert className="text-amber-600" />
      <AlertTitle>{t(`widget.identity_warning.${kind}_title`)}</AlertTitle>
      <AlertDescription>
        <p>{t(`widget.identity_warning.${kind}_body`)}</p>
        <p className="text-xs">
          {t("widget.identity_warning.last_attempt", {
            time: new Date(rejection.rejected_at).toLocaleString(
              locale === "tr" ? "tr-TR" : "en-US",
            ),
          })}{" "}
          {t("widget.identity_warning.clears")}
        </p>
        {onOpenSecurity ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onOpenSecurity}
          >
            <KeyRound className="size-4" />
            {t("widget.identity_warning.go_security")}
          </Button>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}

export function PrivacySection() {
  const { t } = useLocale();
  const ed = useEditor();
  const p = ed.settings.privacy;
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.g.privacy")}
        description={t("widget.g.privacy_hint")}
      >
        <SwitchRow
          path="privacy.consent_required"
          label={t("widget.f.consent_gate")}
          hint={t("widget.f.consent_gate_hint")}
        />
        {p.consent_required ? (
          <LocalizedRow
            path="privacy.consent_text"
            label={t("widget.f.consent_text")}
            maxLength={500}
            multiline
          />
        ) : null}
        <TextRow
          path="privacy.privacy_url"
          label={t("widget.f.privacy_url")}
          placeholder="https://"
          type="url"
        />
        <SwitchRow
          path="privacy.cookieless"
          label={t("widget.f.cookieless")}
          hint={t("widget.f.cookieless_hint")}
        />
        <SelectRow
          path="privacy.persistence"
          label={t("widget.f.persistence")}
          hint={t("widget.f.persistence_hint")}
          disabled={p.cookieless}
          options={(["local", "session", "memory"] as const).map((v) => [
            v,
            t(`widget.persistence.${v}`),
          ])}
        />
      </Group>
    </div>
  );
}

export function SecuritySection({
  widgetId,
  secretHint,
  onSaveSecret,
  canManage,
}: {
  widgetId: string;
  secretHint?: string;
  onSaveSecret: (secret: string) => Promise<void>;
  canManage: boolean;
}) {
  const { t } = useLocale();
  const ed = useEditor();
  const [domains, setDomains] = useState(() =>
    ed.settings.allowed_domains.join("\n"),
  );
  const [secret, setSecret] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  return (
    <div className="space-y-4">
      <Group title={t("widget.g.domains")}>
        <Row
          label={t("widget.fields.domains")}
          hint={t("widget.fields.domains_hint")}
          path="allowed_domains"
          htmlFor="widget-domains"
        >
          <Textarea
            id="widget-domains"
            value={domains}
            rows={4}
            className="font-mono text-xs"
            placeholder={"example.com\n*.example.com"}
            disabled={ed.disabled}
            onChange={(e) => {
              setDomains(e.target.value);
              ed.set("allowed_domains", parseDomains(e.target.value));
            }}
          />
        </Row>
      </Group>
      <Group
        title={t("widget.g.identity")}
        description={t("widget.fields.identity_secret_hint")}
      >
        <IdentityWarning widgetId={widgetId} hasSecret={Boolean(secretHint)} />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Label>{t("widget.fields.identity_secret")}</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!canManage || saving}
            onClick={() => setSecret(generateSecret())}
          >
            <KeyRound className="size-4" />
            {secretHint
              ? t("widget.actions.rotate_secret")
              : t("widget.actions.generate_secret")}
          </Button>
        </div>
        {secret ? (
          <div className="space-y-2">
            <div className="flex gap-2">
              <Input
                readOnly
                value={secret}
                className="font-mono text-xs"
                aria-label={t("widget.fields.identity_secret")}
              />
              <Button
                type="button"
                variant="outline"
                size="icon"
                aria-label={t("widget.actions.copy")}
                onClick={() => copyText(secret, t("widget.toast.copied"))}
              >
                <Copy className="size-4" />
              </Button>
            </div>
            <p className="text-xs text-amber-600">
              {t("widget.fields.identity_secret_once")}
            </p>
            <Button
              type="button"
              size="sm"
              disabled={saving}
              onClick={async () => {
                setSaving(true);
                try {
                  await onSaveSecret(secret);
                  setSecret(null);
                } finally {
                  setSaving(false);
                }
              }}
            >
              {t("widget.actions.save_secret")}
            </Button>
          </div>
        ) : secretHint ? (
          <p className="font-mono text-xs">{secretHint}</p>
        ) : (
          <p className="text-muted-foreground text-xs">
            {t("widget.fields.identity_secret_none")}
          </p>
        )}
        <SwitchRow
          path="require_identity"
          label={t("widget.fields.require_identity")}
          hint={t("widget.fields.require_identity_hint")}
          disabled={!secretHint}
        />
        <Tabs defaultValue="node">
          <TabsList>
            {(["node", "go", "php", "python"] as const).map((l) => (
              <TabsTrigger key={l} value={l}>
                {l}
              </TabsTrigger>
            ))}
          </TabsList>
          {(["node", "go", "php", "python"] as const).map((l) => (
            <TabsContent key={l} value={l}>
              <pre className="bg-muted overflow-x-auto rounded-lg p-3 text-xs">
                {hashSnippet(l)}
              </pre>
            </TabsContent>
          ))}
        </Tabs>
      </Group>
    </div>
  );
}

export function ReplaySection() {
  const { t } = useLocale();
  const ed = useEditor();
  const r = ed.settings.replay;
  const [selectors, setSelectors] = useState(() =>
    r.block_selectors.join("\n"),
  );
  const list = parseSelectors(selectors);
  const bad = invalidSelector(list);
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.tabs.replay")}
        description={t("widget.replay.intro")}
      >
        {(
          ["replay_enabled", "cobrowse_enabled", "control_enabled"] as const
        ).map((key) => (
          <SwitchRow
            key={key}
            path={`replay.${key}`}
            label={t(`widget.replay.${key}`)}
            hint={t(`widget.replay.${key}_hint`)}
            disabled={key === "control_enabled" && !r.cobrowse_enabled}
          />
        ))}
        <NumberRow
          path="replay.retention_days"
          label={t("widget.replay.retention_days")}
          hint={t("widget.replay.retention_hint")}
          min={REPLAY_LIMITS.minRetention}
          max={REPLAY_LIMITS.maxRetention}
        />
        <Row
          label={t("widget.replay.block_selectors")}
          htmlFor="widget-replay-selectors"
        >
          <Textarea
            id="widget-replay-selectors"
            value={selectors}
            rows={4}
            className="font-mono text-xs"
            placeholder={".payment\n[data-account-details]"}
            disabled={ed.disabled}
            aria-invalid={
              bad !== null || list.length > REPLAY_LIMITS.maxSelectors
            }
            onChange={(e) => {
              setSelectors(e.target.value);
              const next = parseSelectors(e.target.value);
              if (
                !invalidSelector(next) &&
                next.length <= REPLAY_LIMITS.maxSelectors
              )
                ed.set("replay.block_selectors", next);
            }}
          />
          {bad !== null ? (
            <p className="text-destructive text-xs" role="alert">
              {t("widget.replay.selector_invalid", {
                selector: bad.slice(0, 80),
              })}
            </p>
          ) : list.length > REPLAY_LIMITS.maxSelectors ? (
            <p className="text-destructive text-xs" role="alert">
              {t("widget.replay.selector_limit", {
                count: REPLAY_LIMITS.maxSelectors,
              })}
            </p>
          ) : (
            <p className="text-muted-foreground text-xs">
              {t("widget.replay.block_selectors_hint")}
            </p>
          )}
        </Row>
        <p className="text-muted-foreground text-xs">
          {t("widget.replay.plan_note")}
        </p>
      </Group>
    </div>
  );
}

/** Loads the real SDK on this page for a live try (app origin is allowed). */
function tryLive(widgetId: string) {
  type Sdk = ((...args: unknown[]) => unknown) & { __clickfix?: boolean };
  const w = window as unknown as { ClickFix?: Sdk };
  if (w.ClickFix?.__clickfix) {
    w.ClickFix("shutdown");
    w.ClickFix("init", { widgetId });
    w.ClickFix("on", "ready", () => w.ClickFix?.("open"));
    return;
  }
  const script = document.createElement("script");
  script.src = "/widget.js";
  script.async = true;
  script.dataset.widget = widgetId;
  script.dataset.manual = "true";
  script.onload = () => {
    w.ClickFix?.("init", { widgetId });
    w.ClickFix?.("on", "ready", () => w.ClickFix?.("open"));
  };
  document.body.append(script);
}

export function InstallSection({
  widgetId,
  hasSecret,
  onOpenSecurity,
}: {
  widgetId: string;
  hasSecret: boolean;
  onOpenSecurity: () => void;
}) {
  const { t, locale } = useLocale();
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  const installs = useInstalls(widgetId);
  const items = installs.data?.items;
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.install.snippet")}
        description={t("widget.install.snippet_hint")}
      >
        <Tabs defaultValue="html">
          {/* The list wraps on narrow panes; the variant pins h-9, so lift it. */}
          <TabsList className="h-auto flex-wrap justify-start group-data-[orientation=horizontal]/tabs:h-auto">
            {PLATFORMS.map((p) => (
              <TabsTrigger key={p} value={p} className="h-8 flex-none">
                {isBrandSlug(p) ? (
                  <BrandLogo slug={p} className="size-3.5" decorative />
                ) : null}
                {t(`widget.platform.${p}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          {PLATFORMS.map((p) => {
            const code = snippet(p, origin, widgetId);
            return (
              <TabsContent key={p} value={p} className="space-y-2">
                <p className="text-muted-foreground text-xs">
                  {t(`widget.platform_hint.${p}`)}
                </p>
                <pre
                  className="bg-muted max-h-72 overflow-auto rounded-lg p-3 text-xs"
                  data-testid={`snippet-${p}`}
                >
                  {code}
                </pre>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => copyText(code, t("widget.toast.copied"))}
                >
                  <Copy className="size-4" />
                  {t("widget.actions.copy")}
                </Button>
              </TabsContent>
            );
          })}
        </Tabs>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => tryLive(widgetId)}
          >
            <Play className="size-4" />
            {t("widget.actions.try_live")}
          </Button>
          <Button asChild variant="ghost" size="sm">
            <a href="/widget.d.ts" target="_blank" rel="noreferrer">
              {t("widget.install.types")} /widget.d.ts
            </a>
          </Button>
        </div>
      </Group>
      <Group
        title={t("widget.install.check")}
        description={t("widget.install.check_hint")}
      >
        <UnknownFieldsAlert channelUuid={widgetId} />
        <div className="flex items-center justify-between gap-2">
          <Label>{t("widget.install.seen_on")}</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={installs.isFetching}
            onClick={() => installs.refetch()}
          >
            <RefreshCw
              className={cn("size-4", installs.isFetching && "animate-spin")}
            />
            {t("widget.install.check_now")}
          </Button>
        </div>
        <IdentityWarning
          widgetId={widgetId}
          hasSecret={hasSecret}
          onOpenSecurity={onOpenSecurity}
        />
        {items && items.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="installs-empty"
          >
            {t("widget.install.not_seen")}
          </p>
        ) : null}
        <ul className="divide-y rounded-lg border">
          {(items ?? []).map((i) => {
            const fresh = installFreshness(i.last_seen_at);
            return (
              <li
                key={i.origin}
                className="flex flex-wrap items-center gap-3 p-3 text-sm"
              >
                <span
                  className={cn(
                    "size-2 rounded-full",
                    fresh === "live"
                      ? "bg-green-500"
                      : fresh === "recent"
                        ? "bg-amber-500"
                        : "bg-gray-400",
                  )}
                />
                <span className="font-medium">{i.origin}</span>
                {i.last_page_url ? (
                  <span className="text-muted-foreground truncate text-xs">
                    {i.last_page_url.replace(i.origin, "") || "/"}
                  </span>
                ) : null}
                <span className="text-muted-foreground ms-auto text-xs">
                  {new Date(i.last_seen_at).toLocaleString(
                    locale === "tr" ? "tr-TR" : "en-US",
                  )}
                </span>
                {i.sdk_version ? (
                  <Badge variant="secondary">SDK {i.sdk_version}</Badge>
                ) : null}
              </li>
            );
          })}
        </ul>
      </Group>
    </div>
  );
}
