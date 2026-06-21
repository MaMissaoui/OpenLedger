import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { api } from "../lib/api";
import { toFloat } from "../lib/money";
import type { BillTerm, NewBillTerm } from "../lib/types";

// Discount is stored as a fraction (2/100 = 2%); the form shows a percentage.
function discountToPercent(n?: { num: number; denom: number }): string {
  if (!n || n.num === 0) return "";
  return String(toFloat(n) * 100);
}
function percentToDiscount(s: string): { num: number; denom: number } {
  const pct = parseFloat(s.trim());
  if (!s.trim() || Number.isNaN(pct)) return { num: 0, denom: 1 };
  return { num: Math.round(pct * 100), denom: 10000 }; // pct% = pct/100
}

interface FormState {
  name: string;
  description: string;
  type: "days" | "proximo";
  dueDays: string;
  discountDays: string;
  discountPct: string;
  cutoff: string;
}

function toForm(term?: BillTerm): FormState {
  return {
    name: term?.name ?? "",
    description: term?.description ?? "",
    type: term?.type ?? "days",
    dueDays: term?.dueDays != null ? String(term.dueDays) : "30",
    discountDays: term?.discountDays != null ? String(term.discountDays) : "0",
    discountPct: discountToPercent(term?.discount),
    cutoff: term?.cutoff != null ? String(term.cutoff) : "0",
  };
}

function BillTermDialog({
  bookGuid,
  existing,
  onSaved,
  onClose,
}: {
  bookGuid: string;
  existing?: BillTerm;
  onSaved: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [f, setF] = useState<FormState>(toForm(existing));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function set<K extends keyof FormState>(k: K, v: FormState[K]) {
    setF((prev) => ({ ...prev, [k]: v }));
  }

  async function handleSave() {
    setError(null);
    if (!f.name.trim()) { setError(t("business.nameRequired")); return; }
    const input: NewBillTerm = {
      name: f.name.trim(),
      description: f.description.trim() || undefined,
      type: f.type,
      dueDays: Number(f.dueDays) || 0,
      discountDays: Number(f.discountDays) || 0,
      discount: percentToDiscount(f.discountPct),
      cutoff: f.type === "proximo" ? Number(f.cutoff) || 0 : 0,
    };
    setSaving(true);
    try {
      if (existing) await api.updateBillTerm(existing.guid, input);
      else await api.createBillTerm(bookGuid, input);
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("business.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>{existing ? t("business.terms.editTerm") : t("business.terms.newTerm")}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
          <label className="field">
            <span className="field__label">{t("common.name")}</span>
            <input value={f.name} onChange={(e) => set("name", e.target.value)} placeholder="Net 30" autoFocus />
          </label>
          <label className="field">
            <span className="field__label">{t("common.description")}</span>
            <input value={f.description} onChange={(e) => set("description", e.target.value)} />
          </label>
          <label className="field">
            <span className="field__label">{t("business.terms.typeLabel")}</span>
            <select value={f.type} onChange={(e) => set("type", e.target.value as "days" | "proximo")}>
              <option value="days">{t("business.terms.netDays")}</option>
              <option value="proximo">{t("business.terms.proximo")}</option>
            </select>
          </label>
          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span className="field__label">{f.type === "days" ? t("business.terms.dueDays") : t("business.terms.dueDay")}</span>
              <input value={f.dueDays} onChange={(e) => set("dueDays", e.target.value)} />
            </label>
            {f.type === "proximo" && (
              <label className="field" style={{ flex: 1 }}>
                <span className="field__label">{t("business.terms.cutoffDay")}</span>
                <input value={f.cutoff} onChange={(e) => set("cutoff", e.target.value)} />
              </label>
            )}
          </div>
          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span className="field__label">{t("business.terms.discountDays")}</span>
              <input value={f.discountDays} onChange={(e) => set("discountDays", e.target.value)} />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span className="field__label">{t("business.terms.discountPct")}</span>
              <input value={f.discountPct} onChange={(e) => set("discountPct", e.target.value)} placeholder="0" />
            </label>
          </div>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>{t("common.cancel")}</button>
          <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? t("common.saving") : t("common.save")}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function BillTermsView({
  bookGuid,
  triggerNew,
}: {
  bookGuid: string;
  triggerNew: number;
}) {
  const { t } = useTranslation();
  const [terms, setTerms] = useState<BillTerm[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BillTerm | undefined>(undefined);

  function reload() {
    setError(null);
    api.listBillTerms(bookGuid)
      .then(setTerms)
      .catch((e) => setError(e instanceof Error ? e.message : t("business.failedToLoad")));
  }
  useEffect(reload, [bookGuid]);

  const seenTrigger = useRef(triggerNew);
  useEffect(() => {
    if (triggerNew > seenTrigger.current) {
      seenTrigger.current = triggerNew;
      setEditing(undefined);
      setFormOpen(true);
    }
  }, [triggerNew]);

  async function handleDelete(term: BillTerm) {
    if (!confirm(`Delete term "${term.name}"?`)) return;
    try {
      await api.deleteBillTerm(term.guid);
      setTerms((prev) => prev?.filter((x) => x.guid !== term.guid) ?? null);
    } catch (e) {
      alert(e instanceof Error ? e.message : t("business.deleteFailed"));
    }
  }

  function summary(term: BillTerm): string {
    if (term.type === "proximo") return t("business.terms.summaryProximo", { day: term.dueDays, cutoff: term.cutoff });
    return t("business.terms.summaryNet", { days: term.dueDays });
  }

  return (
    <>
      {error && <div style={{ padding: "0.75rem 1.5rem" }}><p className="error" style={{ margin: 0 }}>{error}</p></div>}
      {terms === null ? (
        <div className="empty"><span className="spinner" /></div>
      ) : terms.length === 0 ? (
        <div className="empty">{t("business.terms.noTerms")}</div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>{t("common.name")}</th>
              <th>{t("business.terms.termsCol")}</th>
              <th>{t("business.terms.discountCol")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {terms.map((term) => (
              <tr key={term.guid}>
                <td>{term.name}</td>
                <td style={{ color: "var(--ink-soft)" }}>{summary(term)}</td>
                <td className="mono" style={{ fontSize: "0.85rem" }}>
                  {term.discount && term.discount.num !== 0 ? `${discountToPercent(term.discount)}% / ${term.discountDays}d` : "—"}
                </td>
                <td style={{ whiteSpace: "nowrap", textAlign: "right" }}>
                  <button className="btn btn--ghost btn--xs" onClick={() => { setEditing(term); setFormOpen(true); }}>{t("common.edit")}</button>{" "}
                  <button className="btn btn--ghost btn--xs" onClick={() => handleDelete(term)}>{t("common.delete")}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {formOpen && (
        <BillTermDialog
          bookGuid={bookGuid}
          existing={editing}
          onClose={() => setFormOpen(false)}
          onSaved={() => { setFormOpen(false); reload(); }}
        />
      )}
    </>
  );
}
