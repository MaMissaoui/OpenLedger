import { useEffect, useRef, useState } from "react";
import { api } from "../lib/api";
import type { Commodity, Customer, NewCustomer, NewVendor, Vendor } from "../lib/types";

// ── Commodity select — loads its own list when rendered ───────────────────────

function CommoditySelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (guid: string) => void;
}) {
  const [commodities, setCommodities] = useState<Commodity[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.listCommodities()
      .then((list) => setCommodities(list.filter((c) => c.namespace === "CURRENCY")))
      .catch(() => null)
      .finally(() => setLoading(false));
  }, []);

  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} disabled={loading}>
      <option value="">{loading ? "Loading…" : "— select currency —"}</option>
      {commodities.map((c) => (
        <option key={c.guid} value={c.guid}>
          {c.mnemonic}
        </option>
      ))}
    </select>
  );
}

// ── Contact form dialog ───────────────────────────────────────────────────────

interface ContactFields {
  name: string;
  id: string;
  notes: string;
  active: boolean;
  currencyGuid: string;
}

interface ContactFormProps<T extends NewCustomer | NewVendor> {
  bookGuid: string;
  entityLabel: string;
  existing?: Customer | Vendor;
  onClose: () => void;
  onSaved: () => void;
  createFn: (bookGuid: string, input: T) => Promise<Customer | Vendor>;
  updateFn: (guid: string, input: T) => Promise<Customer | Vendor>;
  buildInput: (fields: ContactFields) => T;
}

function ContactForm<T extends NewCustomer | NewVendor>({
  bookGuid,
  entityLabel,
  existing,
  onClose,
  onSaved,
  createFn,
  updateFn,
  buildInput,
}: ContactFormProps<T>) {
  const [name, setName] = useState(existing?.name ?? "");
  const [id, setId] = useState(existing?.id ?? "");
  const [notes, setNotes] = useState(existing?.notes ?? "");
  const [active, setActive] = useState(existing?.active ?? true);
  const [currencyGuid, setCurrencyGuid] = useState(existing?.currencyGuid ?? "");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function handleSave() {
    setError(null);
    if (!name.trim()) { setError("Name is required."); return; }
    const input = buildInput({ name: name.trim(), id: id.trim(), notes: notes.trim(), active, currencyGuid });
    setSaving(true);
    try {
      if (existing) { await updateFn(existing.guid, input); }
      else { await createFn(bookGuid, input); }
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  function handleKey(e: React.KeyboardEvent) {
    if (e.key === "Escape") onClose();
  }

  return (
    <div className="dialog-overlay" onClick={onClose} onKeyDown={handleKey}>
      <div
        className="dialog"
        style={{ width: "min(480px, 96vw)" }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="dialog__header">
          <h2>{existing ? `Edit ${entityLabel}` : `New ${entityLabel}`}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>

        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}

          <label className="field">
            <span>Name *</span>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={entityLabel === "Customer" ? "Acme Corp" : "Office Supplies Ltd"}
              autoFocus
            />
          </label>

          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span>Display ID</span>
              <input
                value={id}
                onChange={(e) => setId(e.target.value)}
                placeholder={entityLabel === "Customer" ? "CUST-0001" : "VEND-0001"}
              />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span>Currency</span>
              <CommoditySelect value={currencyGuid} onChange={setCurrencyGuid} />
            </label>
          </div>

          <label className="field">
            <span>Notes</span>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              rows={3}
              style={{ resize: "vertical" }}
            />
          </label>

          <label
            className="field"
            style={{ flexDirection: "row", alignItems: "center", gap: "0.5rem", cursor: "pointer" }}
          >
            <input
              type="checkbox"
              style={{ width: "auto", cursor: "pointer" }}
              checked={active}
              onChange={(e) => setActive(e.target.checked)}
            />
            <span style={{ textTransform: "none", letterSpacing: 0, fontSize: "0.9rem", color: "var(--ink)" }}>
              Active
            </span>
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

// ── Contact list ──────────────────────────────────────────────────────────────

interface ContactListProps<C extends Customer | Vendor, N extends NewCustomer | NewVendor> {
  bookGuid: string;
  entityLabel: string;
  triggerNew: number;
  commodities: Commodity[];
  loadFn: (bookGuid: string) => Promise<C[]>;
  deleteFn: (guid: string) => Promise<void>;
  createFn: (bookGuid: string, input: N) => Promise<Customer | Vendor>;
  updateFn: (guid: string, input: N) => Promise<Customer | Vendor>;
  buildInput: (f: ContactFields) => N;
}

function ActiveBadge({ active }: { active: boolean }) {
  return (
    <span
      style={{
        display: "inline-block",
        padding: "0.15rem 0.55rem",
        borderRadius: "999px",
        fontSize: "0.75rem",
        fontWeight: 600,
        letterSpacing: "0.03em",
        background: active ? "rgba(26,127,55,0.12)" : "rgba(99,110,123,0.12)",
        color: active ? "var(--forest-dark)" : "var(--ink-soft)",
      }}
    >
      {active ? "Active" : "Inactive"}
    </span>
  );
}

function ContactList<C extends Customer | Vendor, N extends NewCustomer | NewVendor>({
  bookGuid,
  entityLabel,
  triggerNew,
  commodities,
  loadFn,
  deleteFn,
  createFn,
  updateFn,
  buildInput,
}: ContactListProps<C, N>) {
  const [contacts, setContacts] = useState<C[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<C | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);

  // Map guid → mnemonic for the table
  const commodityMap = Object.fromEntries(commodities.map((c) => [c.guid, c.mnemonic]));

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    loadFn(bookGuid)
      .then((rows) => { if (!cancelled) setContacts(rows as C[]); })
      .catch((e) => { if (!cancelled) setError(e instanceof Error ? e.message : "Failed to load"); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [bookGuid]);

  // Open form when parent increments counter, but not on initial mount (value is 0).
  const seenTrigger = useRef(triggerNew);
  useEffect(() => {
    if (triggerNew > seenTrigger.current) {
      seenTrigger.current = triggerNew;
      setEditing(undefined);
      setFormOpen(true);
    }
  }, [triggerNew]);

  async function handleDelete(c: C) {
    if (!confirm(`Delete "${c.name}"?`)) return;
    try {
      await deleteFn(c.guid);
      setContacts((prev) => prev?.filter((x) => x.guid !== c.guid) ?? null);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  function reload() {
    setContacts(null);
    loadFn(bookGuid)
      .then((rows) => setContacts(rows as C[]))
      .catch(() => null);
  }

  return (
    <>
      {error && (
        <div style={{ padding: "0.75rem 1.5rem" }}>
          <p className="error" style={{ margin: 0 }}>{error}</p>
        </div>
      )}

      {loading && (
        <div className="empty"><span className="spinner" /></div>
      )}

      {!loading && contacts?.length === 0 && (
        <div className="empty">
          <span style={{ fontSize: "2.2rem", opacity: 0.25, lineHeight: 1 }}>
            {entityLabel === "Customer" ? "👤" : "🏪"}
          </span>
          <span style={{ fontWeight: 500, color: "var(--ink)" }}>No {entityLabel.toLowerCase()}s yet.</span>
          <span style={{ fontSize: "0.85rem" }}>
            Click <strong>+ New {entityLabel}</strong> to add one.
          </span>
        </div>
      )}

      {contacts && contacts.length > 0 && (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>ID</th>
              <th>Currency</th>
              <th>Notes</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {contacts.map((c) => (
              <tr key={c.guid} className={c.active ? "" : "row--muted"}>
                <td style={{ fontWeight: 500 }}>{c.name}</td>
                <td className="mono" style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>
                  {c.id || "—"}
                </td>
                <td className="mono" style={{ fontSize: "0.85rem" }}>
                  {commodityMap[c.currencyGuid] ?? "—"}
                </td>
                <td style={{ color: "var(--ink-soft)", fontSize: "0.88rem", maxWidth: "18rem" }}>
                  <span style={{ display: "block", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {c.notes || "—"}
                  </span>
                </td>
                <td><ActiveBadge active={c.active} /></td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => { setEditing(c); setFormOpen(true); }}
                  >Edit</button>{" "}
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => handleDelete(c)}
                    style={{ color: "var(--oxblood-soft)" }}
                  >Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {formOpen && (
        <ContactForm<N>
          bookGuid={bookGuid}
          entityLabel={entityLabel}
          existing={editing}
          onClose={() => setFormOpen(false)}
          onSaved={() => { setFormOpen(false); reload(); }}
          createFn={createFn as (bg: string, i: N) => Promise<Customer | Vendor>}
          updateFn={updateFn as (g: string, i: N) => Promise<Customer | Vendor>}
          buildInput={buildInput}
        />
      )}
    </>
  );
}

// ── Top-level BusinessView ────────────────────────────────────────────────────

type BizTab = "customers" | "vendors";

export default function BusinessView({ bookGuid }: { bookGuid: string }) {
  const [tab, setTab] = useState<BizTab>("customers");
  const [newTrigger, setNewTrigger] = useState(0);
  const [commodities, setCommodities] = useState<Commodity[]>([]);

  useEffect(() => {
    api.listCommodities().then(setCommodities).catch(() => null);
  }, []);

  function handleNew() {
    setNewTrigger((n) => n + 1);
  }

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">Business</div>
          <h1>{tab === "customers" ? "Customers" : "Vendors"}</h1>
        </div>
        <div className="register__actions">
          {/* Tab switcher */}
          <div className="biz-tabs">
            <button
              className={`biz-tab${tab === "customers" ? " biz-tab--active" : ""}`}
              onClick={() => setTab("customers")}
            >Customers</button>
            <button
              className={`biz-tab${tab === "vendors" ? " biz-tab--active" : ""}`}
              onClick={() => setTab("vendors")}
            >Vendors</button>
          </div>
          <button className="btn btn--primary btn--sm" onClick={handleNew}>
            + New {tab === "customers" ? "Customer" : "Vendor"}
          </button>
        </div>
      </header>

      {tab === "customers" && (
        <ContactList<Customer, NewCustomer>
          bookGuid={bookGuid}
          entityLabel="Customer"
          triggerNew={newTrigger}
          commodities={commodities}
          loadFn={api.listCustomers}
          deleteFn={api.deleteCustomer}
          createFn={api.createCustomer}
          updateFn={api.updateCustomer}
          buildInput={(f) => ({
            name: f.name,
            id: f.id || undefined,
            notes: f.notes || undefined,
            active: f.active,
            currencyGuid: f.currencyGuid,
          })}
        />
      )}
      {tab === "vendors" && (
        <ContactList<Vendor, NewVendor>
          bookGuid={bookGuid}
          entityLabel="Vendor"
          triggerNew={newTrigger}
          commodities={commodities}
          loadFn={api.listVendors}
          deleteFn={api.deleteVendor}
          createFn={api.createVendor}
          updateFn={api.updateVendor}
          buildInput={(f) => ({
            name: f.name,
            id: f.id || undefined,
            notes: f.notes || undefined,
            active: f.active,
            currencyGuid: f.currencyGuid,
          })}
        />
      )}
    </section>
  );
}
