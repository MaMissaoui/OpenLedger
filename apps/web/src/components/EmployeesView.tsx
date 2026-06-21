import { useEffect, useRef, useState } from "react";
import { api } from "../lib/api";
import type { Commodity, Employee, NewEmployee } from "../lib/types";
import { formatMoney, parseAmount } from "../lib/money";

// Employee directory: entity CRUD for people reimbursed via expense vouchers.
// Vouchers themselves are deferred — this is the record of who exists.
export default function EmployeesView({
  bookGuid,
  triggerNew,
  commodities,
}: {
  bookGuid: string;
  triggerNew: number;
  commodities: Commodity[];
}) {
  const [employees, setEmployees] = useState<Employee[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Employee | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);

  const currencies = commodities.filter((c) => c.namespace === "CURRENCY");
  const commodityMap = Object.fromEntries(commodities.map((c) => [c.guid, c.mnemonic]));

  function load() {
    setLoading(true);
    setError(null);
    api.listEmployees(bookGuid)
      .then(setEmployees)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"))
      .finally(() => setLoading(false));
  }

  useEffect(load, [bookGuid]);

  const seenTrigger = useRef(triggerNew);
  useEffect(() => {
    if (triggerNew > seenTrigger.current) {
      seenTrigger.current = triggerNew;
      setEditing(undefined);
      setFormOpen(true);
    }
  }, [triggerNew]);

  async function handleDelete(e: Employee) {
    if (!confirm(`Delete "${e.name}"?`)) return;
    try {
      await api.deleteEmployee(e.guid);
      setEmployees((prev) => prev?.filter((x) => x.guid !== e.guid) ?? null);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Delete failed");
    }
  }

  return (
    <>
      {error && (
        <div style={{ padding: "0.75rem 1.5rem" }}>
          <p className="error" style={{ margin: 0 }}>{error}</p>
        </div>
      )}

      {loading && <div className="empty"><span className="spinner" /></div>}

      {!loading && employees?.length === 0 && (
        <div className="empty">
          <span style={{ fontSize: "2.2rem", opacity: 0.25, lineHeight: 1 }}>🧑‍💼</span>
          <span style={{ fontWeight: 500, color: "var(--ink)" }}>No employees yet.</span>
          <span style={{ fontSize: "0.85rem" }}>
            Click <strong>+ New Employee</strong> to add one.
          </span>
        </div>
      )}

      {employees && employees.length > 0 && (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Username</th>
              <th>ID</th>
              <th style={{ textAlign: "right" }}>Rate</th>
              <th>Currency</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {employees.map((e) => (
              <tr key={e.guid} className={e.active ? "" : "row--muted"}>
                <td style={{ fontWeight: 500 }}>{e.name}</td>
                <td className="mono" style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>
                  {e.username || "—"}
                </td>
                <td className="mono" style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>
                  {e.id || "—"}
                </td>
                <td className="mono" style={{ textAlign: "right", fontSize: "0.85rem" }}>
                  {e.rate && e.rate.num !== 0 ? formatMoney(e.rate) : "—"}
                </td>
                <td className="mono" style={{ fontSize: "0.85rem" }}>
                  {commodityMap[e.currencyGuid] ?? "—"}
                </td>
                <td>
                  <span
                    style={{
                      display: "inline-block",
                      padding: "0.15rem 0.55rem",
                      borderRadius: "999px",
                      fontSize: "0.75rem",
                      fontWeight: 600,
                      background: e.active ? "rgba(26,127,55,0.12)" : "rgba(99,110,123,0.12)",
                      color: e.active ? "var(--forest-dark)" : "var(--ink-soft)",
                    }}
                  >
                    {e.active ? "Active" : "Inactive"}
                  </span>
                </td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => { setEditing(e); setFormOpen(true); }}
                  >Edit</button>{" "}
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => handleDelete(e)}
                    style={{ color: "var(--oxblood-soft)" }}
                  >Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {formOpen && (
        <EmployeeForm
          bookGuid={bookGuid}
          existing={editing}
          currencies={currencies}
          onClose={() => setFormOpen(false)}
          onSaved={() => { setFormOpen(false); load(); }}
        />
      )}
    </>
  );
}

function EmployeeForm({
  bookGuid,
  existing,
  currencies,
  onClose,
  onSaved,
}: {
  bookGuid: string;
  existing?: Employee;
  currencies: Commodity[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(existing?.name ?? "");
  const [username, setUsername] = useState(existing?.username ?? "");
  const [id, setId] = useState(existing?.id ?? "");
  const [rate, setRate] = useState(
    existing?.rate && existing.rate.num !== 0 ? String(existing.rate.num / existing.rate.denom) : "",
  );
  const [active, setActive] = useState(existing?.active ?? true);
  const [currencyGuid, setCurrencyGuid] = useState(existing?.currencyGuid ?? "");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function handleSave() {
    setError(null);
    if (!name.trim()) { setError("Name is required."); return; }
    if (!currencyGuid) { setError("Currency is required."); return; }
    let rateNum = undefined as NewEmployee["rate"];
    if (rate.trim() !== "") {
      const parsed = parseAmount(rate);
      if (!parsed) { setError("Rate must be a number."); return; }
      rateNum = parsed;
    }
    const input: NewEmployee = {
      name: name.trim(),
      username: username.trim() || undefined,
      id: id.trim() || undefined,
      active,
      currencyGuid,
      rate: rateNum,
    };
    setSaving(true);
    try {
      if (existing) await api.updateEmployee(existing.guid, input);
      else await api.createEmployee(bookGuid, input);
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose} onKeyDown={(e) => { if (e.key === "Escape") onClose(); }}>
      <div className="dialog" style={{ width: "min(480px, 96vw)" }} onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>{existing ? "Edit Employee" : "New Employee"}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>

        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}

          <label className="field">
            <span>Name *</span>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Ada Lovelace" autoFocus />
          </label>

          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span>Username</span>
              <input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="ada" />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span>Display ID</span>
              <input value={id} onChange={(e) => setId(e.target.value)} placeholder="EMP-0001" />
            </label>
          </div>

          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span>Hourly rate</span>
              <input value={rate} onChange={(e) => setRate(e.target.value)} placeholder="75.00" inputMode="decimal" />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span>Currency *</span>
              <select value={currencyGuid} onChange={(e) => setCurrencyGuid(e.target.value)}>
                <option value="">— select —</option>
                {currencies.map((c) => (
                  <option key={c.guid} value={c.guid}>{c.mnemonic}</option>
                ))}
              </select>
            </label>
          </div>

          <label className="field" style={{ flexDirection: "row", alignItems: "center", gap: "0.5rem", cursor: "pointer" }}>
            <input
              type="checkbox"
              style={{ width: "auto", cursor: "pointer" }}
              checked={active}
              onChange={(e) => setActive(e.target.checked)}
            />
            <span style={{ textTransform: "none", letterSpacing: 0, fontSize: "0.9rem", color: "var(--ink)" }}>Active</span>
          </label>
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
