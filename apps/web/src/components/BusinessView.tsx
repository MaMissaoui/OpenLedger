import { useEffect, useState } from "react";
import { api } from "../lib/api";
import type { Customer, NewCustomer, NewVendor, Vendor } from "../lib/types";

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
    if (!name) { setError("Name is required."); return; }
    const input = buildInput({ name, id, notes, active, currencyGuid });
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

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" style={{ width: "min(440px, 100%)" }} onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>{existing ? `Edit ${entityLabel}` : `New ${entityLabel}`}</h2>
          <button className="dialog__close" onClick={onClose}>×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error">{error}</p>}
          <label className="field"><span>Name *</span>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Acme Corp" autoFocus />
          </label>
          <label className="field"><span>Display ID</span>
            <input value={id} onChange={(e) => setId(e.target.value)} placeholder="CUST-0001" />
          </label>
          <label className="field"><span>Currency GUID</span>
            <input value={currencyGuid} onChange={(e) => setCurrencyGuid(e.target.value)} placeholder="32-char commodity GUID" />
          </label>
          <label className="field"><span>Notes</span>
            <input value={notes} onChange={(e) => setNotes(e.target.value)} />
          </label>
          <label className="field" style={{ flexDirection: "row", alignItems: "center", gap: "0.5rem" }}>
            <input type="checkbox" style={{ width: "auto" }} checked={active} onChange={(e) => setActive(e.target.checked)} />
            <span>Active</span>
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

// ── Shared contact list component ─────────────────────────────────────────────

interface ContactListProps<C extends Customer | Vendor, N extends NewCustomer | NewVendor> {
  bookGuid: string;
  entityLabel: string;
  triggerNew: number;
  loadFn: (bookGuid: string) => Promise<C[]>;
  deleteFn: (guid: string) => Promise<void>;
  createFn: (bookGuid: string, input: N) => Promise<Customer | Vendor>;
  updateFn: (guid: string, input: N) => Promise<Customer | Vendor>;
  buildInput: (f: ContactFields) => N;
}

function ContactList<C extends Customer | Vendor, N extends NewCustomer | NewVendor>({
  bookGuid,
  entityLabel,
  triggerNew,
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

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    loadFn(bookGuid).then((rows) => {
      if (!cancelled) setContacts(rows as C[]);
    }).catch((e) => {
      if (!cancelled) setError(e instanceof Error ? e.message : "Failed to load");
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [bookGuid]);

  // Open new form when parent increments triggerNew
  useEffect(() => {
    if (triggerNew > 0) { setEditing(undefined); setFormOpen(true); }
  }, [triggerNew]);

  async function handleDelete(guid: string) {
    if (!confirm(`Delete this ${entityLabel.toLowerCase()}?`)) return;
    try {
      await deleteFn(guid);
      setContacts((prev) => prev?.filter((c) => c.guid !== guid) ?? null);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  return (
    <>
      {error && <p className="error" style={{ padding: "0.75rem 1rem" }}>{error}</p>}
      {loading && <div className="empty"><span className="spinner" /></div>}
      {!loading && contacts?.length === 0 && (
        <div className="empty">No {entityLabel.toLowerCase()}s yet. Click + New to add one.</div>
      )}
      {contacts && contacts.length > 0 && (
        <table className="ledger-table">
          <thead>
            <tr><th>Name</th><th>ID</th><th>Notes</th><th>Active</th><th></th></tr>
          </thead>
          <tbody>
            {contacts.map((c) => (
              <tr key={c.guid}>
                <td>{c.name}</td>
                <td className="mono">{c.id}</td>
                <td>{c.notes}</td>
                <td>{c.active ? "Yes" : "No"}</td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                  <button className="btn btn--ghost btn--xs"
                    onClick={() => { setEditing(c); setFormOpen(true); }}>Edit</button>{" "}
                  <button className="btn btn--ghost btn--xs"
                    onClick={() => handleDelete(c.guid)}
                    style={{ color: "var(--oxblood-soft)" }}>Delete</button>
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
          onSaved={() => { setFormOpen(false); setContacts(null); loadFn(bookGuid).then((rows) => setContacts(rows as C[])).catch(() => null); }}
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

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">Business</div>
          <h2>{tab === "customers" ? "Customers" : "Vendors"}</h2>
        </div>
        <div className="register__actions">
          <button
            className={`btn btn--ghost btn--sm${tab === "customers" ? " btn--active" : ""}`}
            onClick={() => setTab("customers")}
          >Customers</button>
          <button
            className={`btn btn--ghost btn--sm${tab === "vendors" ? " btn--active" : ""}`}
            onClick={() => setTab("vendors")}
          >Vendors</button>
          <button
            className="btn btn--primary btn--sm"
            onClick={() => setNewTrigger((n) => n + 1)}
          >+ New</button>
        </div>
      </header>

      {tab === "customers" && (
        <ContactList<Customer, NewCustomer>
          bookGuid={bookGuid}
          entityLabel="Customer"
          triggerNew={newTrigger}
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
