import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

import pkg from "./package.json";

// Tauri geliştirme sunucusu: tauri.conf.json devUrl ile aynı port. Bu port
// yalnız `tauri dev` içindir; CI geliştirme sunucusu açmaz.
//
// Üç sayfa: ana pencere (index.html), paylaşım çubuğu (bar.html) ve ekran
// kenarı çerçevesi (frame.html).
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
  },
  server: {
    port: 1420,
    strictPort: true,
    host: "127.0.0.1",
  },
  envPrefix: ["VITE_", "TAURI_ENV_"],
  build: {
    target: "es2022",
    sourcemap: false,
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        main: "index.html",
        bar: "bar.html",
        frame: "frame.html",
      },
    },
  },
});
