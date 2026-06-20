import { useEffect, useRef, useState } from "react";

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

function toForm(t?: BillTerm): FormState {
  return {
    name: t?.name ?? "",
    description: t?.description ?? "",
    type: t?.type ?? "days",
    dueDays: t?.dueDays != null ? String(t.dueDays) : "30",
    discountDays: t?.discountDays != null ? String(t.discountDays) : "0",
    discountPct: discountToPercent(t?.discount),
    cutoff: t?.cutoff != null ? String(t.cutoff) : "0",
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
  const [f, setF] = useState<FormState>(toForm(existing));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function set<K extends keyof FormState>(k: K, v: FormState[K]) {
    setF((prev) => ({ ...prev, [k]: v }));
  }

  async function handleSave() {
    setError(null);
    if (!f.name.trim()) { setError("Name is required."); return; }
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
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>{existing ? "Edit Term" : "New Term"}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
          <label className="field">
            <span className="field__label">Name</span>
            <input value={f.name} onChange={(e) => set("name", e.target.value)} placeholder="Net 30" autoFocus />
          </label>
          <label className="field">
            <span className="field__label">Description</span>
            <input value={f.description} onChange={(e) => set("description", e.target.value)} />
          </label>
          <label className="field">
            <span className="field__label">Type</span>
            <select value={f.type} onChange={(e) => set("type", e.target.value as "days" | "proximo")}>
              <option value="days">Net days</option>
              <option value="proximo">Proximo (day of month)</option>
            </select>
          </label>
          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span className="field__label">{f.type === "days" ? "Due days" : "Due day-of-month"}</span>
              <input value={f.dueDays} onChange={(e) => set("dueDays", e.target.value)} />
            </label>
            {f.type === "proximo" && (
              <label className="field" style={{ flex: 1 }}>
                <span className="field__label">Cutoff day</span>
                <input value={f.cutoff} onChange={(e) => set("cutoff", e.target.value)} />
              </label>
            )}
          </div>
          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span className="field__label">Discount days</span>
              <input value={f.discountDays} onChange={(e) => set("discountDays", e.target.value)} />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span className="field__label">Discount %</span>
              <input value={f.discountPct} onChange={(e) => set("discountPct", e.target.value)} placeholder="0" />
            </label>
          </div>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? "Saving…" : "Save"}
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
  const [terms, setTerms] = useState<BillTerm[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BillTerm | undefined>(undefined);

  function reload() {
    setError(null);
    api.listBillTerms(bookGuid)
      .then(setTerms)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"));
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

  async function handleDelete(t: BillTerm) {
    if (!confirm(`Delete term "${t.name}"?`)) return;
    try {
      await api.deleteBillTerm(t.guid);
      setTerms((prev) => prev?.filter((x) => x.guid !== t.guid) ?? null);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  function summary(t: BillTerm): string {
    if (t.type === "proximo") return `Due ${t.dueDays} of the following month (cutoff ${t.cutoff})`;
    return `Net ${t.dueDays} days`;
  }

  return (
    <>
      {error && <div style={{ padding: "0.75rem 1.5rem" }}><p className="error" style={{ margin: 0 }}>{error}</p></div>}
      {terms === null ? (
        <div className="empty"><span className="spinner" /></div>
      ) : terms.length === 0 ? (
        <div className="empty">No payment terms yet. Create one to set invoice due dates automatically.</div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr><th>Name</th><th>Terms</th><th>Discount</th><th /></tr>
          </thead>
          <tbody>
            {terms.map((t) => (
              <tr key={t.guid}>
                <td>{t.name}</td>
                <td style={{ color: "var(--ink-soft)" }}>{summary(t)}</td>
                <td className="mono" style={{ fontSize: "0.85rem" }}>
                  {t.discount && t.discount.num !== 0 ? `${discountToPercent(t.discount)}% / ${t.discountDays}d` : "—"}
                </td>
                <td style={{ whiteSpace: "nowrap", textAlign: "right" }}>
                  <button className="btn btn--ghost btn--xs" onClick={() => { setEditing(t); setFormOpen(true); }}>Edit</button>{" "}
                  <button className="btn btn--ghost btn--xs" onClick={() => handleDelete(t)}>Delete</button>
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
