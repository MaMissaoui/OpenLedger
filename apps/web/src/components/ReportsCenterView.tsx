import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "../lib/api";
import type { Book, Numeric } from "../lib/types";
import { formatMoney } from "../lib/money";

// Statements that live inside ReportsView (selected by tab).
type StatementTab = "balance-sheet" | "income-statement";
// Deep-linkable destinations elsewhere in the app.
type ReportView = "portfolio" | "budget" | "business";
type BizTab = "ar-aging" | "ap-aging";

interface Props {
  book: Book;
  onOpenStatement: (tab: StatementTab) => void;
  onOpenCashFlow: () => void;
  onOpenForecast: () => void;
  onOpenView: (view: ReportView, bizTab?: BizTab) => void;
}

function subtract(a: Numeric, b: Numeric): Numeric {
  if (a.denom === b.denom) return { num: a.num - b.num, denom: a.denom };
  return { num: a.num * b.denom - b.num * a.denom, denom: a.denom * b.denom };
}

function startOfYearISO(): string {
  return `${new Date().getFullYear()}-01-01T00:00:00Z`;
}
function todayISO(): string {
  return new Date().toISOString();
}

// One report entry in a library card.
function ReportRow({
  title,
  desc,
  onClick,
}: {
  title: string;
  desc: string;
  onClick: () => void;
}) {
  return (
    <button className="report-row" onClick={onClick}>
      <span className="report-row__text">
        <span className="report-row__title">{title}</span>
        <span className="report-row__desc">{desc}</span>
      </span>
      <span className="report-row__chevron" aria-hidden>
        ›
      </span>
    </button>
  );
}

const ICONS: Record<string, React.ReactNode> = {
  statements: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="2" width="12" height="14" rx="1.5" />
      <path d="M6 6h6M6 9h6M6 12h4" />
    </svg>
  ),
  investments: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 13l4-4 3 3 7-8" />
      <path d="M13 4h4v4" />
    </svg>
  ),
  business: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="6" width="14" height="10" rx="1" />
      <path d="M6 6V4a3 3 0 016 0v2" />
    </svg>
  ),
  planning: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 2v14M5 16h8" />
      <path d="M4 6H2l2 4 2-4zM14 6h2l-2 4-2-4z" />
      <path d="M4 6h10" />
    </svg>
  ),
};

export function ReportsCenterView({
  book,
  onOpenStatement,
  onOpenCashFlow,
  onOpenForecast,
  onOpenView,
}: Props) {
  const { t } = useTranslation();
  const balanceSheet = useQuery({
    queryKey: ["balance-sheet", book.guid, "center"],
    queryFn: () => api.getBalanceSheet(book.guid, todayISO()),
  });
  const income = useQuery({
    queryKey: ["income-statement", book.guid, "center"],
    queryFn: () => api.getIncomeStatement(book.guid, startOfYearISO(), todayISO()),
  });
  const arAging = useQuery({
    queryKey: ["ar-aging", book.guid],
    queryFn: () => api.arAgingReport(book.guid),
  });

  const netWorth = balanceSheet.data
    ? subtract(balanceSheet.data.assets.total, balanceSheet.data.liabilities.total)
    : null;
  const netIncome = income.data?.netIncome ?? null;
  const arTotal = arAging.data?.total ?? null;

  return (
    <section className="dash">
      <header className="dash__header">
        <div>
          <div className="eyebrow">{t("reports.center.eyebrow")}</div>
          <h1>{t("reports.center.title")}</h1>
        </div>
      </header>

      {/* Snapshot cards — live headline figures, each opens its full report. */}
      <div className="bento" style={{ marginBottom: "1.6rem" }}>
        <SnapCard
          label={t("reports.center.netWorth")}
          value={netWorth}
          hint={t("reports.center.netWorthHint")}
          tone="primary"
          onClick={() => onOpenStatement("balance-sheet")}
        />
        <SnapCard
          label={t("reports.center.netIncomeYtd")}
          value={netIncome}
          hint={t("reports.center.netIncomeHint")}
          tone={netIncome && netIncome.num < 0 ? "neg" : "pos"}
          onClick={() => onOpenStatement("income-statement")}
        />
        <SnapCard
          label={t("reports.center.receivables")}
          value={arTotal}
          hint={t("reports.center.receivablesHint")}
          tone="primary"
          onClick={() => onOpenView("business", "ar-aging")}
        />
      </div>

      {/* Report library */}
      <h2 className="report-lib__heading">{t("reports.center.reportLibrary")}</h2>
      <div className="bento">
        <LibraryCard icon={ICONS.statements} title={t("reports.center.statements")} count={3}>
          <ReportRow
            title={t("reports.center.balanceSheet")}
            desc={t("reports.center.balanceSheetDesc")}
            onClick={() => onOpenStatement("balance-sheet")}
          />
          <ReportRow
            title={t("reports.center.incomeStatement")}
            desc={t("reports.center.incomeStatementDesc")}
            onClick={() => onOpenStatement("income-statement")}
          />
          <ReportRow
            title={t("reports.center.cashFlowStatement")}
            desc={t("reports.center.cashFlowDesc")}
            onClick={onOpenCashFlow}
          />
        </LibraryCard>

        <LibraryCard icon={ICONS.investments} title={t("reports.center.investmentsGroup")} count={2}>
          <ReportRow
            title={t("reports.center.portfolio")}
            desc={t("reports.center.portfolioDesc")}
            onClick={() => onOpenView("portfolio")}
          />
          <ReportRow
            title={t("reports.center.capitalGains")}
            desc={t("reports.center.capitalGainsDesc")}
            onClick={() => onOpenView("portfolio")}
          />
        </LibraryCard>

        <LibraryCard icon={ICONS.business} title={t("reports.center.businessGroup")} count={2}>
          <ReportRow
            title={t("reports.center.arAging")}
            desc={t("reports.center.arAgingDesc")}
            onClick={() => onOpenView("business", "ar-aging")}
          />
          <ReportRow
            title={t("reports.center.apAging")}
            desc={t("reports.center.apAgingDesc")}
            onClick={() => onOpenView("business", "ap-aging")}
          />
        </LibraryCard>

        <LibraryCard icon={ICONS.planning} title={t("reports.center.planningGroup")} count={2}>
          <ReportRow
            title={t("reports.center.budgetActuals")}
            desc={t("reports.center.budgetDesc")}
            onClick={() => onOpenView("budget")}
          />
          <ReportRow
            title={t("reports.center.forecast")}
            desc={t("reports.center.forecastDesc")}
            onClick={onOpenForecast}
          />
        </LibraryCard>
      </div>
    </section>
  );
}

function SnapCard({
  label,
  value,
  hint,
  tone,
  onClick,
}: {
  label: string;
  value: Numeric | null;
  hint: string;
  tone: "primary" | "pos" | "neg";
  onClick: () => void;
}) {
  return (
    <button className="card card--span4 snap-card" onClick={onClick}>
      <span className="card__label">{label}</span>
      <span className={`stat mono snap-card__value snap-card__value--${tone}`}>
        {value ? formatMoney(value) : "—"}
      </span>
      <span className="snap-card__hint">{hint}</span>
    </button>
  );
}

function LibraryCard({
  icon,
  title,
  count,
  children,
}: {
  icon: React.ReactNode;
  title: string;
  count: number;
  children: React.ReactNode;
}) {
  return (
    <div className="card card--span6 card--flush lib-card">
      <div className="lib-card__head">
        <span className="lib-card__icon">{icon}</span>
        <h3 className="lib-card__title">{title}</h3>
        <span className="lib-card__count">
          {count} {count === 1 ? "report" : "reports"}
        </span>
      </div>
      <div className="lib-card__body">{children}</div>
    </div>
  );
}
