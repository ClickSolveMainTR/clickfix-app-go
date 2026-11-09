"use client";

import { useMutation } from "@tanstack/react-query";
import { FlaskConical } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

import { display } from "../lib/json";
import {
  connectService,
  type SystemPlaybook,
  type SystemPreview,
  type SystemPreviewInput,
} from "../services/connect.service";
import { useConnectError } from "./use-connect-error";

const NONE = "__none__";
type Events = NonNullable<SystemPreviewInput["events"]>;

const STATUS_VARIANT = {
  accepted: "success",
  rejected: "danger",
  skipped: "secondary",
  ok: "success",
  error: "danger",
} as const;

function parseEvents(text: string): Events {
  return text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      if (l.startsWith("{")) {
        try {
          return JSON.parse(l) as Events[number];
        } catch {
          return { name: l };
        }
      }
      return { name: l };
    });
}

/**
 * "Bu müşteri olarak dene": what an integration would send for one
 * customer, what ClickFix accepts or rejects and why, and the context the
 * AI would see.
 */
export function PreviewCard({
  slug,
  playbooks,
}: {
  slug: string;
  playbooks: SystemPlaybook[];
}) {
  const { t } = useLocale();
  const onError = useConnectError();
  const [externalId, setExternalId] = useState("");
  const [companyId, setCompanyId] = useState("");
  const [pageUrl, setPageUrl] = useState("");
  const [events, setEvents] = useState("");
  const [attributes, setAttributes] = useState("");
  const [attributesError, setAttributesError] = useState(false);
  const [intent, setIntent] = useState(NONE);
  const [result, setResult] = useState<SystemPreview | null>(null);

  const run = useMutation({
    mutationFn: (attrs: Record<string, unknown>) =>
      connectService.preview(slug, {
        external_id: externalId,
        company_external_id: companyId,
        page_url: pageUrl,
        events: parseEvents(events),
        attributes: attrs,
        intent: intent === NONE ? "" : intent,
      }),
    onSuccess: setResult,
    onError,
  });
  const submit = () => {
    let attrs: Record<string, unknown> = {};
    if (attributes.trim()) {
      try {
        const parsed: unknown = JSON.parse(attributes);
        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
          throw new Error("not an object");
        }
        attrs = parsed as Record<string, unknown>;
      } catch {
        setAttributesError(true);
        return;
      }
    }
    setAttributesError(false);
    run.mutate(attrs);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FlaskConical className="size-4" />
          {t("integrations_connect.preview.title")}
        </CardTitle>
        <CardDescription>
          {t("integrations_connect.preview.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="pv-ext">
              {t("integrations_connect.preview.external_id")}
            </Label>
            <Input
              id="pv-ext"
              value={externalId}
              maxLength={255}
              placeholder="CUST-42"
              onChange={(e) => setExternalId(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="pv-comp">
              {t("integrations_connect.preview.company_external_id")}
            </Label>
            <Input
              id="pv-comp"
              value={companyId}
              maxLength={255}
              onChange={(e) => setCompanyId(e.target.value)}
            />
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label htmlFor="pv-page">
              {t("integrations_connect.preview.page_url")}
            </Label>
            <Input
              id="pv-page"
              value={pageUrl}
              maxLength={2000}
              placeholder="https://app.firma.com/faturalar/F-2026-101"
              onChange={(e) => setPageUrl(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="pv-events">
              {t("integrations_connect.preview.events")}
            </Label>
            <Textarea
              id="pv-events"
              rows={3}
              value={events}
              placeholder={'fatura_goruntulendi\n{"name":"odeme_hatasi"}'}
              onChange={(e) => setEvents(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="pv-attrs">
              {t("integrations_connect.preview.attributes")}
            </Label>
            <Textarea
              id="pv-attrs"
              rows={3}
              className="font-mono"
              value={attributes}
              placeholder={'{"plan": "Pro"}'}
              onChange={(e) => setAttributes(e.target.value)}
            />
            {attributesError && (
              <p className="text-destructive text-xs">
                {t("integrations_connect.preview.attributes_invalid")}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label>{t("integrations_connect.preview.intent")}</Label>
            <Select value={intent} onValueChange={setIntent}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>
                  {t("integrations_connect.preview.no_intent")}
                </SelectItem>
                {playbooks.map((p) => (
                  <SelectItem key={p.intent} value={p.intent}>
                    {p.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <Button type="button" disabled={run.isPending} onClick={submit}>
          {t("integrations_connect.preview.run")}
        </Button>

        {result && (
          <div className="space-y-4 rounded-md border p-3">
            <div className="text-sm">
              <span className="font-medium">
                {t("integrations_connect.preview.identity")}:
              </span>{" "}
              {result.identity.contact_name ||
                t("integrations_connect.preview.no_contact")}{" "}
              <code className="text-xs">{result.identity.external_id}</code>
              {result.identity.company_name && (
                <span className="text-muted-foreground">
                  {" "}
                  · {result.identity.company_name}{" "}
                  <code className="text-xs">
                    {result.identity.company_external_id}
                  </code>
                </span>
              )}
            </div>
            <div className="space-y-1">
              <p className="text-sm font-medium">
                {t("integrations_connect.preview.checks")}
              </p>
              {result.checks.map((c, i) => (
                <div
                  key={i}
                  className="flex flex-wrap items-start gap-2 text-sm"
                >
                  <Badge variant={STATUS_VARIANT[c.status]}>
                    {t(`integrations_connect.status.${c.status}`)}
                  </Badge>
                  <code className="text-xs">{c.key}</code>
                  <span className="text-muted-foreground">{c.reason}</span>
                </div>
              ))}
            </div>
            {result.widget_rejections.length > 0 && (
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t("integrations_connect.preview.rejections")}
                </p>
                {result.widget_rejections.map((r) => (
                  <p key={r.channel_uuid} className="text-sm">
                    <Badge variant="danger">{r.code}</Badge> {r.channel_name}{" "}
                    <span className="text-muted-foreground">
                      {t(`integrations_connect.rejection.${r.code}`)}
                    </span>
                  </p>
                ))}
              </div>
            )}
            {result.screen && (
              <div className="text-sm">
                <span className="font-medium">
                  {t("integrations_connect.preview.screen")}:
                </span>{" "}
                <code className="text-xs">{result.screen.page_path}</code>
                {result.screen.last_event && (
                  <span className="text-muted-foreground">
                    {" "}
                    · {result.screen.last_event}
                  </span>
                )}
              </div>
            )}
            {result.lookups.map((l) => (
              <div key={l.concept} className="space-y-1">
                <p className="flex items-center gap-2 text-sm font-medium">
                  {l.label || l.concept}
                  <Badge variant={STATUS_VARIANT[l.status]}>
                    {t(`integrations_connect.status.${l.status}`)}
                  </Badge>
                  {l.reason && (
                    <span className="text-muted-foreground text-xs font-normal">
                      {l.reason}
                    </span>
                  )}
                </p>
                {l.items.map((item, i) => (
                  <p key={i} className="text-muted-foreground text-xs">
                    {Object.entries(item)
                      .map(([k, v]) => `${k}: ${display(v)}`)
                      .join(" · ")}
                  </p>
                ))}
              </div>
            ))}
            <div className="space-y-1">
              <p className="text-sm font-medium">
                {t("integrations_connect.preview.ai_context")}
              </p>
              <pre className="bg-muted max-h-72 overflow-auto rounded p-2 text-xs whitespace-pre-wrap">
                {result.ai_context ||
                  t("integrations_connect.preview.ai_context_empty")}
              </pre>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
