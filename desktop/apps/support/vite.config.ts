import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Tauri geliştirme sunucusu: tauri.conf.json devUrl ile aynı port. Bu port
// yalnız `tauri dev` içindir; CI geliştirme sunucusu açmaz.
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
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
  },
});
