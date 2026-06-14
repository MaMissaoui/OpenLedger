import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import type { Account, Numeric } from "../lib/types";
import { formatMoney } from "../lib/money";

interface Props {
  account: Account;
  onNewTransaction: () => void;
  onEditTransaction: (txGuid: string) => void;
}

function amountCell(n: Numeric) {
  const cls = n.num < 0 ? "num neg" : "num";
  return <td className={cls}>{n.num === 0 ? "—" : formatMoney(n)}</td>;
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "2-digit" });
}

export function RegisterView({ account, onNewTransaction, onEditTransaction }: Props) {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["register", account.guid],
    queryFn: () => api.getRegister(account.guid),
  });

  const del = useMutation({
    mutationFn: (txGuid: string) => api.deleteTransaction(txGuid),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["register"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["balance-sheet"] });
      qc.invalidateQueries({ queryKey: ["income-statement"] });
    },
  });

  function confirmDelete(txGuid: string, description: string) {
    if (window.confirm(`Delete "${description || "this transaction"}"? This cannot be undone.`)) {
      del.mutate(txGuid);
    }
  }

  const entries = data?.entries ?? [];
  const currentBalance = entries.length > 0 ? entries[entries.length - 1].balance : null;

  return (
    <section className="register">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">
            {account.type}
            {account.code ? ` · ${account.code}` : ""}
          </div>
          <h1>{account.name}</h1>
        </div>
        <div style={{ display: "flex", alignItems: "flex-end", gap: "1.4rem" }}>
          {currentBalance && (
            <div className="register__balance">
              <div className="eyebrow">Balance</div>
              <div className={`amt${currentBalance.num < 0 ? " neg" : ""}`}>
                {formatMoney(currentBalance)}
              </div>
            </div>
          )}
          <button className="btn btn--accent" onClick={onNewTransaction}>
            + New transaction
          </button>
        </div>
      </header>

      {isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : entries.length === 0 ? (
        <div className="empty">
          No entries yet. Post a transaction to start this account's history.
        </div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>Date</th>
              <th>Description</th>
              <th className="num">Amount</th>
              <th className="num">Balance</th>
              <th className="row-actions__head" aria-label="Actions" />
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.splitGuid}>
                <td className="date">{formatDate(e.postDate)}</td>
                <td>
                  <div className="desc">{e.description || "—"}</div>
                  {e.memo && <div className="memo">{e.memo}</div>}
                </td>
                {amountCell(e.quantity)}
                <td className={`num balance${e.balance.num < 0 ? " neg" : ""}`}>
                  {formatMoney(e.balance)}
                </td>
                <td className="row-actions">
                  <button
                    className="row-actions__btn"
                    onClick={() => onEditTransaction(e.txGuid)}
                    title="Edit transaction"
                  >
                    Edit
                  </button>
                  <button
                    className="row-actions__btn row-actions__btn--danger"
                    onClick={() => confirmDelete(e.txGuid, e.description)}
                    disabled={del.isPending}
                    title="Delete transaction"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
