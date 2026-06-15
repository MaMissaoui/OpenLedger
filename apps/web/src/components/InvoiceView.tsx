import { useEffect, useRef, useState } from "react";
import { api } from "../lib/api";
import type { Account, Entry, Invoice, NewEntry, NewInvoice, Numeric } from "../lib/types";

// ── Helpers ───────────────────────────────────────────────────────────────────

function numericToFloat(n: Numeric): number {
  return n.num / n.denom;
}

function formatMoney(n: Numeric): string {
  return numericToFloat(n).toFixed(2);
}

function parseAmount(s: string, denom = 100): Numeric {
  const v = Math.round(parseFloat(s) * denom);
  return { num: isNaN(v) ? 0 : v, denom };
}

function today(): string {
  return new Date().toISOString().slice(0, 10);
}

// ── AccountSelect ─────────────────────────────────────────────────────────────

function AccountSelect({
  accounts,
  value,
  onChange,
  filter,
  placeholder,
}: {
  accounts: Account[];
  value: string;
  onChange: (guid: string) => void;
  filter?: (a: Account) => boolean;
  placeholder?: string;
}) {
  const opts = filter ? accounts.filter(filter) : accounts;
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder ?? "— select account —"}</option>
      {opts.map((a) => (
        <option key={a.guid} value={a.guid}>
          {a.name}
        </option>
      ))}
    </select>
  );
}

// ── Entry form (inline) ───────────────────────────────────────────────────────

interface EntryFormProps {
  invoiceGuid: string;
  invType: "invoice" | "bill";
  accounts: Account[];
  existing?: Entry;
  onSaved: () => void;
  onCancel: () => void;
}

function EntryForm({ invoiceGuid, invType, accounts, existing, onSaved, onCancel }: EntryFormProps) {
  const [description, setDescription] = useState(existing?.description ?? "");
  const [accountGuid, setAccountGuid] = useState(existing?.accountGuid ?? "");
  const [qty, setQty] = useState(existing ? String(numericToFloat(existing.quantity ?? { num: 1, denom: 1 })) : "1");
  const [price, setPrice] = useState(existing ? String(numericToFloat(existing.price)) : "");
  const [date, setDate] = useState(existing?.date ?? today());
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const accountFilter = invType === "invoice"
    ? (a: Account) => a.type === "INCOME" && !a.placeholder
    : (a: Account) => a.type === "EXPENSE" && !a.placeholder;

  async function handleSave() {
    setError(null);
    if (!accountGuid) { setError("Account is required."); return; }
    if (!price) { setError("Price is required."); return; }
    const input: NewEntry = {
      date,
      description,
      quantity: parseAmount(qty, 1000),
      accountGuid,
      price: parseAmount(price, 100),
    };
    setSaving(true);
    try {
      if (existing) {
        await api.updateEntry(existing.guid, input);
      } else {
        await api.addEntry(invoiceGuid, input);
      }
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <tr className="entry-form-row">
      <td>
        <input
          type="date"
          value={date}
          onChange={(e) => setDate(e.target.value)}
          style={{ fontSize: "0.85rem", width: "100%" }}
        />
      </td>
      <td>
        <input
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Description"
          autoFocus
          style={{ fontSize: "0.85rem", width: "100%" }}
        />
      </td>
      <td>
        <AccountSelect
          accounts={accounts}
          value={accountGuid}
          onChange={setAccountGuid}
          filter={accountFilter}
          placeholder={invType === "invoice" ? "— income account —" : "— expense account —"}
        />
      </td>
      <td>
        <input
          value={qty}
          onChange={(e) => setQty(e.target.value)}
          style={{ width: "5rem", textAlign: "right", fontSize: "0.85rem" }}
        />
      </td>
      <td>
        <input
          value={price}
          onChange={(e) => setPrice(e.target.value)}
          placeholder="0.00"
          style={{ width: "6rem", textAlign: "right", fontSize: "0.85rem" }}
        />
      </td>
      <td style={{ color: "var(--ink-soft)", fontSize: "0.85rem", textAlign: "right" }}>
        {price ? formatMoney(parseAmount(price, 100)) : "—"}
      </td>
      <td style={{ whiteSpace: "nowrap" }}>
        {error && <span className="error" style={{ fontSize: "0.78rem", display: "block" }}>{error}</span>}
        <button className="btn btn--primary btn--xs" onClick={handleSave} disabled={saving}>
          {saving ? "…" : "Save"}
        </button>{" "}
        <button className="btn btn--ghost btn--xs" onClick={onCancel}>Cancel</button>
      </td>
    </tr>
  );
}

// ── Post dialog ───────────────────────────────────────────────────────────────

function PostDialog({
  invoice,
  accounts,
  onClose,
  onPosted,
}: {
  invoice: Invoice;
  accounts: Account[];
  onClose: () => void;
  onPosted: () => void;
}) {
  const [postDate, setPostDate] = useState(today());
  const [dueDate, setDueDate] = useState("");
  const [postAccGuid, setPostAccGuid] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const accFilter = invoice.type === "invoice"
    ? (a: Account) => a.type === "RECEIVABLE" && !a.placeholder
    : (a: Account) => a.type === "PAYABLE" && !a.placeholder;

  async function handlePost() {
    setError(null);
    if (!postAccGuid) {
      setError(invoice.type === "invoice" ? "Select an A/R account." : "Select an A/P account.");
      return;
    }
    setSaving(true);
    try {
      await api.postInvoice(invoice.guid, postAccGuid, postDate || undefined, dueDate || undefined);
      onPosted();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Post failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" style={{ width: "min(420px, 96vw)" }} onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>Post {invoice.type === "invoice" ? "Invoice" : "Bill"}</h2>
          <button className="dialog__close" onClick={onClose}>×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span>Post Date</span>
              <input type="date" value={postDate} onChange={(e) => setPostDate(e.target.value)} />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span>Due Date</span>
              <input type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} />
            </label>
          </div>
          <label className="field">
            <span>{invoice.type === "invoice" ? "A/R Account *" : "A/P Account *"}</span>
            <AccountSelect
              accounts={accounts}
              value={postAccGuid}
              onChange={setPostAccGuid}
              filter={accFilter}
              placeholder={invoice.type === "invoice" ? "— receivable account —" : "— payable account —"}
            />
          </label>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handlePost} disabled={saving}>
            {saving ? "Posting…" : "Post to Ledger"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Invoice detail ────────────────────────────────────────────────────────────

function InvoiceDetail({
  invoice,
  accounts,
  onBack,
  onRefresh,
}: {
  invoice: Invoice;
  accounts: Account[];
  onBack: () => void;
  onRefresh: () => void;
}) {
  const [entries, setEntries] = useState<Entry[]>(invoice.entries ?? []);
  const [addingEntry, setAddingEntry] = useState(false);
  const [editingEntry, setEditingEntry] = useState<Entry | null>(null);
  const [showPost, setShowPost] = useState(false);
  const [loadingEntries, setLoadingEntries] = useState(false);

  async function reloadEntries() {
    setLoadingEntries(true);
    try {
      const list = await api.listEntries(invoice.guid);
      setEntries(list);
    } catch {
      // ignore
    } finally {
      setLoadingEntries(false);
    }
  }

  async function handleDeleteEntry(e: Entry) {
    if (!confirm(`Delete entry "${e.description || "this line"}"?`)) return;
    await api.deleteEntry(e.guid);
    reloadEntries();
  }

  const total = entries.reduce((sum, e) => sum + numericToFloat(e.lineTotal), 0);
  const isPosted = invoice.datePosted !== null;

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: "0.75rem", marginBottom: "1rem" }}>
        <button className="btn btn--ghost btn--sm" onClick={onBack}>← Back</button>
        <h2 style={{ margin: 0, fontSize: "1.1rem" }}>
          {invoice.type === "invoice" ? "Invoice" : "Bill"} {invoice.id || invoice.guid.slice(0, 8)}
        </h2>
        {isPosted ? (
          <span style={{ background: "rgba(26,127,55,0.12)", color: "var(--forest-dark)", borderRadius: "999px", padding: "0.15rem 0.55rem", fontSize: "0.75rem", fontWeight: 600 }}>
            Posted
          </span>
        ) : (
          <span style={{ background: "rgba(99,110,123,0.12)", color: "var(--ink-soft)", borderRadius: "999px", padding: "0.15rem 0.55rem", fontSize: "0.75rem", fontWeight: 600 }}>
            Draft
          </span>
        )}
        {!isPosted && (
          <button className="btn btn--primary btn--sm" style={{ marginLeft: "auto" }} onClick={() => setShowPost(true)}>
            Post to Ledger
          </button>
        )}
      </div>

      {invoice.notes && (
        <p style={{ color: "var(--ink-soft)", fontSize: "0.9rem", margin: "0 0 1rem" }}>{invoice.notes}</p>
      )}

      <table className="ledger-table" style={{ marginBottom: "0.5rem" }}>
        <thead>
          <tr>
            <th>Date</th>
            <th>Description</th>
            <th>Account</th>
            <th style={{ textAlign: "right" }}>Qty</th>
            <th style={{ textAlign: "right" }}>Price</th>
            <th style={{ textAlign: "right" }}>Total</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {entries.map((e) =>
            editingEntry?.guid === e.guid ? (
              <EntryForm
                key={e.guid}
                invoiceGuid={invoice.guid}
                invType={invoice.type as "invoice" | "bill"}
                accounts={accounts}
                existing={e}
                onSaved={() => { setEditingEntry(null); reloadEntries(); }}
                onCancel={() => setEditingEntry(null)}
              />
            ) : (
              <tr key={e.guid}>
                <td className="mono" style={{ fontSize: "0.85rem" }}>{e.date}</td>
                <td>{e.description || "—"}</td>
                <td style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>
                  {accounts.find((a) => a.guid === e.accountGuid)?.name ?? "—"}
                </td>
                <td style={{ textAlign: "right", fontSize: "0.85rem" }}>{numericToFloat(e.quantity ?? { num: 1, denom: 1 }).toFixed(3)}</td>
                <td style={{ textAlign: "right", fontSize: "0.85rem" }}>{formatMoney(e.price)}</td>
                <td style={{ textAlign: "right", fontWeight: 500 }}>{formatMoney(e.lineTotal)}</td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                  {!isPosted && (
                    <>
                      <button className="btn btn--ghost btn--xs" onClick={() => setEditingEntry(e)}>Edit</button>{" "}
                      <button className="btn btn--ghost btn--xs" style={{ color: "var(--oxblood-soft)" }} onClick={() => handleDeleteEntry(e)}>Delete</button>
                    </>
                  )}
                </td>
              </tr>
            ),
          )}
          {addingEntry && (
            <EntryForm
              invoiceGuid={invoice.guid}
              invType={invoice.type as "invoice" | "bill"}
              accounts={accounts}
              onSaved={() => { setAddingEntry(false); reloadEntries(); }}
              onCancel={() => setAddingEntry(false)}
            />
          )}
        </tbody>
        <tfoot>
          <tr>
            <td colSpan={5} style={{ textAlign: "right", fontWeight: 600, paddingTop: "0.5rem" }}>Total</td>
            <td style={{ textAlign: "right", fontWeight: 700, paddingTop: "0.5rem" }}>{total.toFixed(2)}</td>
            <td />
          </tr>
        </tfoot>
      </table>

      {!isPosted && !addingEntry && !editingEntry && (
        <button className="btn btn--ghost btn--sm" onClick={() => setAddingEntry(true)} style={{ marginTop: "0.25rem" }}>
          + Add Line
        </button>
      )}

      {loadingEntries && <div className="empty"><span className="spinner" /></div>}

      {showPost && (
        <PostDialog
          invoice={invoice}
          accounts={accounts}
          onClose={() => setShowPost(false)}
          onPosted={() => { setShowPost(false); onRefresh(); }}
        />
      )}
    </div>
  );
}

// ── Invoice form dialog ───────────────────────────────────────────────────────

function InvoiceFormDialog({
  bookGuid,
  invType,
  owners,
  existing,
  onClose,
  onSaved,
}: {
  bookGuid: string;
  invType: "invoice" | "bill";
  owners: Array<{ guid: string; name: string }>;
  existing?: Invoice;
  onClose: () => void;
  onSaved: (inv: Invoice) => void;
}) {
  const [id, setId] = useState(existing?.id ?? "");
  const [ownerGuid, setOwnerGuid] = useState(existing?.ownerGuid ?? "");
  const [dateOpened, setDateOpened] = useState(existing?.dateOpened ?? today());
  const [notes, setNotes] = useState(existing?.notes ?? "");
  const [currencyGuid, setCurrencyGuid] = useState(existing?.currencyGuid ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Load currencies for selector
  const [currencies, setCurrencies] = useState<Array<{ guid: string; mnemonic: string }>>([]);
  useEffect(() => {
    api.listCommodities()
      .then((list) => setCurrencies(list.filter((c) => c.namespace === "CURRENCY")))
      .catch(() => null);
  }, []);

  async function handleSave() {
    setError(null);
    if (!ownerGuid) { setError(`${invType === "invoice" ? "Customer" : "Vendor"} is required.`); return; }
    if (!currencyGuid) { setError("Currency is required."); return; }
    const input: NewInvoice = { id, type: invType, ownerGuid, dateOpened, notes, active: true, currencyGuid };
    setSaving(true);
    try {
      let result: Invoice;
      if (existing) {
        result = await api.updateInvoice(existing.guid, input);
      } else {
        result = await api.createInvoice(bookGuid, input);
      }
      onSaved(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  const entityLabel = invType === "invoice" ? "Customer" : "Vendor";
  const numPrefix = invType === "invoice" ? "INV" : "BILL";

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" style={{ width: "min(460px, 96vw)" }} onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>{existing ? "Edit" : "New"} {invType === "invoice" ? "Invoice" : "Bill"}</h2>
          <button className="dialog__close" onClick={onClose}>×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}

          <label className="field">
            <span>{entityLabel} *</span>
            <select value={ownerGuid} onChange={(e) => setOwnerGuid(e.target.value)}>
              <option value="">— select {entityLabel.toLowerCase()} —</option>
              {owners.map((o) => <option key={o.guid} value={o.guid}>{o.name}</option>)}
            </select>
          </label>

          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span>Number</span>
              <input value={id} onChange={(e) => setId(e.target.value)} placeholder={`${numPrefix}-0001`} />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span>Date</span>
              <input type="date" value={dateOpened} onChange={(e) => setDateOpened(e.target.value)} />
            </label>
          </div>

          <label className="field">
            <span>Currency *</span>
            <select value={currencyGuid} onChange={(e) => setCurrencyGuid(e.target.value)}>
              <option value="">— select currency —</option>
              {currencies.map((c) => <option key={c.guid} value={c.guid}>{c.mnemonic}</option>)}
            </select>
          </label>

          <label className="field">
            <span>Notes</span>
            <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} style={{ resize: "vertical" }} />
          </label>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? "Saving…" : existing ? "Save" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Invoice list ──────────────────────────────────────────────────────────────

export default function InvoiceView({
  bookGuid,
  invType,
  triggerNew,
  accounts,
}: {
  bookGuid: string;
  invType: "invoice" | "bill";
  triggerNew: number;
  accounts: Account[];
}) {
  const [invoices, setInvoices] = useState<Invoice[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Invoice | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Invoice | undefined>(undefined);

  // Owners: customers for invoices, vendors for bills
  const [owners, setOwners] = useState<Array<{ guid: string; name: string }>>([]);
  useEffect(() => {
    const fn = invType === "invoice" ? api.listCustomers : api.listVendors;
    fn(bookGuid).then((list) => setOwners(list)).catch(() => null);
  }, [bookGuid, invType]);

  function load() {
    setLoading(true);
    setError(null);
    api.listInvoices(bookGuid, invType)
      .then((rows) => setInvoices(rows))
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    setInvoices(null);
    setSelected(null);
    load();
  }, [bookGuid, invType]);

  const seenTrigger = useRef(triggerNew);
  useEffect(() => {
    if (triggerNew > seenTrigger.current) {
      seenTrigger.current = triggerNew;
      setEditing(undefined);
      setFormOpen(true);
    }
  }, [triggerNew]);

  async function handleDelete(inv: Invoice) {
    if (!confirm(`Delete ${invType === "invoice" ? "invoice" : "bill"} "${inv.id || inv.guid.slice(0, 8)}"?`)) return;
    try {
      await api.deleteInvoice(inv.guid);
      setInvoices((prev) => prev?.filter((i) => i.guid !== inv.guid) ?? null);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  // If viewing a specific invoice, show detail
  if (selected) {
    return (
      <div style={{ padding: "1rem 1.5rem" }}>
        <InvoiceDetail
          invoice={selected}
          accounts={accounts}
          onBack={() => setSelected(null)}
          onRefresh={() => {
            api.getInvoice(selected.guid)
              .then((inv) => setSelected(inv))
              .catch(() => setSelected(null));
            load();
          }}
        />
      </div>
    );
  }

  const ownerMap = Object.fromEntries(owners.map((o) => [o.guid, o.name]));

  return (
    <>
      {error && <div style={{ padding: "0.75rem 1.5rem" }}><p className="error" style={{ margin: 0 }}>{error}</p></div>}

      {loading && <div className="empty"><span className="spinner" /></div>}

      {!loading && invoices?.length === 0 && (
        <div className="empty">
          <span style={{ fontSize: "2.2rem", opacity: 0.25, lineHeight: 1 }}>
            {invType === "invoice" ? "🧾" : "📄"}
          </span>
          <span style={{ fontWeight: 500, color: "var(--ink)" }}>
            No {invType === "invoice" ? "invoices" : "bills"} yet.
          </span>
          <span style={{ fontSize: "0.85rem" }}>
            Click <strong>+ New {invType === "invoice" ? "Invoice" : "Bill"}</strong> to create one.
          </span>
        </div>
      )}

      {invoices && invoices.length > 0 && (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>Number</th>
              <th>{invType === "invoice" ? "Customer" : "Vendor"}</th>
              <th>Date</th>
              <th>Due</th>
              <th style={{ textAlign: "right" }}>Total</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {invoices.map((inv) => (
              <tr key={inv.guid} style={{ cursor: "pointer" }} onClick={() => setSelected(inv)}>
                <td className="mono" style={{ fontSize: "0.85rem" }}>
                  {inv.id || inv.guid.slice(0, 8)}
                </td>
                <td style={{ fontWeight: 500 }}>
                  {ownerMap[inv.ownerGuid] ?? <span style={{ color: "var(--ink-soft)" }}>—</span>}
                </td>
                <td className="mono" style={{ fontSize: "0.85rem" }}>{inv.dateOpened}</td>
                <td className="mono" style={{ fontSize: "0.85rem", color: "var(--ink-soft)" }}>
                  {inv.dateDue ?? "—"}
                </td>
                <td style={{ textAlign: "right", fontWeight: 500 }}>
                  {inv.entries?.length
                    ? inv.entries.reduce((s, e) => s + numericToFloat(e.lineTotal), 0).toFixed(2)
                    : "—"}
                </td>
                <td>
                  {inv.datePosted ? (
                    <span style={{ background: "rgba(26,127,55,0.12)", color: "var(--forest-dark)", borderRadius: "999px", padding: "0.15rem 0.55rem", fontSize: "0.75rem", fontWeight: 600 }}>
                      Posted
                    </span>
                  ) : (
                    <span style={{ background: "rgba(99,110,123,0.12)", color: "var(--ink-soft)", borderRadius: "999px", padding: "0.15rem 0.55rem", fontSize: "0.75rem", fontWeight: 600 }}>
                      Draft
                    </span>
                  )}
                </td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }} onClick={(e) => e.stopPropagation()}>
                  <button className="btn btn--ghost btn--xs" onClick={() => { setEditing(inv); setFormOpen(true); }}>Edit</button>{" "}
                  {!inv.datePosted && (
                    <button className="btn btn--ghost btn--xs" style={{ color: "var(--oxblood-soft)" }} onClick={() => handleDelete(inv)}>Delete</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {formOpen && (
        <InvoiceFormDialog
          bookGuid={bookGuid}
          invType={invType}
          owners={owners}
          existing={editing}
          onClose={() => setFormOpen(false)}
          onSaved={(inv) => {
            setFormOpen(false);
            load();
            // Open the new invoice for editing entries
            if (!editing) setSelected(inv);
          }}
        />
      )}
    </>
  );
}
