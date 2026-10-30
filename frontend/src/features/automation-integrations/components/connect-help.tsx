"use client";

import { BookOpen, Workflow } from "lucide-react";

import { BrandLogo, type BrandSlug } from "@/components/brand-logos";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useLocale } from "@/providers/locale-provider";

import { ENDPOINTS, TRIGGER_EVENTS } from "../lib/constants";

/** Public docs pages (getclickfix.com/docs) for the tools below. */
const DOCS = {
  integrations: "/docs/mcp-integrations",
  n8n: "/docs/n8n-node",
  sdks: "/docs/sdks",
};

const TOOLS: { slug: BrandSlug; name: string }[] = [
  { slug: "zapier", name: "Zapier" },
  { slug: "make", name: "Make" },
  { slug: "n8n", name: "n8n" },
];

/** "Connect with Zapier / Make / n8n": the documented public API surface. */
export function ConnectHelp() {
  const { t } = useLocale();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Workflow className="size-4" />
          {t("automation_integrations.help.title")}
        </CardTitle>
        <CardDescription>
          {t("automation_integrations.help.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <ol className="list-decimal space-y-1.5 pl-5">
          <li>{t("automation_integrations.help.step_key")}</li>
          <li>
            {t("automation_integrations.help.step_base")}{" "}
            <code className="bg-muted rounded px-1 text-xs">
              https://&lt;api&gt;/v1/public-api
            </code>
          </li>
          <li>
            {t("automation_integrations.help.step_auth")}{" "}
            <code className="bg-muted rounded px-1 text-xs">
              Authorization: Bearer cf_sk_…
            </code>
          </li>
          <li>{t("automation_integrations.help.step_idempotency")}</li>
          <li>{t("automation_integrations.help.step_signature")}</li>
        </ol>

        <div className="overflow-x-auto rounded-md border">
          <table className="w-full min-w-[34rem] text-xs">
            <thead className="bg-muted/50 text-muted-foreground text-left">
              <tr>
                <th className="px-3 py-2 font-medium">
                  {t("automation_integrations.help.operation")}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t("automation_integrations.help.endpoint")}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t("automation_integrations.help.scope")}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {ENDPOINTS.map((e) => (
                <tr key={e.key}>
                  <td className="px-3 py-2">
                    {t(`automation_integrations.help.ops.${e.key}`)}
                  </td>
                  <td className="px-3 py-2 font-mono">
                    <span className="text-muted-foreground mr-1">
                      {e.method}
                    </span>
                    {e.path}
                  </td>
                  <td className="px-3 py-2 font-mono">{e.scope}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="space-y-1.5">
          <p className="font-medium">
            {t("automation_integrations.help.events")}
          </p>
          <div className="flex flex-wrap gap-1">
            {TRIGGER_EVENTS.map((e) => (
              <Badge key={e} variant="outline" className="font-mono">
                {e}
              </Badge>
            ))}
          </div>
          <p className="text-muted-foreground text-xs">
            {t("automation_integrations.help.polling")}
          </p>
        </div>

        <ul className="text-muted-foreground space-y-1.5 text-xs">
          {TOOLS.map(({ slug, name }) => (
            <li key={slug} className="flex items-start gap-2">
              <BrandLogo
                slug={slug}
                className="mt-px size-3.5 shrink-0"
                decorative
              />
              <span>
                <span className="text-foreground font-medium">{name}:</span>{" "}
                {t(`automation_integrations.help.${slug}`)}
                {slug === "n8n" ? (
                  <>
                    {" "}
                    <a
                      href={DOCS.n8n}
                      target="_blank"
                      rel="noopener"
                      className="text-foreground underline underline-offset-2"
                    >
                      {t("automation_integrations.help.n8n_docs")}
                    </a>
                  </>
                ) : null}
              </span>
            </li>
          ))}
        </ul>

        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" asChild>
            <a href={DOCS.integrations} target="_blank" rel="noopener">
              <BookOpen className="size-4" />
              {t("automation_integrations.help.docs")}
            </a>
          </Button>
          <Button size="sm" variant="ghost" asChild>
            <a href={DOCS.sdks} target="_blank" rel="noopener">
              {t("automation_integrations.help.sdks")}
            </a>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
