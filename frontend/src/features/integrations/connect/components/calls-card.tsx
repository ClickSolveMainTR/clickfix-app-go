"use client";

import { History } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { SystemCall } from "../services/connect.service";

/** The latest calls to the organization's system (masked, no bodies). */
export function CallsCard({ calls }: { calls: SystemCall[] }) {
  const { t, locale } = useLocale();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <History className="size-4" />
          {t("integrations_connect.calls.title")}
        </CardTitle>
        <CardDescription>
          {t("integrations_connect.calls.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {calls.length === 0 && (
          <p className="text-muted-foreground text-sm">
            {t("integrations_connect.calls.empty")}
          </p>
        )}
        {calls.map((c, i) => (
          <div key={i} className="flex flex-wrap items-center gap-2 text-xs">
            <Badge variant={c.ok ? "success" : "danger"}>
              {c.ok ? c.status_code : c.error_code || c.status_code}
            </Badge>
            <Badge variant="outline">
              {t(`integrations_connect.channel.${c.channel}`)}
            </Badge>
            <span className="font-medium">{c.target}</span>
            <code className="text-muted-foreground min-w-0 flex-1 truncate">
              {c.method} {c.masked_path}
            </code>
            <span className="text-muted-foreground">
              {c.duration_ms} ms · {relativeDatetime(c.created_at, locale)}
            </span>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
