import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In production all routing goes through Traefik (+ Authelia forward-auth).
// In dev the Vite server proxies /api/* to the local Go API, mirroring the
// Traefik path rule. Auth is handled at the Authelia layer (running in
// docker-compose.dev.yml), not by Vite.
const apiTarget = process.env.API_PROXY_TARGET ?? "http://localhost:8090";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: apiTarget, changeOrigin: true },
    },
  },
});
