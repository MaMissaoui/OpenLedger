import { useEffect, useRef, useState } from "react";

import { api } from "../lib/api";
import { parseAmount, toFloat } from "../lib/money";
import type { Account, NewTaxTable, TaxTable, TaxTableEntry } from "../lib/types";

interface EntryRow {
  accountGuid: string;
  type: "percentage" | "value";
  amount: string;
}

function toRows(tt?: TaxTable): EntryRow[] {
  if (!tt || tt.entries.length === 0) {
    return [{ accountGuid: "", type: "percentage", amount: "" }];
  }
  return tt.entries.map((e) => ({
    accountGuid: e.accountGuid,
    type: e.type,
    amount: String(toFloat(e.amount)),
  }));
}

function TaxTableDialog({
  bookGuid,
  accounts,
  existing,
  onSaved,
  onClose,
}: {
  bookGuid: string;
  accounts: Account[];
  existing?: TaxTable;
  onSaved: () => void;
  onClose: () => void;
}) {
  const [name, setName] = useState(existing?.name ?? "");
  const [rows, setRows] = useState<EntryRow[]>(toRows(existing));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const taxAccounts = accounts.filter((a) => !a.placeholder && a.type !== "ROOT");

  function setRow(i: number, patch: Partial<EntryRow>) {
    setRows((prev) => prev.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  }
  function addRow() {
    setRows((prev) => [...prev, { accountGuid: "", type: "percentage", amount: "" }]);
  }
  function removeRow(i: number) {
    setRows((prev) => (prev.length === 1 ? prev : prev.filter((_, j) => j !== i)));
  }

  async function handleSave() {
    setError(null);
    if (!name.trim()) { setError("Name is required."); return; }
    const entries: TaxTableEntry[] = [];
    for (const r of rows) {
      if (!r.accountGuid) { setError("Every entry needs a tax account."); return; }
      const amount = parseAmount(r.amount, 100);
      if (amount === null) { setError("Every entry needs a valid amount."); return; }
      entries.push({ accountGuid: r.accountGuid, type: r.type, amount });
    }
    const input: NewTaxTable = { name: name.trim(), entries };
    setSaving(true);
    try {
      if (existing) await api.updateTaxTable(existing.guid, input);
      else await api.createTaxTable(bookGuid, input);
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
          <h2>{existing ? "Edit Tax Table" : "New Tax Table"}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
          <label className="field">
            <span className="field__label">Name</span>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="VAT 20%" autoFocus />
          </label>
          <div className="field">
            <span className="field__label">Components</span>
            <table className="ledger-table" style={{ fontSize: "0.85rem" }}>
              <thead>
                <tr><th>Tax account</th><th>Type</th><th style={{ textAlign: "right" }}>Rate / amount</th><th /></tr>
              </thead>
              <tbody>
                {rows.map((r, i) => (
                  <tr key={i}>
                    <td>
                      <select value={r.accountGuid} onChange={(e) => setRow(i, { accountGuid: e.target.value })}>
                        <option value="">— tax account —</option>
                        {taxAccounts.map((a) => (
                          <option key={a.guid} value={a.guid}>{a.name}</option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <select value={r.type} onChange={(e) => setRow(i, { type: e.target.value as "percentage" | "value" })}>
                        <option value="percentage">Percentage</option>
                        <option value="value">Flat value</option>
                      </select>
                    </td>
                    <td>
                      <input
                        value={r.amount}
                        onChange={(e) => setRow(i, { amount: e.target.value })}
                        placeholder={r.type === "percentage" ? "20" : "0.00"}
                        style={{ width: "5rem", textAlign: "right" }}
                      />
                    </td>
                    <td style={{ textAlign: "right" }}>
                      <button className="btn btn--ghost btn--xs" onClick={() => removeRow(i)} disabled={rows.length === 1}>×</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <button className="btn btn--ghost btn--xs" onClick={addRow} style={{ marginTop: "0.5rem" }}>+ Add component</button>
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

export default function TaxTablesView({
  bookGuid,
  accounts,
  triggerNew,
}: {
  bookGuid: string;
  accounts: Account[];
  triggerNew: number;
}) {
  const [tables, setTables] = useState<TaxTable[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<TaxTable | undefined>(undefined);

  const accountMap = Object.fromEntries(accounts.map((a) => [a.guid, a.name]));

  function reload() {
    setError(null);
    api.listTaxTables(bookGuid)
      .then(setTables)
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

  async function handleDelete(tt: TaxTable) {
    if (!confirm(`Delete tax table "${tt.name}"?`)) return;
    try {
      await api.deleteTaxTable(tt.guid);
      setTables((prev) => prev?.filter((x) => x.guid !== tt.guid) ?? null);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  function summary(tt: TaxTable): string {
    return tt.entries
      .map((e) => (e.type === "percentage" ? `${toFloat(e.amount)}%` : `${toFloat(e.amount)} flat`) + ` → ${accountMap[e.accountGuid] ?? "?"}`)
      .join(", ") || "no components";
  }

  return (
    <>
      {error && <div style={{ padding: "0.75rem 1.5rem" }}><p className="error" style={{ margin: 0 }}>{error}</p></div>}
      {tables === null ? (
        <div className="empty"><span className="spinner" /></div>
      ) : tables.length === 0 ? (
        <div className="empty">No tax tables yet. Create one to apply tax to invoice and bill lines.</div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr><th>Name</th><th>Components</th><th /></tr>
          </thead>
          <tbody>
            {tables.map((tt) => (
              <tr key={tt.guid}>
                <td>{tt.name}</td>
                <td style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>{summary(tt)}</td>
                <td style={{ whiteSpace: "nowrap", textAlign: "right" }}>
                  <button className="btn btn--ghost btn--xs" onClick={() => { setEditing(tt); setFormOpen(true); }}>Edit</button>{" "}
                  <button className="btn btn--ghost btn--xs" onClick={() => handleDelete(tt)}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {formOpen && (
        <TaxTableDialog
          bookGuid={bookGuid}
          accounts={accounts}
          existing={editing}
          onClose={() => setFormOpen(false)}
          onSaved={() => { setFormOpen(false); reload(); }}
        />
      )}
    </>
  );
}
