import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import type { Account, Numeric } from "../lib/types";
import { formatMoney } from "../lib/money";

interface Props {
  account: Account;
  onNewTransaction: () => void;
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

export function RegisterView({ account, onNewTransaction }: Props) {
  const { data, isLoading } = useQuery({
    queryKey: ["register", account.guid],
    queryFn: () => api.getRegister(account.guid),
  });

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
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
