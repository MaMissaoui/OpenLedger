import { useEffect, useState } from "react";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";

// Placeholder shell: confirms the SPA can reach the API's health endpoint.
// Replace with the account-tree + register UI per docs/ARCHITECTURE.md.
export function App() {
  const [status, setStatus] = useState<string>("checking…");

  useEffect(() => {
    fetch(`${API_BASE_URL}/healthz`)
      .then((r) => r.json())
      .then((d: { status: string }) => setStatus(d.status))
      .catch(() => setStatus("unreachable"));
  }, []);

  return (
    <main style={{ fontFamily: "system-ui", padding: "2rem" }}>
      <h1>OpenLedger</h1>
      <p>GnuCash-inspired double-entry accounting for the web.</p>
      <p>
        API health: <strong>{status}</strong>
      </p>
    </main>
  );
}
