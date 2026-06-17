import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import type { Book, CashFlowSection } from "../lib/types";
import { formatMoney } from "../lib/money";

interface Props {
  book: Book;
  onBack?: () => void;
}

function today(): string {
  return new Date().toISOString().slice(0, 10);
}
function startOfYear(): string {
  return `${new Date().getFullYear()}-01-01`;
}
function startISO(d: string): string | undefined {
  return d ? `${d}T00:00:00Z` : undefined;
}
function endISO(d: string): string | undefined {
  return d ? `${d}T23:59:59Z` : undefined;
}

// One activity category: a bold header row carrying the section total, then an
// indented line per contributing account.
function Section({ title, section }: { title: string; section: CashFlowSection }) {
  return (
    <>
      <tr className="cf-cat">
        <td>{title}</td>
        <td className={`amount${section.total.num < 0 ? " neg" : ""}`}>
          {formatMoney(section.total)}
        </td>
      </tr>
      {section.lines.length === 0 ? (
        <tr className="cf-line">
          <td className="cf-empty">No activity in this period.</td>
          <td />
        </tr>
      ) : (
        section.lines.map((l) => (
          <tr className="cf-line" key={l.account.guid}>
            <td className="cf-line__name">{l.account.name}</td>
            <td className={`amount${l.amount.num < 0 ? " neg" : ""}`}>{formatMoney(l.amount)}</td>
          </tr>
        ))
      )}
    </>
  );
}

export function CashFlowView({ book, onBack }: Props) {
  const [from, setFrom] = useState(startOfYear());
  const [to, setTo] = useState(today());

  const q = useQuery({
    queryKey: ["cash-flow", book.guid, from, to],
    queryFn: () => api.getCashFlow(book.guid, startISO(from), endISO(to)),
  });

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          {onBack ? (
            <button className="back-link" onClick={onBack}>
              ‹ Reports Center
            </button>
          ) : (
            <div className="eyebrow">Reports</div>
          )}
          <h1>Cash flow statement</h1>
        </div>
      </header>

      <div className="report__controls">
        <label className="field">
          <span>From</span>
          <input type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} />
        </label>
        <label className="field">
          <span>To</span>
          <input type="date" value={to} max={today()} onChange={(e) => setTo(e.target.value)} />
        </label>
      </div>

      {q.isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : q.isError ? (
        <div className="empty">Could not load the cash flow statement.</div>
      ) : (
        q.data && (
          <>
            {/* Summary cards */}
            <div className="cf-summary">
              <div className="cf-card cf-card--primary">
                <span className="cf-card__label">Cash at end of period</span>
                <span className="cf-card__value mono">{formatMoney(q.data.endingCash)}</span>
              </div>
              <div className="cf-card">
                <span className="cf-card__label">Net change in cash</span>
                <span className={`cf-card__value mono${q.data.netChange.num < 0 ? " neg" : " pos"}`}>
                  {q.data.netChange.num >= 0 ? "+" : ""}
                  {formatMoney(q.data.netChange)}
                </span>
              </div>
            </div>

            {/* Detailed statement */}
            <table className="report-table cf-table">
              <thead>
                <tr>
                  <th>Activity</th>
                  <th className="amount">Amount</th>
                </tr>
              </thead>
              <tbody>
                <Section title="Operating activities" section={q.data.operating} />
                <Section title="Investing activities" section={q.data.investing} />
                <Section title="Financing activities" section={q.data.financing} />
              </tbody>
              <tfoot>
                <tr className="cf-net">
                  <td>Net change in cash</td>
                  <td className={`amount${q.data.netChange.num < 0 ? " neg" : ""}`}>
                    {formatMoney(q.data.netChange)}
                  </td>
                </tr>
                <tr className="cf-begin">
                  <td>Cash at beginning of period</td>
                  <td className="amount">{formatMoney(q.data.beginningCash)}</td>
                </tr>
                <tr className="cf-end">
                  <td>Cash at end of period</td>
                  <td className="amount">{formatMoney(q.data.endingCash)}</td>
                </tr>
              </tfoot>
            </table>

            <p className="report__note">
              Direct method by counterparty classification. Single-currency only — balances are not
              converted across commodities.
            </p>
          </>
        )
      )}
    </section>
  );
}
