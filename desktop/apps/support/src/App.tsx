import { useEffect, useState, type FormEvent } from "react";

import { formatCode, isCompleteCode, normalizeCode } from "./code";
import { sidecarVersion, type SidecarInfo } from "./sidecar";

type SidecarState =
  | { kind: "loading" }
  | { kind: "ready"; info: SidecarInfo }
  | { kind: "browser" }
  | { kind: "error"; message: string };

export function App() {
  const [code, setCode] = useState("");
  const [notice, setNotice] = useState<string | null>(null);
  const [sidecar, setSidecar] = useState<SidecarState>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    sidecarVersion()
      .then((info) => {
        if (!cancelled) {
          setSidecar(info ? { kind: "ready", info } : { kind: "browser" });
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) setSidecar({ kind: "error", message: String(err) });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!isCompleteCode(code)) return;
    // Oturum çözme (POST /v1/public/remote-support/resolve) CF-239'da gelir.
    setNotice("Bağlantı henüz hazır değil. Bu ekran bir yer tutucudur.");
  }

  return (
    <main className="card">
      <div className="logo" aria-hidden="true">
        C
      </div>
      <h1>ClickFix Destek</h1>
      <p className="muted">
        Destek temsilcinizin konuşmada paylaştığı 9 haneli kodu girin.
      </p>

      <form onSubmit={onSubmit}>
        <label htmlFor="code">Kodunuzu girin</label>
        <input
          id="code"
          inputMode="numeric"
          autoComplete="one-time-code"
          autoFocus
          placeholder="123 456 789"
          value={formatCode(code)}
          onChange={(e) => {
            setCode(normalizeCode(e.target.value));
            setNotice(null);
          }}
        />
        <button type="submit" disabled={!isCompleteCode(code)}>
          Bağlan
        </button>
      </form>

      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}

      <p className="warning">
        ClickFix çalışanları ve işletmeler sizden asla para transferi, banka
        girişi veya kart bilgisi istemez.
      </p>

      <footer className="muted small">{footer(sidecar)}</footer>
    </main>
  );
}

function footer(s: SidecarState): string {
  switch (s.kind) {
    case "loading":
      return "Yardımcı süreç başlatılıyor…";
    case "ready":
      return `Sürüm ${s.info.version} · ${s.info.os}/${s.info.arch}`;
    case "browser":
      return "Tarayıcı önizlemesi";
    case "error":
      return `Yardımcı süreç hazır değil: ${s.message}`;
  }
}
