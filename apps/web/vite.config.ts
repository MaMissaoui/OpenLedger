import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In production all routing goes through Traefik (+ Authelia forward-auth).
// In dev the Vite server proxies /api/* to the local Go API and injects the
// same fake identity headers the nginx shim would add, so requireAuth passes
// without needing a real Authelia stack.
const apiTarget = process.env.API_PROXY_TARGET ?? "http://localhost:8090";
const devHeaders =
  process.env.NODE_ENV !== "production"
    ? { "Remote-User": "dev", "Remote-Email": "dev@localhost" }
    : {};

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: apiTarget, changeOrigin: true, headers: devHeaders },
    },
  },
});
