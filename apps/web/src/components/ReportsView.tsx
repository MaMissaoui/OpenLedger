import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import type { Book, Numeric, ReportSection } from "../lib/types";
import { formatMoney } from "../lib/money";

interface Props {
  book: Book;
}

type Tab = "balance-sheet" | "income-statement";

function today(): string {
  return new Date().toISOString().slice(0, 10);
}
function startOfYear(): string {
  return `${new Date().getFullYear()}-01-01`;
}
// HTML date inputs yield "YYYY-MM-DD"; the API wants RFC 3339. We anchor the
// lower bound to the start of its day and the upper bound to the end, so the
// chosen day is fully included. UTC keeps it simple and deterministic.
function startISO(d: string): string | undefined {
  return d ? `${d}T00:00:00Z` : undefined;
}
function endISO(d: string): string | undefined {
  return d ? `${d}T23:59:59Z` : undefined;
}

// equalAmount compares two exact amounts without assuming a shared denominator.
function equalAmount(a: Numeric, b: Numeric): boolean {
  return a.num * b.denom === b.num * a.denom;
}

function SectionTable({ label, section }: { label: string; section: ReportSection }) {
  return (
    <table className="ledger-table report-section">
      <thead>
        <tr>
          <th>{label}</th>
          <th className="num">Balance</th>
        </tr>
      </thead>
      <tbody>
        {section.lines.length === 0 ? (
          <tr>
            <td colSpan={2} className="empty">
              No accounts with a balance.
            </td>
          </tr>
        ) : (
          section.lines.map((l) => (
            <tr key={l.account.guid}>
              <td className="desc">{l.account.name}</td>
              <td className={`num${l.balance.num < 0 ? " neg" : ""}`}>{formatMoney(l.balance)}</td>
            </tr>
          ))
        )}
      </tbody>
      <tfoot>
        <tr className="report-total">
          <td>Total {label.toLowerCase()}</td>
          <td className={`num${section.total.num < 0 ? " neg" : ""}`}>{formatMoney(section.total)}</td>
        </tr>
      </tfoot>
    </table>
  );
}

function TotalRow({ label, amount, strong }: { label: string; amount: Numeric; strong?: boolean }) {
  return (
    <div className={`report-line${strong ? " report-line--strong" : ""}`}>
      <span>{label}</span>
      <span className={`mono${amount.num < 0 ? " neg" : ""}`}>{formatMoney(amount)}</span>
    </div>
  );
}

function BalanceSheetPanel({ book }: Props) {
  const [asOf, setAsOf] = useState(today());
  const q = useQuery({
    queryKey: ["balance-sheet", book.guid, asOf],
    queryFn: () => api.getBalanceSheet(book.guid, endISO(asOf)),
  });

  return (
    <>
      <div className="report__controls">
        <label className="field">
          <span>As of</span>
          <input type="date" value={asOf} max={today()} onChange={(e) => setAsOf(e.target.value)} />
        </label>
      </div>

      {q.isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : q.isError ? (
        <div className="empty">Could not load the balance sheet.</div>
      ) : (
        q.data && (
          <>
            <div className="report-grid">
              <SectionTable label="Assets" section={q.data.assets} />
              <div className="report-col">
                <SectionTable label="Liabilities" section={q.data.liabilities} />
                <SectionTable label="Equity" section={q.data.equity} />
              </div>
            </div>
            <div className="report-summary">
              <TotalRow label="Retained earnings (net income)" amount={q.data.retainedEarnings} />
              <TotalRow label="Total assets" amount={q.data.assets.total} strong />
              <TotalRow
                label="Total liabilities + equity"
                amount={q.data.totalLiabilitiesAndEquity}
                strong
              />
              <span
                className={`balance-pill ${
                  equalAmount(q.data.assets.total, q.data.totalLiabilitiesAndEquity)
                    ? "balance-pill--ok"
                    : "balance-pill--off"
                }`}
              >
                {equalAmount(q.data.assets.total, q.data.totalLiabilitiesAndEquity)
                  ? "In balance"
                  : "Out of balance"}
              </span>
            </div>
          </>
        )
      )}
    </>
  );
}

function IncomeStatementPanel({ book }: Props) {
  const [from, setFrom] = useState(startOfYear());
  const [to, setTo] = useState(today());
  const q = useQuery({
    queryKey: ["income-statement", book.guid, from, to],
    queryFn: () => api.getIncomeStatement(book.guid, startISO(from), endISO(to)),
  });

  return (
    <>
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
        <div className="empty">Could not load the income statement.</div>
      ) : (
        q.data && (
          <>
            <div className="report-grid">
              <SectionTable label="Income" section={q.data.income} />
              <SectionTable label="Expenses" section={q.data.expense} />
            </div>
            <div className="report-summary">
              <TotalRow label="Total income" amount={q.data.income.total} />
              <TotalRow label="Total expenses" amount={q.data.expense.total} />
              <TotalRow label="Net income" amount={q.data.netIncome} strong />
            </div>
          </>
        )
      )}
    </>
  );
}

export function ReportsView({ book }: Props) {
  const [tab, setTab] = useState<Tab>("balance-sheet");

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">Reports</div>
          <h1>{tab === "balance-sheet" ? "Balance sheet" : "Income statement"}</h1>
        </div>
        <div className="report__tabs">
          <button
            className={`btn btn--sm ${tab === "balance-sheet" ? "btn--accent" : "btn--ghost"}`}
            onClick={() => setTab("balance-sheet")}
          >
            Balance sheet
          </button>
          <button
            className={`btn btn--sm ${tab === "income-statement" ? "btn--accent" : "btn--ghost"}`}
            onClick={() => setTab("income-statement")}
          >
            Income statement
          </button>
        </div>
      </header>

      {tab === "balance-sheet" ? (
        <BalanceSheetPanel book={book} />
      ) : (
        <IncomeStatementPanel book={book} />
      )}

      <p className="report__note">
        Single-currency only — balances are not yet converted across commodities.
      </p>
    </section>
  );
}
