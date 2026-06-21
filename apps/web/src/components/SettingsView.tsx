import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { api } from "../lib/api";
import type { Member, Role } from "../lib/types";
import { ROLES } from "../lib/types";

type Tab = "members" | "system";

export default function SettingsView({ bookGuid }: { bookGuid: string }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("members");

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">{t("settings.eyebrow")}</div>
          <h1>{t("settings.title")}</h1>
        </div>
      </header>

      <div className="settings__tabs" style={{ display: "flex", gap: "0.5rem", padding: "0 1.5rem 0.75rem" }}>
        <button
          className={`btn btn--sm ${tab === "members" ? "btn--primary" : "btn--ghost"}`}
          onClick={() => setTab("members")}
        >
          {t("settings.tabs.members")}
        </button>
        <button
          className={`btn btn--sm ${tab === "system" ? "btn--primary" : "btn--ghost"}`}
          onClick={() => setTab("system")}
        >
          {t("settings.tabs.system")}
        </button>
      </div>

      {tab === "members" ? (
        <MembersPanel bookGuid={bookGuid} />
      ) : (
        <SystemPanel />
      )}
    </section>
  );
}

function MembersPanel({ bookGuid }: { bookGuid: string }) {
  const { t } = useTranslation();
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
      setError(e instanceof Error ? e.message : t("settings.members.errorLoad"));
      setMembers([]);
    }
  }

  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bookGuid]);

  async function handleAdd() {
    if (!email.trim()) {
      setError(t("settings.members.emailRequired"));
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
      setError(e instanceof Error ? e.message : t("settings.members.errorAdd"));
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
      setError(e instanceof Error ? e.message : t("settings.members.errorRole"));
    }
  }

  async function handleRemove(m: Member) {
    setError(null);
    try {
      await api.removeMember(bookGuid, m.userId);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("settings.members.errorRemove"));
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
          <span className="field__label">{t("settings.members.addLabel")}</span>
          <input
            type="email"
            placeholder={t("settings.members.addPlaceholder")}
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <label className="field">
          <span className="field__label">{t("settings.members.roleLabel")}</span>
          <select value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {ROLES.map((r) => (
              <option key={r} value={r}>{r}</option>
            ))}
          </select>
        </label>
        <button className="btn btn--primary btn--sm" onClick={handleAdd} disabled={busy}>
          {busy ? t("settings.members.adding") : t("settings.members.add")}
        </button>
      </div>

      {members === null ? (
        <div className="empty"><span className="spinner" /></div>
      ) : members.length === 0 ? (
        <div className="empty">{t("settings.members.noMembers")}</div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>{t("settings.members.emailCol")}</th>
              <th>{t("settings.members.userCol")}</th>
              <th>{t("settings.members.roleCol")}</th>
              <th></th>
            </tr>
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
                    {t("settings.members.remove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <p style={{ color: "var(--ink-soft)", fontSize: "0.8rem", marginTop: "1rem" }}>
        {t("settings.members.signInNote")}
      </p>
    </div>
  );
}

const LANGUAGE_OPTIONS = [
  { code: "en", key: "settings.system.languageEn" },
  { code: "fr", key: "settings.system.languageFr" },
  { code: "de", key: "settings.system.languageDe" },
] as const;

function SystemPanel() {
  const { t, i18n } = useTranslation();

  function handleLanguageChange(lng: string) {
    void i18n.changeLanguage(lng);
  }

  return (
    <div style={{ padding: "0 1.5rem 1.5rem" }}>
      <div className="field" style={{ maxWidth: "20rem" }}>
        <label className="field__label">{t("settings.system.languageLabel")}</label>
        <select value={i18n.resolvedLanguage ?? i18n.language} onChange={(e) => handleLanguageChange(e.target.value)}>
          {LANGUAGE_OPTIONS.map(({ code, key }) => (
            <option key={code} value={code}>{t(key)}</option>
          ))}
        </select>
      </div>

      <p style={{ color: "var(--ink-soft)", fontSize: "0.875rem", marginTop: "1.5rem" }}>
        {t("settings.system.comingSoon")}
      </p>
    </div>
  );
}
