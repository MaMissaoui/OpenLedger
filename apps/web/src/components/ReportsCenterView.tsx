import { useQuery } from "@tanstack/react-query";
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

export function ReportsCenterView({ book, onOpenStatement, onOpenView }: Props) {
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
          <div className="eyebrow">Reports</div>
          <h1>Reports Center</h1>
        </div>
      </header>

      {/* Snapshot cards — live headline figures, each opens its full report. */}
      <div className="bento" style={{ marginBottom: "1.6rem" }}>
        <SnapCard
          label="Net Worth"
          value={netWorth}
          hint="Assets less liabilities · today"
          tone="primary"
          onClick={() => onOpenStatement("balance-sheet")}
        />
        <SnapCard
          label="Net Income · YTD"
          value={netIncome}
          hint="Income less expenses this year"
          tone={netIncome && netIncome.num < 0 ? "neg" : "pos"}
          onClick={() => onOpenStatement("income-statement")}
        />
        <SnapCard
          label="Outstanding Receivables"
          value={arTotal}
          hint="Open customer invoices"
          tone="primary"
          onClick={() => onOpenView("business", "ar-aging")}
        />
      </div>

      {/* Report library */}
      <h2 className="report-lib__heading">Report Library</h2>
      <div className="bento">
        <LibraryCard icon={ICONS.statements} title="Financial Statements" count={2}>
          <ReportRow
            title="Balance Sheet"
            desc="Assets, liabilities, and equity as of a date."
            onClick={() => onOpenStatement("balance-sheet")}
          />
          <ReportRow
            title="Income Statement (P&L)"
            desc="Revenue and expenses over a period."
            onClick={() => onOpenStatement("income-statement")}
          />
        </LibraryCard>

        <LibraryCard icon={ICONS.investments} title="Investments" count={2}>
          <ReportRow
            title="Portfolio Holdings"
            desc="Shares, cost basis, and market value by security."
            onClick={() => onOpenView("portfolio")}
          />
          <ReportRow
            title="Realized Capital Gains"
            desc="FIFO gains and losses from security sales."
            onClick={() => onOpenView("portfolio")}
          />
        </LibraryCard>

        <LibraryCard icon={ICONS.business} title="Receivables & Payables" count={2}>
          <ReportRow
            title="A/R Aging"
            desc="Outstanding customer invoices by age bucket."
            onClick={() => onOpenView("business", "ar-aging")}
          />
          <ReportRow
            title="A/P Aging"
            desc="Outstanding vendor bills by age bucket."
            onClick={() => onOpenView("business", "ap-aging")}
          />
        </LibraryCard>

        <LibraryCard icon={ICONS.planning} title="Planning" count={1}>
          <ReportRow
            title="Budget vs. Actuals"
            desc="Variance of actual spend against budget."
            onClick={() => onOpenView("budget")}
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
