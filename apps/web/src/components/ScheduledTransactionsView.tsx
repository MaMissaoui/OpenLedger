import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import type { Account, Book, NewScheduledTransaction, ScheduledTransaction } from "../lib/types";
import { parseAmount, formatMoney } from "../lib/money";

interface Props {
  book: Book;
  accounts: Account[];
}

// ScheduledTransactionsView lists all scheduled transactions for a book and
// lets the user create new ones and post due transactions with one click.
export function ScheduledTransactionsView({ book, accounts }: Props) {
  const qc = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [editSched, setEditSched] = useState<ScheduledTransaction | null>(null);
  const [postResult, setPostResult] = useState<string | null>(null);

  const q = useQuery({
    queryKey: ["scheduled-transactions", book.guid],
    queryFn: () => api.listScheduledTransactions(book.guid),
  });

  const postDueMutation = useMutation({
    mutationFn: () => api.postDueSchedules(book.guid),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["scheduled-transactions", book.guid] });
      qc.invalidateQueries({ queryKey: ["accounts", book.guid] });
      const n = data.posted.length;
      setPostResult(
        n === 0
          ? "No scheduled transactions due."
          : `Posted ${n} transaction${n === 1 ? "" : "s"}: ${data.posted.map((p) => p.name).join(", ")}.`,
      );
      setTimeout(() => setPostResult(null), 5000);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (guid: string) => api.deleteScheduledTransaction(guid),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["scheduled-transactions", book.guid] }),
  });

  const scheds = q.data ?? [];

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">Automation</div>
          <h1>Scheduled Transactions</h1>
        </div>
        <div className="register__actions">
          <button
            className="btn btn--ghost btn--sm"
            onClick={() => postDueMutation.mutate()}
            disabled={postDueMutation.isPending}
          >
            {postDueMutation.isPending ? "Posting…" : "Post Due"}
          </button>
          <button
            className="btn btn--primary btn--sm"
            onClick={() => {
              setEditSched(null);
              setShowForm(true);
            }}
          >
            + New
          </button>
        </div>
      </header>

      {postResult && (
        <div className="report-note" style={{ padding: "0.5rem 1rem", color: "var(--accent)" }}>
          {postResult}
        </div>
      )}

      {q.isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : scheds.length === 0 ? (
        <div className="empty">No scheduled transactions yet.</div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Period</th>
              <th>Next Due</th>
              <th>Last Posted</th>
              <th className="num">Amounts</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {scheds.map((s) => (
              <tr key={s.guid} className={s.enabled ? "" : "row--muted"}>
                <td>{s.name}</td>
                <td className="mono">
                  {s.every > 1 ? `every ${s.every} ` : ""}
                  {s.period}
                </td>
                <td className="mono">{s.nextDueDate ?? "—"}</td>
                <td className="mono">{s.lastPostedDate ?? "never"}</td>
                <td className="num mono">
                  {s.splits
                    .filter((sp) => sp.value.num > 0)
                    .map((sp) => formatMoney(sp.value))
                    .join(" / ")}
                </td>
                <td>
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => {
                      setEditSched(s);
                      setShowForm(true);
                    }}
                  >
                    Edit
                  </button>
                  <button
                    className="btn btn--ghost btn--xs"
                    onClick={() => deleteMutation.mutate(s.guid)}
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {showForm && (
        <ScheduledTransactionForm
          book={book}
          accounts={accounts}
          initial={editSched}
          onClose={() => {
            setShowForm(false);
            setEditSched(null);
            qc.invalidateQueries({ queryKey: ["scheduled-transactions", book.guid] });
          }}
        />
      )}
    </section>
  );
}

// ──────────────────────────────────────────────────────────────────────────────
// Form dialog

interface FormProps {
  book: Book;
  accounts: Account[];
  initial: ScheduledTransaction | null;
  onClose: () => void;
}

interface SplitRow {
  accountGuid: string;
  memo: string;
  amountStr: string;
}

function ScheduledTransactionForm({ book, accounts, initial, onClose }: FormProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [currencyGuid, setCurrencyGuid] = useState(
    initial?.currencyGuid ?? accounts[0]?.commodityGuid ?? "",
  );
  const [period, setPeriod] = useState<NewScheduledTransaction["period"]>(
    initial?.period ?? "monthly",
  );
  const [every, setEvery] = useState(String(initial?.every ?? 1));
  const [startDate, setStartDate] = useState(initial?.startDate ?? today());
  const [splits, setSplits] = useState<SplitRow[]>(
    initial?.splits.map((s) => ({
      accountGuid: s.accountGuid,
      memo: s.memo,
      amountStr: (s.value.num / s.value.denom).toFixed(2),
    })) ?? [
      { accountGuid: accounts[0]?.guid ?? "", memo: "", amountStr: "" },
      { accountGuid: accounts[1]?.guid ?? "", memo: "", amountStr: "" },
    ],
  );
  const [error, setError] = useState<string | null>(null);

  const qc = useQueryClient();

  const createMut = useMutation({
    mutationFn: (input: NewScheduledTransaction) =>
      api.createScheduledTransaction(book.guid, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["scheduled-transactions", book.guid] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  const updateMut = useMutation({
    mutationFn: (input: NewScheduledTransaction) =>
      api.updateScheduledTransaction(initial!.guid, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["scheduled-transactions", book.guid] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  function addSplit() {
    setSplits([...splits, { accountGuid: accounts[0]?.guid ?? "", memo: "", amountStr: "" }]);
  }

  function removeSplit(i: number) {
    setSplits(splits.filter((_, j) => j !== i));
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const parsedSplits = splits.map((sp, i) => {
      const v = parseAmount(sp.amountStr);
      if (!v) {
        setError(`Invalid amount on split ${i + 1}`);
        return null;
      }
      return { accountGuid: sp.accountGuid, memo: sp.memo, value: v };
    });
    if (parsedSplits.some((s) => s === null)) return;

    const input: NewScheduledTransaction = {
      name,
      enabled,
      currencyGuid,
      period,
      every: parseInt(every) || 1,
      startDate,
      splits: parsedSplits as NonNullable<(typeof parsedSplits)[number]>[],
    };

    if (initial) {
      updateMut.mutate(input);
    } else {
      createMut.mutate(input);
    }
  }

  const postableAccounts = accounts.filter((a) => !a.placeholder && a.type !== "ROOT");

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <header className="dialog__header">
          <h2>{initial ? "Edit Scheduled Transaction" : "New Scheduled Transaction"}</h2>
          <button className="dialog__close" onClick={onClose}>
            ✕
          </button>
        </header>
        <form onSubmit={submit} className="dialog__body">
          <div className="field">
            <label>Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </div>
          <div className="field row">
            <label>
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />{" "}
              Enabled
            </label>
          </div>
          <div className="field">
            <label>Currency (commodity GUID)</label>
            <input
              value={currencyGuid}
              onChange={(e) => setCurrencyGuid(e.target.value)}
              placeholder="e.g. the GUID of USD"
              required
            />
          </div>
          <div className="field row">
            <div>
              <label>Repeat every</label>
              <input
                type="number"
                min={1}
                value={every}
                onChange={(e) => setEvery(e.target.value)}
                style={{ width: "4rem" }}
              />
            </div>
            <div>
              <label>Period</label>
              <select value={period} onChange={(e) => setPeriod(e.target.value as typeof period)}>
                <option value="once">once</option>
                <option value="daily">day(s)</option>
                <option value="weekly">week(s)</option>
                <option value="monthly">month(s)</option>
                <option value="yearly">year(s)</option>
              </select>
            </div>
          </div>
          <div className="field">
            <label>Start date</label>
            <input
              type="date"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
              required
            />
          </div>

          <fieldset>
            <legend>Template splits (must balance)</legend>
            {splits.map((sp, i) => (
              <div key={i} className="split-row">
                <select
                  value={sp.accountGuid}
                  onChange={(e) => {
                    const next = [...splits];
                    next[i] = { ...next[i], accountGuid: e.target.value };
                    setSplits(next);
                  }}
                >
                  {postableAccounts.map((a) => (
                    <option key={a.guid} value={a.guid}>
                      {a.name}
                    </option>
                  ))}
                </select>
                <input
                  placeholder="0.00 (negative to debit)"
                  value={sp.amountStr}
                  onChange={(e) => {
                    const next = [...splits];
                    next[i] = { ...next[i], amountStr: e.target.value };
                    setSplits(next);
                  }}
                  className="mono"
                  style={{ width: "8rem" }}
                />
                {splits.length > 2 && (
                  <button type="button" className="btn btn--ghost btn--xs" onClick={() => removeSplit(i)}>
                    ✕
                  </button>
                )}
              </div>
            ))}
            <button type="button" className="btn btn--ghost btn--xs" onClick={addSplit}>
              + Add split
            </button>
          </fieldset>

          {error && <p className="error">{error}</p>}

          <footer className="dialog__footer">
            <button type="button" className="btn btn--ghost" onClick={onClose}>
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn--primary"
              disabled={createMut.isPending || updateMut.isPending}
            >
              {initial ? "Save" : "Create"}
            </button>
          </footer>
        </form>
      </div>
    </div>
  );
}

function today(): string {
  return new Date().toISOString().slice(0, 10);
}
