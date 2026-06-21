import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "../lib/api";
import type { Book, Numeric, ReportSection } from "../lib/types";
import { formatMoney } from "../lib/money";

interface Props {
  book: Book;
}

type Tab = "balance-sheet" | "income-statement";

interface ReportsViewProps {
  book: Book;
  initialTab?: Tab;
  onBack?: () => void;
}

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
  const { t } = useTranslation();
  return (
    <table className="ledger-table report-section">
      <thead>
        <tr>
          <th>{label}</th>
          <th className="num">{t("common.balance")}</th>
        </tr>
      </thead>
      <tbody>
        {section.lines.length === 0 ? (
          <tr>
            <td colSpan={2} className="empty">
              {t("reports.balanceSheet.noBalance")}
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
          <td>{t("reports.balanceSheet.totalLabel", { section: label.toLowerCase() })}</td>
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
  const { t } = useTranslation();
  const [asOf, setAsOf] = useState(today());
  const q = useQuery({
    queryKey: ["balance-sheet", book.guid, asOf],
    queryFn: () => api.getBalanceSheet(book.guid, endISO(asOf)),
  });

  return (
    <>
      <div className="report__controls">
        <label className="field">
          <span>{t("common.asOf")}</span>
          <input type="date" value={asOf} max={today()} onChange={(e) => setAsOf(e.target.value)} />
        </label>
      </div>

      {q.isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : q.isError ? (
        <div className="empty">{t("reports.balanceSheet.loadError")}</div>
      ) : (
        q.data && (
          <>
            <div className="report-grid">
              <SectionTable label={t("reports.balanceSheet.assets")} section={q.data.assets} />
              <div className="report-col">
                <SectionTable label={t("reports.balanceSheet.liabilities")} section={q.data.liabilities} />
                <SectionTable label={t("reports.balanceSheet.equity")} section={q.data.equity} />
              </div>
            </div>
            <div className="report-summary">
              <TotalRow label={t("reports.balanceSheet.retainedEarnings")} amount={q.data.retainedEarnings} />
              <TotalRow label={t("reports.balanceSheet.totalAssets")} amount={q.data.assets.total} strong />
              <TotalRow
                label={t("reports.balanceSheet.totalLiabilitiesEquity")}
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
                  ? t("reports.balanceSheet.inBalance")
                  : t("reports.balanceSheet.outOfBalance")}
              </span>
            </div>
          </>
        )
      )}
    </>
  );
}

function IncomeStatementPanel({ book }: Props) {
  const { t } = useTranslation();
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
          <span>{t("common.from")}</span>
          <input type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} />
        </label>
        <label className="field">
          <span>{t("common.to")}</span>
          <input type="date" value={to} max={today()} onChange={(e) => setTo(e.target.value)} />
        </label>
      </div>

      {q.isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : q.isError ? (
        <div className="empty">{t("reports.incomeStatement.loadError")}</div>
      ) : (
        q.data && (
          <>
            <div className="report-grid">
              <SectionTable label={t("reports.incomeStatement.income")} section={q.data.income} />
              <SectionTable label={t("reports.incomeStatement.expenses")} section={q.data.expense} />
            </div>
            <div className="report-summary">
              <TotalRow label={t("reports.incomeStatement.totalIncome")} amount={q.data.income.total} />
              <TotalRow label={t("reports.incomeStatement.totalExpenses")} amount={q.data.expense.total} />
              <TotalRow label={t("reports.incomeStatement.netIncome")} amount={q.data.netIncome} strong />
            </div>
          </>
        )
      )}
    </>
  );
}

export function ReportsView({ book, initialTab, onBack }: ReportsViewProps) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>(initialTab ?? "balance-sheet");

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          {onBack ? (
            <button className="back-link" onClick={onBack}>
              {t("reports.reportsCenter")}
            </button>
          ) : (
            <div className="eyebrow">{t("nav.reports")}</div>
          )}
          <h1>{tab === "balance-sheet" ? t("reports.balanceSheet.title") : t("reports.incomeStatement.title")}</h1>
        </div>
        <div className="report__tabs">
          <button
            className={`btn btn--sm ${tab === "balance-sheet" ? "btn--accent" : "btn--ghost"}`}
            onClick={() => setTab("balance-sheet")}
          >
            {t("reports.balanceSheet.tabLabel")}
          </button>
          <button
            className={`btn btn--sm ${tab === "income-statement" ? "btn--accent" : "btn--ghost"}`}
            onClick={() => setTab("income-statement")}
          >
            {t("reports.incomeStatement.tabLabel")}
          </button>
        </div>
      </header>

      {tab === "balance-sheet" ? (
        <BalanceSheetPanel book={book} />
      ) : (
        <IncomeStatementPanel book={book} />
      )}

      <p className="report__note">
        {t("reports.balanceSheet.singleCurrencyNote")}
      </p>
    </section>
  );
}
