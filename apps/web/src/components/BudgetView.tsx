import { useState } from "react";
import { api } from "../lib/api";
import { formatMoney, parseAmount } from "../lib/money";
import type { Budget, BudgetAmount, BudgetReport, NewBudget, Numeric } from "../lib/types";

function zero(): Numeric {
  return { num: 0, denom: 100 };
}

// ── Budget form dialog ────────────────────────────────────────────────────────

interface BudgetFormProps {
  bookGuid: string;
  existing?: Budget;
  onClose: () => void;
  onSaved: () => void;
}

function BudgetForm({ bookGuid, existing, onClose, onSaved }: BudgetFormProps) {
  const [name, setName] = useState(existing?.name ?? "");
  const [description, setDescription] = useState(existing?.description ?? "");
  const [periodType, setPeriodType] = useState<"monthly" | "quarterly" | "yearly">(
    existing?.periodType ?? "monthly",
  );
  const [numPeriods, setNumPeriods] = useState(String(existing?.numPeriods ?? 12));
  const [startDate, setStartDate] = useState(existing?.startDate ?? "");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function handleSave() {
    setError(null);
    const n = parseInt(numPeriods, 10);
    if (!name || !startDate || isNaN(n) || n <= 0) {
      setError("Name, start date, and a positive number of periods are required.");
      return;
    }
    const input: NewBudget = {
      name,
      description,
      periodType,
      numPeriods: n,
      startDate,
      amounts: existing?.amounts ?? [],
    };
    setSaving(true);
    try {
      if (existing) {
        await api.updateBudget(existing.guid, input);
      } else {
        await api.createBudget(bookGuid, input);
      }
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" style={{ width: "min(480px, 100%)" }} onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>{existing ? "Edit Budget" : "New Budget"}</h2>
          <button className="dialog__close" onClick={onClose}>×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error">{error}</p>}
          <label className="field">
            <span>Name</span>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. 2025 Annual Budget" />
          </label>
          <label className="field">
            <span>Description</span>
            <input value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
          <div className="dialog__row">
            <label className="field">
              <span>Period type</span>
              <select value={periodType} onChange={(e) => setPeriodType(e.target.value as typeof periodType)}>
                <option value="monthly">Monthly</option>
                <option value="quarterly">Quarterly</option>
                <option value="yearly">Yearly</option>
              </select>
            </label>
            <label className="field">
              <span>Number of periods</span>
              <input type="number" min={1} value={numPeriods} onChange={(e) => setNumPeriods(e.target.value)} />
            </label>
          </div>
          <label className="field">
            <span>Start date</span>
            <input type="date" value={startDate} onChange={(e) => setStartDate(e.target.value)} />
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

// ── Budget amounts editor ─────────────────────────────────────────────────────

interface AmountEditorProps {
  budget: Budget;
  onClose: () => void;
  onSaved: () => void;
}

function AmountEditor({ budget, onClose, onSaved }: AmountEditorProps) {
  const [rows, setRows] = useState<BudgetAmount[]>(budget.amounts ?? []);
  const [newAcct, setNewAcct] = useState("");
  const [newPeriod, setNewPeriod] = useState("0");
  const [newVal, setNewVal] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  function addRow() {
    const p = parseInt(newPeriod, 10);
    const v = parseAmount(newVal);
    if (!newAcct || isNaN(p) || !v) {
      setError("Account GUID, period number, and amount are required.");
      return;
    }
    setError(null);
    setRows((prev) => [
      ...prev.filter((r) => !(r.accountGuid === newAcct && r.periodNum === p)),
      { accountGuid: newAcct, periodNum: p, value: v },
    ]);
    setNewAcct("");
    setNewPeriod("0");
    setNewVal("");
  }

  function removeRow(idx: number) {
    setRows((prev) => prev.filter((_, i) => i !== idx));
  }

  async function handleSave() {
    setError(null);
    setSaving(true);
    try {
      await api.updateBudget(budget.guid, {
        name: budget.name,
        description: budget.description,
        periodType: budget.periodType,
        numPeriods: budget.numPeriods,
        startDate: budget.startDate,
        amounts: rows,
      });
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div
        className="dialog"
        style={{ width: "min(560px, 100%)" }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="dialog__header">
          <h2>Budget amounts — {budget.name}</h2>
          <button className="dialog__close" onClick={onClose}>×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error">{error}</p>}
          <p className="sub" style={{ margin: 0, fontSize: "0.85rem", color: "var(--ink-soft)" }}>
            Enter planned amounts per account per period number (0-based).
          </p>
          <table className="report-table">
            <thead>
              <tr>
                <th>Account GUID</th>
                <th>Period #</th>
                <th className="amount">Amount</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  <td className="mono" style={{ fontSize: "0.8rem" }}>{r.accountGuid}</td>
                  <td>{r.periodNum}</td>
                  <td className="amount">{formatMoney(r.value)}</td>
                  <td>
                    <button className="btn btn--ghost btn--xs" onClick={() => removeRow(i)}>Remove</button>
                  </td>
                </tr>
              ))}
              <tr>
                <td>
                  <input
                    style={{ width: "100%", fontFamily: "var(--mono)", fontSize: "0.8rem", padding: "0.3rem 0.4rem", border: "1px solid var(--rule-strong)", borderRadius: 2 }}
                    placeholder="account GUID"
                    value={newAcct}
                    onChange={(e) => setNewAcct(e.target.value)}
                  />
                </td>
                <td>
                  <input
                    type="number"
                    min={0}
                    style={{ width: 64, padding: "0.3rem 0.4rem", border: "1px solid var(--rule-strong)", borderRadius: 2 }}
                    value={newPeriod}
                    onChange={(e) => setNewPeriod(e.target.value)}
                  />
                </td>
                <td>
                  <input
                    placeholder="0.00"
                    style={{ width: 90, textAlign: "right", fontFamily: "var(--mono)", padding: "0.3rem 0.4rem", border: "1px solid var(--rule-strong)", borderRadius: 2 }}
                    value={newVal}
                    onChange={(e) => setNewVal(e.target.value)}
                  />
                </td>
                <td>
                  <button className="btn btn--ghost btn--xs" onClick={addRow}>+ Add</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? "Saving…" : "Save amounts"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Variance report panel ─────────────────────────────────────────────────────

interface ReportPanelProps {
  budgetGuid: string;
  onClose: () => void;
}

function ReportPanel({ budgetGuid, onClose }: ReportPanelProps) {
  const [asOf, setAsOf] = useState(new Date().toISOString().slice(0, 10));
  const [report, setReport] = useState<BudgetReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function loadReport() {
    setError(null);
    setLoading(true);
    try {
      const r = await api.budgetReport(budgetGuid, asOf ? asOf + "T00:00:00Z" : undefined);
      setReport(r);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load report");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div
        className="dialog"
        style={{ width: "min(640px, 100%)", maxHeight: "80vh", display: "flex", flexDirection: "column" }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="dialog__header">
          <h2>Budget variance report</h2>
          <button className="dialog__close" onClick={onClose}>×</button>
        </div>

        <div style={{ display: "flex", gap: 8, alignItems: "flex-end", marginBottom: 12, flexShrink: 0 }}>
          <label className="field" style={{ margin: 0 }}>
            <span>As of date</span>
            <input type="date" value={asOf} onChange={(e) => setAsOf(e.target.value)} />
          </label>
          <button className="btn btn--primary btn--sm" onClick={loadReport} disabled={loading}>
            {loading ? <span className="spinner" /> : "Run report"}
          </button>
        </div>

        {error && <p className="error">{error}</p>}

        <div style={{ overflowY: "auto", flex: 1 }}>
          {report && (
            <>
              <p className="eyebrow" style={{ margin: "0 0 0.6rem" }}>
                {report.periodLabel} &ensp;{report.periodStart} – {report.periodEnd}
              </p>
              <table className="report-table">
                <thead>
                  <tr>
                    <th>Account</th>
                    <th className="amount">Budgeted</th>
                    <th className="amount">Actual</th>
                    <th className="amount">Variance</th>
                  </tr>
                </thead>
                <tbody>
                  {report.lines.length === 0 && (
                    <tr>
                      <td colSpan={4} className="empty">No budget amounts for this period.</td>
                    </tr>
                  )}
                  {report.lines.map((l, i) => (
                    <tr key={i}>
                      <td>{l.account.name}</td>
                      <td className="amount mono">{formatMoney(l.budgeted)}</td>
                      <td className="amount mono">{formatMoney(l.actual)}</td>
                      <td className={`amount mono ${l.variance.num < 0 ? "neg" : ""}`}>
                        {formatMoney(l.variance)}
                      </td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr>
                    <td><strong>Total</strong></td>
                    <td className="amount mono"><strong>{formatMoney(report.totalBudgeted ?? zero())}</strong></td>
                    <td className="amount mono"><strong>{formatMoney(report.totalActual ?? zero())}</strong></td>
                    <td className={`amount mono ${(report.totalVariance?.num ?? 0) < 0 ? "neg" : ""}`}>
                      <strong>{formatMoney(report.totalVariance ?? zero())}</strong>
                    </td>
                  </tr>
                </tfoot>
              </table>
            </>
          )}
        </div>

        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
}

// ── Main BudgetView ───────────────────────────────────────────────────────────

interface BudgetViewProps {
  bookGuid: string;
}

export default function BudgetView({ bookGuid }: BudgetViewProps) {
  const [budgets, setBudgets] = useState<Budget[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState<Budget | null>(null);
  const [amountsTarget, setAmountsTarget] = useState<Budget | null>(null);
  const [reportTarget, setReportTarget] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError(null);
    try {
      const res = await api.listBudgets(bookGuid);
      setBudgets(res.budgets ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load budgets");
    } finally {
      setLoading(false);
    }
  }

  useState(() => {
    void load();
  });

  async function handleDelete(guid: string) {
    if (!confirm("Delete this budget and all its amounts?")) return;
    try {
      await api.deleteBudget(guid);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Delete failed");
    }
  }

  return (
    <div className="budget-view">
      <div className="view-header">
        <h2>Budgets</h2>
        <button className="btn btn--primary btn--sm" onClick={() => setShowForm(true)}>
          + New budget
        </button>
      </div>

      <div style={{ padding: "1.2rem 2rem", overflowY: "auto", flex: 1 }}>
        {error && <p className="error">{error}</p>}

        {loading && (
          <div className="empty"><span className="spinner" /></div>
        )}

        {!loading && budgets.length === 0 && (
          <div className="empty">No budgets yet. Create one to get started.</div>
        )}

        {budgets.length > 0 && (
          <table className="report-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Description</th>
                <th>Period</th>
                <th>Periods</th>
                <th>Start date</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {budgets.map((b) => (
                <tr key={b.guid}>
                  <td style={{ fontWeight: 600 }}>{b.name}</td>
                  <td style={{ color: "var(--ink-soft)", fontSize: "0.85rem" }}>{b.description || "—"}</td>
                  <td style={{ textTransform: "capitalize" }}>{b.periodType}</td>
                  <td>{b.numPeriods}</td>
                  <td className="mono" style={{ fontSize: "0.85rem" }}>{b.startDate}</td>
                  <td style={{ whiteSpace: "nowrap" }}>
                    <button className="btn btn--ghost btn--xs" onClick={() => setReportTarget(b.guid)}>Report</button>
                    {" "}
                    <button className="btn btn--ghost btn--xs" onClick={() => setAmountsTarget(b)}>Amounts</button>
                    {" "}
                    <button className="btn btn--ghost btn--xs" onClick={() => setEditTarget(b)}>Edit</button>
                    {" "}
                    <button className="btn btn--ghost btn--xs" onClick={() => void handleDelete(b.guid)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showForm && (
        <BudgetForm
          bookGuid={bookGuid}
          onClose={() => setShowForm(false)}
          onSaved={() => { setShowForm(false); void load(); }}
        />
      )}
      {editTarget && (
        <BudgetForm
          bookGuid={bookGuid}
          existing={editTarget}
          onClose={() => setEditTarget(null)}
          onSaved={() => { setEditTarget(null); void load(); }}
        />
      )}
      {amountsTarget && (
        <AmountEditor
          budget={amountsTarget}
          onClose={() => setAmountsTarget(null)}
          onSaved={() => { setAmountsTarget(null); void load(); }}
        />
      )}
      {reportTarget && (
        <ReportPanel budgetGuid={reportTarget} onClose={() => setReportTarget(null)} />
      )}
    </div>
  );
}
