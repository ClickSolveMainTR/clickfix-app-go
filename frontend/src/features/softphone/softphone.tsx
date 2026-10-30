"use client";

import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Phone } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import {
  createSoftphoneEngine,
  engineConfigFrom,
  type SoftphoneEngine,
  type CallSnapshot,
  type RegistrationState,
} from "./engine";
import { clientCallEvent } from "./state";
import type { WebphoneConfig } from "./types";

const PhoneContext = createContext<{
  enabled: boolean;
  dial: (phone: string) => void;
}>({ enabled: false, dial: () => {} });
export function CallButton({ phone }: { phone?: string | null }) {
  const ctx = useContext(PhoneContext);
  if (!ctx.enabled || !phone) return null;
  return (
    <Button size="sm" variant="outline" onClick={() => ctx.dial(phone)}>
      <Phone className="size-4" />
      Ara
    </Button>
  );
}
export function SoftphoneProvider({ children }: { children: ReactNode }) {
  const { slug } = useTenant();
  return (
    <PhoneSession key={slug} slug={slug}>
      {children}
    </PhoneSession>
  );
}
function PhoneSession({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  const engine = useRef<SoftphoneEngine | null>(null);
  const audio = useRef<HTMLAudioElement | null>(null);
  const began = useRef<{ id: string; at: number; event: string } | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [registration, setRegistration] =
    useState<RegistrationState>("disconnected");
  const [call, setCall] = useState<CallSnapshot | null>(null);
  const [number, setNumber] = useState("");
  const [open, setOpen] = useState(false);
  const [now, setNow] = useState(0);
  const [failure, setFailure] = useState("");
  useEffect(() => {
    if (!slug) return;
    let stopped = false;
    const controller = new AbortController();
    void tenantRequest<WebphoneConfig>(
      slug,
      "GET",
      "/v1/tenant/webphone/config",
      { silent: true, signal: controller.signal },
    )
      .then(async (config) => {
        const settings = engineConfigFrom(config);
        if (!settings || stopped) return;
        const next = await createSoftphoneEngine(settings, {
          onRegistration: (state) => {
            if (!stopped) setRegistration(state);
          },
          onRemoteStream: (stream) => {
            if (audio.current) {
              audio.current.srcObject = stream;
              if (stream)
                void audio.current
                  .play()
                  .catch(() =>
                    setFailure("Sesi duymak için ses oynat düğmesine basın."),
                  );
            }
          },
          onCall: (snapshot) => {
            if (stopped) return;
            setCall(snapshot);
            if (!snapshot) return;
            setOpen(true);
            if (!began.current || began.current.id !== snapshot.id)
              began.current = {
                id: snapshot.id,
                at: Date.now(),
                event: crypto.randomUUID(),
              };
            if (snapshot.phase === "ended") {
              const event = {
                ...clientCallEvent(snapshot, {}, began.current.at, Date.now()),
                event_id: began.current.event,
              };
              void tenantRequest(slug, "POST", "/v1/tenant/webphone/events", {
                body: event,
                silent: true,
              }).catch(() =>
                setFailure(
                  "Arama özeti kaydedilemedi; santral raporu ile tamamlanacak.",
                ),
              );
            }
          },
        });
        if (stopped) {
          next.stop();
          return;
        }
        engine.current = next;
        setEnabled(true);
        next.start();
      })
      .catch(() => {});
    return () => {
      stopped = true;
      controller.abort();
      engine.current?.stop();
      engine.current = null;
    };
  }, [slug]);
  useEffect(() => {
    if (call?.phase !== "active") return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [call?.phase]);
  const active = Boolean(call && call.phase !== "ended");
  const dial = (phone: string) => {
    setNumber(phone);
    setOpen(true);
    setFailure("");
    engine.current?.call(phone);
  };
  const seconds = call?.startedAt
    ? Math.max(0, Math.floor(((now || call.startedAt) - call.startedAt) / 1000))
    : 0;
  return (
    <PhoneContext.Provider
      value={{ enabled: enabled && registration === "registered", dial }}
    >
      {children}
      <audio ref={audio} autoPlay aria-label="Arama sesi" />
      {enabled && (
        <div className="bg-background fixed right-4 bottom-4 z-50 max-w-xs rounded-xl border p-3 shadow-lg">
          <Button variant="outline" onClick={() => setOpen(!open)}>
            <Phone className="size-4" />
            Telefon ·{" "}
            {registration === "registered"
              ? "Hazır"
              : registration === "failed"
                ? "Bağlantı hatası"
                : "Bağlanıyor"}
          </Button>
          {open && (
            <div className="mt-3 space-y-3" aria-label="Telefon paneli">
              {failure && (
                <p role="alert" className="text-destructive text-sm">
                  {failure}
                </p>
              )}
              {call && (
                <p aria-live="polite">
                  {call.remoteName || call.remote} ·{" "}
                  {call.phase === "active"
                    ? `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`
                    : call.phase === "ringing"
                      ? "Çalıyor"
                      : call.phase === "ended"
                        ? call.endReason || "Arama bitti"
                        : "Bağlanıyor"}
                </p>
              )}
              <Input
                aria-label="Telefon numarası"
                value={number}
                onChange={(e) => setNumber(e.target.value)}
                type="tel"
              />
              <div className="grid grid-cols-3 gap-1">
                {"123456789*0#".split("").map((tone) => (
                  <Button
                    key={tone}
                    variant="outline"
                    onClick={() => {
                      if (active) engine.current?.sendDtmf(tone);
                      else setNumber(number + tone);
                    }}
                  >
                    {tone}
                  </Button>
                ))}
              </div>
              <div className="flex flex-wrap gap-2">
                {!active && (
                  <Button
                    disabled={registration !== "registered" || !number}
                    onClick={() => dial(number)}
                  >
                    Ara
                  </Button>
                )}
                {active && (
                  <>
                    {call?.direction === "incoming" &&
                      call.phase === "ringing" && (
                        <Button onClick={() => engine.current?.answer()}>
                          Kabul et
                        </Button>
                      )}
                    <Button
                      variant="destructive"
                      onClick={() => engine.current?.hangup()}
                    >
                      {call?.phase === "ringing" ? "Reddet" : "Kapat"}
                    </Button>
                    {call?.phase === "active" && (
                      <>
                        <Button
                          variant="outline"
                          onClick={() => engine.current?.setMuted(!call.muted)}
                        >
                          {call.muted ? "Mikrofonu aç" : "Sessiz"}
                        </Button>
                        <Button
                          variant="outline"
                          onClick={() => engine.current?.setHold?.(!call.held)}
                        >
                          {call.held ? "Devam et" : "Beklet"}
                        </Button>
                        <Button
                          variant="outline"
                          disabled={!number}
                          onClick={() => engine.current?.transfer(number)}
                        >
                          Aktar
                        </Button>
                      </>
                    )}
                  </>
                )}
                <Button
                  variant="ghost"
                  onClick={() =>
                    void audio.current
                      ?.play()
                      .catch(() =>
                        setFailure("Tarayıcı ses oynatmaya izin vermedi."),
                      )
                  }
                >
                  Ses oynat
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
    </PhoneContext.Provider>
  );
}
