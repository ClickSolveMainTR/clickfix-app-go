"use client";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
type SIP = {
  sip_extension: string;
  sip_username: string;
  sip_password?: string;
  has_password?: boolean;
};
export function MemberSIPSettings({ user }: { user: string }) {
  const { slug, roles } = useTenant();
  const [sip, setSIP] = useState<SIP>({
    sip_extension: "",
    sip_username: "",
    sip_password: "",
  });
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const admin = roles.includes("admin") || roles.includes("owner");
  useEffect(() => {
    if (!admin || !slug) return;
    let cancelled = false;
    void tenantRequest<SIP>(
      slug,
      "GET",
      `/v1/tenant/webphone/members/${user}`,
      { silent: true },
    )
      .then((v) => {
        if (!cancelled) {
          setSIP({ ...v, sip_password: "" });
          setLoaded(true);
        }
      })
      .catch(() => {
        if (!cancelled) setError("SIP ayarları yüklenemedi.");
      });
    return () => {
      cancelled = true;
    };
  }, [slug, user, admin]);
  if (!admin) return null;
  return (
    <section className="space-y-3 border-t px-4 pt-4">
      <h3 className="font-medium">Netsantral SIP</h3>
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
      {(
        [
          ["sip_extension", "Dahili"],
          ["sip_username", "SIP kullanıcı adı"],
          ["sip_password", "SIP şifresi"],
        ] as const
      ).map(([key, label]) => (
        <div key={key} className="space-y-1">
          <Label htmlFor={key}>{label}</Label>
          <Input
            id={key}
            type={key === "sip_password" ? "password" : "text"}
            autoComplete="off"
            value={sip[key] ?? ""}
            onChange={(e) => setSIP({ ...sip, [key]: e.target.value })}
            placeholder={
              key === "sip_password" && sip.has_password
                ? "Kayıtlı şifreyi korumak için boş bırakın"
                : undefined
            }
          />
        </div>
      ))}
      <Button
        disabled={!loaded || saving}
        variant="outline"
        onClick={async () => {
          setSaving(true);
          setError("");
          try {
            const v = await tenantRequest<SIP>(
              slug,
              "PUT",
              `/v1/tenant/webphone/members/${user}`,
              { body: sip },
            );
            setSIP({ ...v, sip_password: "" });
          } catch {
            setError("SIP ayarları kaydedilemedi.");
          } finally {
            setSaving(false);
          }
        }}
      >
        SIP ayarlarını kaydet
      </Button>
    </section>
  );
}
