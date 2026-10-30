"use client";

import { Fingerprint, KeyRound, Mail } from "lucide-react";
import type { ReactNode } from "react";

import { BrandLogo } from "@/components/brand-logos";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { UserAuthMethod } from "@/features/users/services/users.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils/index";

function methodLabel(
  method: UserAuthMethod,
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  if (method.kind === "password") {
    return t("users.auth_methods.password");
  }
  if (method.kind === "passkey") {
    if (method.label?.trim()) {
      return t("users.auth_methods.passkey_named", {
        name: method.label.trim(),
      });
    }
    return t("users.auth_methods.passkey");
  }
  const provider = method.provider ?? "oauth";
  const key = `users.auth_methods.oauth.${provider}`;
  const label = t(key);
  return label === key
    ? t("users.auth_methods.oauth.generic", { provider })
    : label;
}

function MethodIcon({ method }: { method: UserAuthMethod }) {
  const className = "size-3.5";
  if (method.kind === "password") {
    return <Mail className={className} />;
  }
  if (method.kind === "passkey") {
    return <Fingerprint className={className} />;
  }
  switch (method.provider) {
    case "github":
      return <BrandLogo slug="github" className={className} decorative />;
    case "google":
      return <BrandLogo slug="google" className={className} decorative />;
    case "facebook":
      return <BrandLogo slug="facebook" className={className} decorative />;
    case "apple":
      return <BrandLogo slug="apple" className={className} decorative />;
    default:
      return <KeyRound className={className} />;
  }
}

type UserAuthMethodsIconsProps = {
  methods: UserAuthMethod[];
  className?: string;
  empty?: ReactNode;
};

export function UserAuthMethodsIcons({
  methods,
  className,
  empty,
}: UserAuthMethodsIconsProps) {
  const { t, locale } = useLocale();

  if (methods.length === 0) {
    return (
      empty ?? (
        <span className="text-muted-foreground text-sm">
          {t("users.auth_methods.empty")}
        </span>
      )
    );
  }

  return (
    <div className={cn("flex flex-wrap items-center gap-1", className)}>
      {methods.map((method, index) => {
        const label = methodLabel(method, t);
        const linked = datetime(method.linked_at, undefined, locale);
        return (
          <Tooltip
            key={`${method.kind}-${method.provider ?? method.label ?? index}-${method.linked_at}`}
          >
            <TooltipTrigger asChild>
              <span
                className="border-border bg-muted/40 text-muted-foreground inline-flex size-7 items-center justify-center rounded-md border"
                aria-label={`${label} · ${linked}`}
              >
                <MethodIcon method={method} />
              </span>
            </TooltipTrigger>
            <TooltipContent side="top">
              <p className="font-medium">{label}</p>
              <p className="text-primary-foreground/80">
                {t("users.auth_methods.linked_at", { date: linked })}
              </p>
            </TooltipContent>
          </Tooltip>
        );
      })}
    </div>
  );
}
