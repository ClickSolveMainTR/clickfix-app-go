// Ekran kenarı çerçevesi penceresi (şeffaf, tıklama geçiren, her zaman
// üstte). Yalnız Rust kabuğu paylaşım sürerken açar.

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { systemLang, translator } from "./i18n";
import { Frame } from "./overlay";
import "./styles.css";
import "./overlay.css";

document.documentElement.classList.add("overlay");
const root = document.getElementById("root");
if (!root) throw new Error("#root bulunamadı");
createRoot(root).render(
  <StrictMode>
    <Frame t={translator(systemLang())} />
  </StrictMode>,
);
