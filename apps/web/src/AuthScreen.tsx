import { useState, type FormEvent } from "react";
import { api, ApiError } from "./lib/api";

type Mode = "login" | "register";

export function AuthScreen() {
  const [mode, setMode] = useState<Mode>("register");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [orgName, setOrgName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      if (mode === "register") {
        await api.register(email, password, orgName || undefined);
      } else {
        await api.login(email, password);
      }
      // authStore change re-renders App into the ledger.
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong");
      setBusy(false);
    }
  }

  return (
    <div className="auth">
      <aside className="auth__aside">
        <div className="auth__wordmark">OpenLedger</div>
        <p className="auth__pitch">
          Every entry <em>balances</em> to the penny.
        </p>
        <p className="auth__fineprint">
          Double-entry accounting on the web, modeled on GnuCash's proven kernel.
          Exact rational money, multi-currency, and books that import and export
          cleanly — no floating point, ever.
        </p>
      </aside>

      <div className="auth__form-wrap">
        <div className="auth__card">
          <div className="eyebrow">{mode === "register" ? "Open a new set of books" : "Welcome back"}</div>
          <h1>{mode === "register" ? "Create your ledger" : "Sign in"}</h1>
          <p className="sub">
            {mode === "register"
              ? "Start a new organization and your first book."
              : "Pick up where your last entry left off."}
          </p>

          <form className="auth__form" onSubmit={submit}>
            {mode === "register" && (
              <div className="field">
                <label htmlFor="org">Organization (optional)</label>
                <input
                  id="org"
                  value={orgName}
                  onChange={(e) => setOrgName(e.target.value)}
                  placeholder="Acme Bookkeeping"
                  autoComplete="organization"
                />
              </div>
            )}
            <div className="field">
              <label htmlFor="email">Email</label>
              <input
                id="email"
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                autoComplete="email"
              />
            </div>
            <div className="field">
              <label htmlFor="password">Password</label>
              <input
                id="password"
                type="password"
                required
                minLength={8}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="At least 8 characters"
                autoComplete={mode === "register" ? "new-password" : "current-password"}
              />
            </div>

            <div className="error-note">{error}</div>

            <button className="btn btn--accent" type="submit" disabled={busy}>
              {busy ? <span className="spinner" /> : mode === "register" ? "Create ledger" : "Sign in"}
            </button>
          </form>

          <p className="auth__switch">
            {mode === "register" ? "Already keeping books? " : "New here? "}
            <button
              type="button"
              onClick={() => {
                setMode(mode === "register" ? "login" : "register");
                setError("");
              }}
            >
              {mode === "register" ? "Sign in" : "Create a ledger"}
            </button>
          </p>
        </div>
      </div>
    </div>
  );
}
