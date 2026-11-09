"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
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
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  connectKeys,
  connectService,
  type SystemConnection,
  type SystemConnectionInput,
} from "../services/connect.service";
import { useConnectError } from "./use-connect-error";

type AuthType = NonNullable<SystemConnectionInput["auth_type"]>;
const AUTH_TYPES: AuthType[] = ["none", "bearer", "header", "basic"];

/** Base URL and authentication of the organization's system. */
export function ConnectionCard({
  slug,
  connection,
  canManage,
}: {
  slug: string;
  connection: SystemConnection | null;
  canManage: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useConnectError();
  const [name, setName] = useState(connection?.name ?? "");
  const [baseUrl, setBaseUrl] = useState(connection?.base_url ?? "");
  const [authType, setAuthType] = useState<AuthType>(
    (connection?.auth_type as AuthType) ?? "bearer",
  );
  const [header, setHeader] = useState(connection?.auth_header ?? "");
  const [secret, setSecret] = useState("");
  const [timeout, setTimeoutMs] = useState(connection?.timeout_ms ?? 8000);

  const save = useMutation({
    mutationFn: () =>
      connectService.saveConnection(slug, {
        name,
        base_url: baseUrl,
        auth_type: authType,
        auth_header: authType === "header" ? header : "",
        secret: secret || undefined,
        keep_secret: secret === "" && Boolean(connection?.has_secret),
        timeout_ms: timeout,
      }),
    onSuccess: async () => {
      setSecret("");
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
      appToast.success(t("integrations_connect.connection.saved"));
    },
    onError,
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRound className="size-4" />
          {t("integrations_connect.connection.title")}
          {connection ? (
            <Badge variant="secondary">
              {t("integrations_connect.connection.connected")}
            </Badge>
          ) : (
            <Badge variant="outline">
              {t("integrations_connect.connection.not_connected")}
            </Badge>
          )}
        </CardTitle>
        <CardDescription>
          {t("integrations_connect.connection.description")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-4 md:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="sys-name">
              {t("integrations_connect.connection.name")}
            </Label>
            <Input
              id="sys-name"
              value={name}
              maxLength={120}
              disabled={!canManage}
              placeholder={t(
                "integrations_connect.connection.name_placeholder",
              )}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="sys-base">
              {t("integrations_connect.connection.base_url")}
            </Label>
            <Input
              id="sys-base"
              value={baseUrl}
              required
              maxLength={500}
              disabled={!canManage}
              placeholder="https://api.firma.com/v1"
              onChange={(e) => setBaseUrl(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="sys-auth">
              {t("integrations_connect.connection.auth_type")}
            </Label>
            <Select
              value={authType}
              disabled={!canManage}
              onValueChange={(v) => setAuthType(v as AuthType)}
            >
              <SelectTrigger id="sys-auth" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {AUTH_TYPES.map((a) => (
                  <SelectItem key={a} value={a}>
                    {t(`integrations_connect.connection.auth.${a}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {authType === "header" && (
            <div className="space-y-2">
              <Label htmlFor="sys-header">
                {t("integrations_connect.connection.auth_header")}
              </Label>
              <Input
                id="sys-header"
                value={header}
                maxLength={64}
                disabled={!canManage}
                placeholder="X-Api-Key"
                onChange={(e) => setHeader(e.target.value)}
              />
            </div>
          )}
          {authType !== "none" && (
            <div className="space-y-2">
              <Label htmlFor="sys-secret">
                {t(
                  authType === "basic"
                    ? "integrations_connect.connection.secret_basic"
                    : "integrations_connect.connection.secret",
                )}
              </Label>
              <Input
                id="sys-secret"
                type="password"
                autoComplete="off"
                value={secret}
                maxLength={4000}
                disabled={!canManage}
                placeholder={
                  connection?.has_secret
                    ? t("integrations_connect.connection.secret_kept")
                    : ""
                }
                onChange={(e) => setSecret(e.target.value)}
              />
              <p className="text-muted-foreground text-xs">
                {t("integrations_connect.connection.secret_help")}
              </p>
            </div>
          )}
          <div className="space-y-2">
            <Label htmlFor="sys-timeout">
              {t("integrations_connect.connection.timeout")}
            </Label>
            <Input
              id="sys-timeout"
              type="number"
              min={1000}
              max={15000}
              step={500}
              value={timeout}
              disabled={!canManage}
              onChange={(e) => setTimeoutMs(Number(e.target.value))}
            />
          </div>
          <p className="text-muted-foreground text-xs md:col-span-2">
            {t("integrations_connect.connection.security")}
          </p>
          {canManage && (
            <div className="md:col-span-2">
              <Button type="submit" disabled={save.isPending || !baseUrl}>
                {t("integrations_connect.save")}
              </Button>
            </div>
          )}
        </form>
      </CardContent>
    </Card>
  );
}
