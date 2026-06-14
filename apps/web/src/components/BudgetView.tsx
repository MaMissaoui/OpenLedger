import { useState } from "react";
import { api } from "../lib/api";
import { formatMoney, parseAmount } from "../lib/money";
import type { Budget, BudgetAmount, BudgetReport, NewBudget, Numeric } from "../lib/types";

// ── helpers ──────────────────────────────────────────────────────────────────

function zero(): Numeric {
  return { num: 0, denom: 100 };
}

function varianceClass(v: Numeric): string {
  if (v.num < 0) return "negative";
  if (v.num > 0) return "positive";
  return "";
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
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <h2>{existing ? "Edit Budget" : "New Budget"}</h2>
        {error && <p className="error">{error}</p>}
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label>
          Description
          <input value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
        <label>
          Period type
          <select
            value={periodType}
            onChange={(e) => setPeriodType(e.target.value as typeof periodType)}
          >
            <option value="monthly">Monthly</option>
            <option value="quarterly">Quarterly</option>
            <option value="yearly">Yearly</option>
          </select>
        </label>
        <label>
          Number of periods
          <input
            type="number"
            min={1}
            value={numPeriods}
            onChange={(e) => setNumPeriods(e.target.value)}
          />
        </label>
        <label>
          Start date
          <input
            type="date"
            value={startDate}
            onChange={(e) => setStartDate(e.target.value)}
          />
        </label>
        <div className="dialog-actions">
          <button onClick={onClose}>Cancel</button>
          <button onClick={handleSave} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Budget amounts editor (per-account per-period amounts) ────────────────────

interface AmountEditorProps {
  budget: Budget;
  onClose: () => void;
  onSaved: () => void;
}

function AmountEditor({ budget, onClose, onSaved }: AmountEditorProps) {
  // Build a period × account grid from existing amounts.
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
      <div className="dialog" style={{ minWidth: 500 }} onClick={(e) => e.stopPropagation()}>
        <h2>Budget amounts — {budget.name}</h2>
        {error && <p className="error">{error}</p>}
        <table className="report-table">
          <thead>
            <tr>
              <th>Account GUID</th>
              <th>Period #</th>
              <th>Amount</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={i}>
                <td>{r.accountGuid}</td>
                <td>{r.periodNum}</td>
                <td className="amount">{formatMoney(r.value)}</td>
                <td>
                  <button onClick={() => removeRow(i)}>×</button>
                </td>
              </tr>
            ))}
            <tr>
              <td>
                <input
                  placeholder="account GUID"
                  value={newAcct}
                  onChange={(e) => setNewAcct(e.target.value)}
                />
              </td>
              <td>
                <input
                  type="number"
                  min={0}
                  value={newPeriod}
                  onChange={(e) => setNewPeriod(e.target.value)}
                  style={{ width: 60 }}
                />
              </td>
              <td>
                <input
                  placeholder="0.00"
                  value={newVal}
                  onChange={(e) => setNewVal(e.target.value)}
                  style={{ width: 80 }}
                />
              </td>
              <td>
                <button onClick={addRow}>+</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div className="dialog-actions">
          <button onClick={onClose}>Cancel</button>
          <button onClick={handleSave} disabled={saving}>
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
        style={{ minWidth: 600, maxHeight: "80vh", overflowY: "auto" }}
        onClick={(e) => e.stopPropagation()}
      >
        <h2>Budget variance report</h2>
        <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 12 }}>
          <label>
            As of
            <input type="date" value={asOf} onChange={(e) => setAsOf(e.target.value)} />
          </label>
          <button onClick={loadReport} disabled={loading}>
            {loading ? "Loading…" : "Run report"}
          </button>
        </div>
        {error && <p className="error">{error}</p>}
        {report && (
          <>
            <h3>
              {report.periodLabel} ({report.periodStart} – {report.periodEnd})
            </h3>
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
                    <td colSpan={4} style={{ textAlign: "center", color: "#888" }}>
                      No budget amounts for this period.
                    </td>
                  </tr>
                )}
                {report.lines.map((l, i) => (
                  <tr key={i}>
                    <td>{l.account.name}</td>
                    <td className="amount">{formatMoney(l.budgeted)}</td>
                    <td className="amount">{formatMoney(l.actual)}</td>
                    <td className={`amount ${varianceClass(l.variance)}`}>
                      {formatMoney(l.variance)}
                    </td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr>
                  <td>
                    <strong>Total</strong>
                  </td>
                  <td className="amount">
                    <strong>{formatMoney(report.totalBudgeted ?? zero())}</strong>
                  </td>
                  <td className="amount">
                    <strong>{formatMoney(report.totalActual ?? zero())}</strong>
                  </td>
                  <td className={`amount ${varianceClass(report.totalVariance ?? zero())}`}>
                    <strong>{formatMoney(report.totalVariance ?? zero())}</strong>
                  </td>
                </tr>
              </tfoot>
            </table>
          </>
        )}
        <div className="dialog-actions">
          <button onClick={onClose}>Close</button>
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
    if (!confirm("Delete this budget?")) return;
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
        <button onClick={() => setShowForm(true)}>+ New budget</button>
      </div>

      {loading && <p>Loading…</p>}
      {error && <p className="error">{error}</p>}

      {!loading && budgets.length === 0 && (
        <p style={{ color: "#888" }}>No budgets yet. Create one to get started.</p>
      )}

      {budgets.length > 0 && (
        <table className="report-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Period</th>
              <th>Periods</th>
              <th>Start</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {budgets.map((b) => (
              <tr key={b.guid}>
                <td>{b.name}</td>
                <td style={{ textTransform: "capitalize" }}>{b.periodType}</td>
                <td>{b.numPeriods}</td>
                <td>{b.startDate}</td>
                <td style={{ display: "flex", gap: 6 }}>
                  <button onClick={() => setReportTarget(b.guid)}>Report</button>
                  <button onClick={() => setAmountsTarget(b)}>Amounts</button>
                  <button onClick={() => setEditTarget(b)}>Edit</button>
                  <button onClick={() => void handleDelete(b.guid)}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {showForm && (
        <BudgetForm
          bookGuid={bookGuid}
          onClose={() => setShowForm(false)}
          onSaved={() => {
            setShowForm(false);
            void load();
          }}
        />
      )}
      {editTarget && (
        <BudgetForm
          bookGuid={bookGuid}
          existing={editTarget}
          onClose={() => setEditTarget(null)}
          onSaved={() => {
            setEditTarget(null);
            void load();
          }}
        />
      )}
      {amountsTarget && (
        <AmountEditor
          budget={amountsTarget}
          onClose={() => setAmountsTarget(null)}
          onSaved={() => {
            setAmountsTarget(null);
            void load();
          }}
        />
      )}
      {reportTarget && (
        <ReportPanel budgetGuid={reportTarget} onClose={() => setReportTarget(null)} />
      )}
    </div>
  );
}
