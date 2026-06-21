import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
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
          <td className="cf-empty">{t("reports.cashFlow.noPeriodActivity")}</td>
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
  const { t } = useTranslation();
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
              {t("reports.reportsCenter")}
            </button>
          ) : (
            <div className="eyebrow">{t("reports.cashFlow.eyebrow")}</div>
          )}
          <h1>{t("reports.cashFlow.title")}</h1>
        </div>
      </header>

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
        <div className="empty">{t("reports.cashFlow.loadError")}</div>
      ) : (
        q.data && (
          <>
            {/* Summary cards */}
            <div className="cf-summary">
              <div className="cf-card cf-card--primary">
                <span className="cf-card__label">{t("reports.cashFlow.endingCashCard")}</span>
                <span className="cf-card__value mono">{formatMoney(q.data.endingCash)}</span>
              </div>
              <div className="cf-card">
                <span className="cf-card__label">{t("reports.cashFlow.netChangeCard")}</span>
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
                  <th>{t("reports.cashFlow.activity")}</th>
                  <th className="amount">{t("common.amount")}</th>
                </tr>
              </thead>
              <tbody>
                <Section title={t("reports.cashFlow.operating")} section={q.data.operating} />
                <Section title={t("reports.cashFlow.investing")} section={q.data.investing} />
                <Section title={t("reports.cashFlow.financing")} section={q.data.financing} />
              </tbody>
              <tfoot>
                <tr className="cf-net">
                  <td>{t("reports.cashFlow.netChange")}</td>
                  <td className={`amount${q.data.netChange.num < 0 ? " neg" : ""}`}>
                    {formatMoney(q.data.netChange)}
                  </td>
                </tr>
                <tr className="cf-begin">
                  <td>{t("reports.cashFlow.beginningCash")}</td>
                  <td className="amount">{formatMoney(q.data.beginningCash)}</td>
                </tr>
                <tr className="cf-end">
                  <td>{t("reports.cashFlow.endingCash")}</td>
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
