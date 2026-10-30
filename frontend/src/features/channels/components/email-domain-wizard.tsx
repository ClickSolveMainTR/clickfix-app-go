"use client";

import { CheckCircle2, Copy, RefreshCw } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useEmailDomain,
  useRegisterEmailDomain,
  useVerifyEmailDomain,
} from "@/features/channels/hooks/use-channels";
import type { EmailDomain } from "@/features/channels/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function copy(value: string, done: string) {
  void navigator.clipboard.writeText(value).then(() => appToast.success(done));
}

/**
 * Sender domain wizard: register the domain, publish the SPF/DKIM records
 * shown, add a DMARC record, then verify. Sending through ClickFix needs a
 * verified domain owned by this organization.
 */
export function EmailDomainWizard({
  initialDomain = "",
}: {
  initialDomain?: string;
}) {
  const { t } = useLocale();
  const [input, setInput] = useState(initialDomain);
  const [domain, setDomain] = useState(initialDomain);
  const existing = useEmailDomain(domain);
  const register = useRegisterEmailDomain();
  const verify = useVerifyEmailDomain();
  const data: EmailDomain | undefined = existing.data ?? register.data;

  return (
    <div className="space-y-3 rounded-md border p-3">
      <div className="space-y-1">
        <Label htmlFor="email-domain">{t("channels.email.domain")}</Label>
        <div className="flex gap-2">
          <Input
            id="email-domain"
            placeholder="acme.com"
            value={input}
            onChange={(e) => setInput(e.target.value.trim().toLowerCase())}
          />
          <Button
            type="button"
            variant="outline"
            disabled={!input || register.isPending}
            onClick={() =>
              register.mutate(input, { onSuccess: () => setDomain(input) })
            }
          >
            {t("channels.email.domain_register")}
          </Button>
        </div>
        <p className="text-muted-foreground text-xs">
          {t("channels.email.domain_hint")}
        </p>
      </div>
      {data ? (
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <Badge variant={data.status === "verified" ? "success" : "warning"}>
              {t(`channels.email.domain_status.${data.status}`, {
                defaultValue: data.status,
              })}
            </Badge>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={verify.isPending}
              onClick={() => verify.mutate(data.domain)}
            >
              <RefreshCw className="size-4" />
              {t("channels.email.domain_verify")}
            </Button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="text-muted-foreground text-left">
                <tr>
                  <th className="py-1 pr-2">{t("channels.email.record")}</th>
                  <th className="py-1 pr-2">{t("channels.email.name")}</th>
                  <th className="py-1 pr-2">{t("channels.email.value")}</th>
                  <th className="py-1" />
                </tr>
              </thead>
              <tbody>
                {data.records.map((r) => (
                  <tr key={`${r.record}-${r.name}`} className="border-t">
                    <td className="py-1 pr-2 whitespace-nowrap">
                      {r.record} · {r.type}
                    </td>
                    <td className="max-w-40 truncate py-1 pr-2 font-mono">
                      {r.name}
                    </td>
                    <td className="max-w-56 truncate py-1 pr-2 font-mono">
                      {r.value}
                    </td>
                    <td className="py-1 text-right whitespace-nowrap">
                      {r.status === "verified" ? (
                        <CheckCircle2 className="inline size-4 text-emerald-600" />
                      ) : (
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          aria-label={t("channels.copy")}
                          onClick={() => copy(r.value, t("channels.copied"))}
                        >
                          <Copy className="size-3.5" />
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
                <tr className="border-t">
                  <td className="py-1 pr-2">DMARC · TXT</td>
                  <td className="py-1 pr-2 font-mono">_dmarc</td>
                  <td className="max-w-56 truncate py-1 pr-2 font-mono">
                    {data.dmarc || data.dmarc_recommended}
                  </td>
                  <td className="py-1 text-right">
                    {data.dmarc ? (
                      <CheckCircle2 className="inline size-4 text-emerald-600" />
                    ) : (
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        aria-label={t("channels.copy")}
                        onClick={() =>
                          copy(data.dmarc_recommended, t("channels.copied"))
                        }
                      >
                        <Copy className="size-3.5" />
                      </Button>
                    )}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </div>
  );
}
