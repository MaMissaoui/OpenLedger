import { useEffect, useRef, useState } from "react";
import { api } from "../lib/api";
import type { Job, NewJob } from "../lib/types";

type Owner = { guid: string; name: string; id?: string };

// Jobs group invoices/bills under one customer or vendor. Entity CRUD only —
// attaching a job to an invoice is left to the invoice editor later.
export default function JobsView({
  bookGuid,
  triggerNew,
  customers,
  vendors,
}: {
  bookGuid: string;
  triggerNew: number;
  customers: Owner[];
  vendors: Owner[];
}) {
  const [jobs, setJobs] = useState<Job[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Job | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  const ownerName = (j: Job) => {
    const pool = j.ownerType === "customer" ? customers : vendors;
    const o = pool.find((x) => x.guid === j.ownerGuid);
    if (!o) return j.ownerGuid;
    return o.id ? `${o.id} — ${o.name}` : o.name;
  };

  function load() {
    setLoading(true);
    setError(null);
    api.listJobs(bookGuid)
      .then(setJobs)
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

  async function handleDelete(j: Job) {
    if (!confirm(`Delete "${j.name}"?`)) return;
    try {
      await api.deleteJob(j.guid);
      setJobs((prev) => prev?.filter((x) => x.guid !== j.guid) ?? null);
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

      {!loading && jobs?.length === 0 && (
        <div className="empty">
          <span style={{ fontSize: "2.2rem", opacity: 0.25, lineHeight: 1 }}>📋</span>
          <span style={{ fontWeight: 500, color: "var(--ink)" }}>No jobs yet.</span>
          <span style={{ fontSize: "0.85rem" }}>
            Click <strong>+ New Job</strong> to add one.
          </span>
        </div>
      )}

      {jobs && jobs.length > 0 && (() => {
        const filtered = query
          ? jobs.filter((j) => {
              const q = query.toLowerCase();
              return j.name.toLowerCase().includes(q) ||
                (j.id ?? "").toLowerCase().includes(q) ||
                ownerName(j).toLowerCase().includes(q);
            })
          : jobs;
        return (
          <>
            <div style={{ padding: "0.5rem 1.5rem 0", display: "flex", gap: "0.5rem" }}>
              <input
                type="search"
                placeholder="Filter by name, ID, or owner…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                style={{ maxWidth: "20rem" }}
              />
            </div>
            <table className="ledger-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>ID</th>
              <th>Owner</th>
              <th>Reference</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {filtered.map((j) => (
              <tr key={j.guid} className={j.active ? "" : "row--muted"}>
                <td style={{ fontWeight: 500 }}>{j.name}</td>
                <td className="mono" style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>{j.id || "—"}</td>
                <td style={{ fontSize: "0.88rem" }}>
                  {ownerName(j)}{" "}
                  <span style={{ color: "var(--ink-soft)", fontSize: "0.75rem" }}>({j.ownerType})</span>
                </td>
                <td className="mono" style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>{j.reference || "—"}</td>
                <td>
                  <span
                    style={{
                      display: "inline-block",
                      padding: "0.15rem 0.55rem",
                      borderRadius: "999px",
                      fontSize: "0.75rem",
                      fontWeight: 600,
                      background: j.active ? "rgba(26,127,55,0.12)" : "rgba(99,110,123,0.12)",
                      color: j.active ? "var(--forest-dark)" : "var(--ink-soft)",
                    }}
                  >
                    {j.active ? "Active" : "Inactive"}
                  </span>
                </td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => { setEditing(j); setFormOpen(true); }}
                  >Edit</button>{" "}
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => handleDelete(j)}
                    style={{ color: "var(--oxblood-soft)" }}
                  >Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
          </>
        );
      })()}

      {formOpen && (
        <JobForm
          bookGuid={bookGuid}
          existing={editing}
          customers={customers}
          vendors={vendors}
          onClose={() => setFormOpen(false)}
          onSaved={() => { setFormOpen(false); load(); }}
        />
      )}
    </>
  );
}

function JobForm({
  bookGuid,
  existing,
  customers,
  vendors,
  onClose,
  onSaved,
}: {
  bookGuid: string;
  existing?: Job;
  customers: Owner[];
  vendors: Owner[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(existing?.name ?? "");
  const [id, setId] = useState(existing?.id ?? "");
  const [reference, setReference] = useState(existing?.reference ?? "");
  const [active, setActive] = useState(existing?.active ?? true);
  // owner = "<type>:<guid>"; fixed once the job exists.
  const [owner, setOwner] = useState(existing ? `${existing.ownerType}:${existing.ownerGuid}` : "");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function handleSave() {
    setError(null);
    if (!name.trim()) { setError("Name is required."); return; }
    if (!existing && !owner) { setError("Owner is required."); return; }
    const [ownerType, ownerGuid] = owner.split(":") as ["customer" | "vendor", string];
    const input: NewJob = {
      name: name.trim(),
      id: id.trim() || undefined,
      reference: reference.trim() || undefined,
      active,
      ownerType,
      ownerGuid,
    };
    setSaving(true);
    try {
      if (existing) await api.updateJob(existing.guid, input);
      else await api.createJob(bookGuid, input);
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
          <h2>{existing ? "Edit Job" : "New Job"}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>

        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}

          <label className="field">
            <span>Name *</span>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Website rebuild" autoFocus />
          </label>

          <label className="field">
            <span>Owner *</span>
            <select value={owner} onChange={(e) => setOwner(e.target.value)} disabled={!!existing}>
              <option value="">— select customer or vendor —</option>
              {customers.length > 0 && (
                <optgroup label="Customers">
                  {customers.map((c) => (
                    <option key={c.guid} value={`customer:${c.guid}`}>{c.id ? `${c.id} — ${c.name}` : c.name}</option>
                  ))}
                </optgroup>
              )}
              {vendors.length > 0 && (
                <optgroup label="Vendors">
                  {vendors.map((v) => (
                    <option key={v.guid} value={`vendor:${v.guid}`}>{v.id ? `${v.id} — ${v.name}` : v.name}</option>
                  ))}
                </optgroup>
              )}
            </select>
          </label>

          <div className="dialog__row">
            <label className="field" style={{ flex: 1 }}>
              <span>Display ID</span>
              <input value={id} onChange={(e) => setId(e.target.value)} placeholder="JOB-0001" />
            </label>
            <label className="field" style={{ flex: 1 }}>
              <span>Reference</span>
              <input value={reference} onChange={(e) => setReference(e.target.value)} placeholder="PO-42" />
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
