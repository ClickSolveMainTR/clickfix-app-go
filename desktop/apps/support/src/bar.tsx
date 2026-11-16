// Paylaşım çubuğu penceresi (şeffaf, her zaman üstte, sürüklenebilir).
// Durumu Rust kabuğu `support://overlay` olayıyla verir; düğmeler
// `bar_action` komutunu çağırır ve Rust sidecar'a doğrudan iletir.

import { StrictMode, useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";

import { systemLang, translator } from "./i18n";
import { isSharing, type StateEvent } from "./ipc/types";
import { barAction, onOverlayState, overlayState } from "./native";
import { Bar } from "./overlay";
import "./styles.css";
import "./overlay.css";

const platform = navigator.userAgent.includes("Mac") ? "darwin" : "windows";

function BarWindow() {
  const t = useMemo(() => translator(systemLang()), []);
  const [state, setState] = useState<StateEvent | null>(null);
  useEffect(() => {
    void overlayState().then((s) => s && setState(s));
    return onOverlayState(setState);
  }, []);
  if (!state || !isSharing(state)) return null;
  return (
    <Bar
      t={t}
      platform={platform}
      state={state}
      native
      onStop={() => void barAction("stop")}
      onRevoke={() => void barAction("revoke")}
    />
  );
}

document.documentElement.classList.add("overlay");
const root = document.getElementById("root");
if (!root) throw new Error("#root bulunamadı");
createRoot(root).render(
  <StrictMode>
    <BarWindow />
  </StrictMode>,
);
