import { useEffect, useState } from "react";

import { api } from "../lib/api";
import type { Member, Role } from "../lib/types";
import { ROLES } from "../lib/types";

type Tab = "members" | "system";

// SettingsView is the admin/system home. It opens on member management (book
// RBAC) and reserves a "System" tab for locale/default-currency setup that the
// i18n and system-setup work will fill in.
export default function SettingsView({ bookGuid }: { bookGuid: string }) {
  const [tab, setTab] = useState<Tab>("members");

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">Administration</div>
          <h1>Settings</h1>
        </div>
      </header>

      <div className="settings__tabs" style={{ display: "flex", gap: "0.5rem", padding: "0 1.5rem 0.75rem" }}>
        <button
          className={`btn btn--sm ${tab === "members" ? "btn--primary" : "btn--ghost"}`}
          onClick={() => setTab("members")}
        >
          Members
        </button>
        <button
          className={`btn btn--sm ${tab === "system" ? "btn--primary" : "btn--ghost"}`}
          onClick={() => setTab("system")}
        >
          System
        </button>
      </div>

      {tab === "members" ? (
        <MembersPanel bookGuid={bookGuid} />
      ) : (
        <div className="empty" style={{ padding: "1.5rem" }}>
          System setup (default currency, locale, date &amp; number formats) is coming soon.
        </div>
      )}
    </section>
  );
}

function MembersPanel({ bookGuid }: { bookGuid: string }) {
  const [members, setMembers] = useState<Member[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("viewer");
  const [busy, setBusy] = useState(false);

  async function reload() {
    setError(null);
    try {
      setMembers(await api.listMembers(bookGuid));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not load members");
      setMembers([]);
    }
  }

  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bookGuid]);

  async function handleAdd() {
    if (!email.trim()) {
      setError("An email is required.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.addMember(bookGuid, { email: email.trim(), role });
      setEmail("");
      setRole("viewer");
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not add member");
    } finally {
      setBusy(false);
    }
  }

  async function handleRoleChange(m: Member, next: Role) {
    setError(null);
    try {
      await api.updateMember(bookGuid, m.userId, next);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not change role");
    }
  }

  async function handleRemove(m: Member) {
    setError(null);
    try {
      await api.removeMember(bookGuid, m.userId);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not remove member");
    }
  }

  return (
    <div style={{ padding: "0 1.5rem 1.5rem" }}>
      {error && <p className="error" style={{ margin: "0 0 0.75rem" }}>{error}</p>}

      <div
        className="settings__add"
        style={{ display: "flex", gap: "0.5rem", alignItems: "flex-end", marginBottom: "1rem", flexWrap: "wrap" }}
      >
        <label className="field" style={{ flex: "1 1 16rem" }}>
          <span className="field__label">Add member by email</span>
          <input
            type="email"
            placeholder="person@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <label className="field">
          <span className="field__label">Role</span>
          <select value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {ROLES.map((r) => (
              <option key={r} value={r}>{r}</option>
            ))}
          </select>
        </label>
        <button className="btn btn--primary btn--sm" onClick={handleAdd} disabled={busy}>
          {busy ? "Adding…" : "Add"}
        </button>
      </div>

      {members === null ? (
        <div className="empty"><span className="spinner" /></div>
      ) : members.length === 0 ? (
        <div className="empty">No members yet.</div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr><th>Email</th><th>User</th><th>Role</th><th></th></tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.userId}>
                <td>{m.email}</td>
                <td className="mono" style={{ color: "var(--ink-soft)" }}>{m.ldapUser}</td>
                <td>
                  <select value={m.role} onChange={(e) => handleRoleChange(m, e.target.value as Role)}>
                    {ROLES.map((r) => (
                      <option key={r} value={r}>{r}</option>
                    ))}
                  </select>
                </td>
                <td style={{ textAlign: "right" }}>
                  <button className="btn btn--ghost btn--sm" onClick={() => handleRemove(m)}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <p style={{ color: "var(--ink-soft)", fontSize: "0.8rem", marginTop: "1rem" }}>
        Members must have signed in at least once before they can be added. A book always keeps at least one owner.
      </p>
    </div>
  );
}
