import { useQueries, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "../lib/api";
import type {
  Account,
  BalanceSheet,
  Book,
  IncomeStatement,
  Numeric,
  RegisterEntry,
} from "../lib/types";
import { formatMoney, toFloat } from "../lib/money";

interface Props {
  book: Book;
  onNavigate: (view: "ledger" | "reports") => void;
}

// ── date helpers ──────────────────────────────────────────────────────────────
function startOfYearISO(): string {
  return `${new Date().getFullYear()}-01-01T00:00:00Z`;
}
function todayISO(): string {
  return new Date().toISOString();
}
function monthAgoISO(): string {
  const d = new Date();
  d.setMonth(d.getMonth() - 1);
  return d.toISOString();
}
function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleDateString(undefined, { month: "short", day: "2-digit", year: "numeric" });
}

// subtract returns a − b as an exact Numeric, aligning denominators when they
// differ (cross-commodity totals are approximate but the dashboard is a glance,
// not a ledger).
function subtract(a: Numeric, b: Numeric): Numeric {
  if (a.denom === b.denom) return { num: a.num - b.num, denom: a.denom };
  return { num: a.num * b.denom - b.num * a.denom, denom: a.denom * b.denom };
}

// bucketIsAsset mirrors the chart-of-accounts roll-up: anything that isn't a
// liability/equity/income/expense lands in the asset column.
function bucketIsAsset(type: string): boolean {
  return !["LIABILITY", "CREDIT", "PAYABLE", "EQUITY", "INCOME", "EXPENSE"].includes(type);
}

// Donut palette — top categories then a muted "Others".
const SEGMENT_COLORS = ["#1a365d", "#555f71", "#4bb278", "#86a0cd", "#c4c6cf"];

export function DashboardView({ book, onNavigate }: Props) {
  const { t } = useTranslation();
  const balanceSheet = useQuery({
    queryKey: ["balance-sheet", book.guid, "dashboard"],
    queryFn: () => api.getBalanceSheet(book.guid, todayISO()),
  });
  const priorSheet = useQuery({
    queryKey: ["balance-sheet", book.guid, "dashboard-prior"],
    queryFn: () => api.getBalanceSheet(book.guid, monthAgoISO()),
  });
  const income = useQuery({
    queryKey: ["income-statement", book.guid, "dashboard"],
    queryFn: () => api.getIncomeStatement(book.guid, startOfYearISO(), todayISO()),
  });
  const accountsQ = useQuery({
    queryKey: ["accounts", book.guid],
    queryFn: () => api.listAccounts(book.guid),
  });

  // Recent activity is merged from the asset-class account registers (each cash
  // movement touches one), deduped by transaction so a transfer counts once.
  const assetAccounts = (accountsQ.data ?? []).filter(
    (a) => !a.placeholder && a.type !== "ROOT" && bucketIsAsset(a.type),
  );
  const registers = useQueries({
    queries: assetAccounts.map((a) => ({
      queryKey: ["register", a.guid],
      queryFn: () => api.getRegister(a.guid),
    })),
  });

  const loading = balanceSheet.isLoading || income.isLoading;

  return (
    <section className="dash">
      <header className="dash__header">
        <div>
          <div className="eyebrow">{t("dashboard.eyebrow")}</div>
          <h1>{t("dashboard.title")}</h1>
        </div>
        <button className="btn btn--ghost btn--sm" onClick={() => onNavigate("reports")}>
          {t("common.viewReports")}
        </button>
      </header>

      {loading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : (
        <div className="bento">
          <NetWorthCard sheet={balanceSheet.data} prior={priorSheet.data} />
          <CashFlowCard income={income.data} onNavigate={onNavigate} />
          <RecentTransactions registers={registers} accounts={assetAccounts} onNavigate={onNavigate} />
          <ExpenseBreakdown income={income.data} />
        </div>
      )}
    </section>
  );
}

// ── Net worth ─────────────────────────────────────────────────────────────────
function NetWorthCard({ sheet, prior }: { sheet?: BalanceSheet; prior?: BalanceSheet }) {
  const { t } = useTranslation();
  if (!sheet) return <div className="card card--span4" />;

  const netWorth = subtract(sheet.assets.total, sheet.liabilities.total);
  const liquidLines = sheet.assets.lines.filter(
    (l) => l.account.type === "BANK" || l.account.type === "CASH",
  );
  const liquid: Numeric = {
    num: Math.round(liquidLines.reduce((s, l) => s + toFloat(l.balance) * netWorth.denom, 0)),
    denom: netWorth.denom,
  };

  // Month-over-month change, shown only when there's a meaningful prior value.
  let trend: { pct: number; up: boolean } | null = null;
  if (prior) {
    const prev = subtract(prior.assets.total, prior.liabilities.total);
    if (prev.num !== 0) {
      const delta = toFloat(netWorth) - toFloat(prev);
      const pct = (delta / Math.abs(toFloat(prev))) * 100;
      if (Math.abs(pct) >= 0.05) trend = { pct, up: delta >= 0 };
    }
  }

  return (
    <div className="card card--span4 card--feature">
      <div className="card__label">{t("dashboard.netWorth")}</div>
      <div className="stat mono">{formatMoney(netWorth)}</div>
      {trend && (
        <div className="trend-row">
          <span className={`trend-chip ${trend.up ? "trend-chip--up" : "trend-chip--down"}`}>
            {trend.up ? "▲" : "▼"} {Math.abs(trend.pct).toFixed(1)}%
          </span>
          <span className="trend-note">{t("dashboard.vsLastMonth")}</span>
        </div>
      )}
      <div className="card__split">
        <div>
          <div className="card__split-label">{t("dashboard.liquidCash")}</div>
          <div className="mono card__split-value">{formatMoney(liquid)}</div>
        </div>
        <div className="card__split-right">
          <div className="card__split-label">{t("dashboard.liabilities")}</div>
          <div className={`mono card__split-value${sheet.liabilities.total.num !== 0 ? " neg" : ""}`}>
            {formatMoney(sheet.liabilities.total)}
          </div>
        </div>
      </div>
    </div>
  );
}

// ── Cash flow (income vs expenses for the year) ───────────────────────────────
function CashFlowCard({
  income,
  onNavigate,
}: {
  income?: IncomeStatement;
  onNavigate: (view: "ledger" | "reports") => void;
}) {
  const { t } = useTranslation();
  if (!income) return <div className="card card--span8" />;

  const inc = toFloat(income.income.total);
  const exp = toFloat(income.expense.total);
  const max = Math.max(inc, exp, 1);

  return (
    <div className="card card--span8">
      <div className="card__head">
        <div className="card__label">{t("dashboard.cashFlow")}</div>
        <div className="legend">
          <span className="legend__item">
            <span className="legend__dot legend__dot--in" /> {t("dashboard.income")}
          </span>
          <span className="legend__item">
            <span className="legend__dot legend__dot--out" /> {t("dashboard.expenses")}
          </span>
        </div>
      </div>

      <div className="flow-bars">
        <div className="flow-bar">
          <div className="flow-bar__label">{t("dashboard.income")}</div>
          <div className="flow-bar__track">
            <div className="flow-bar__fill flow-bar__fill--in" style={{ width: `${(inc / max) * 100}%` }} />
          </div>
          <div className="flow-bar__value mono">{formatMoney(income.income.total)}</div>
        </div>
        <div className="flow-bar">
          <div className="flow-bar__label">{t("dashboard.expenses")}</div>
          <div className="flow-bar__track">
            <div className="flow-bar__fill flow-bar__fill--out" style={{ width: `${(exp / max) * 100}%` }} />
          </div>
          <div className="flow-bar__value mono">{formatMoney(income.expense.total)}</div>
        </div>
      </div>

      <div className="card__foot">
        <div>
          <div className="card__split-label">{t("dashboard.netSavings")}</div>
          <div className={`mono flow-net${income.netIncome.num < 0 ? " neg" : ""}`}>
            {formatMoney(income.netIncome)}
          </div>
        </div>
        <button className="link-btn" onClick={() => onNavigate("reports")}>
          {t("common.viewReport")}
        </button>
      </div>
    </div>
  );
}

// ── Recent transactions ───────────────────────────────────────────────────────
const RECON_CLS: Record<string, string> = { y: "tag--ok", c: "tag--ok", n: "tag--muted" };
const RECON_KEY: Record<string, "dashboard.reconciled" | "dashboard.cleared" | "dashboard.pending"> = {
  y: "dashboard.reconciled",
  c: "dashboard.cleared",
  n: "dashboard.pending",
};

function RecentTransactions({
  registers,
  accounts,
  onNavigate,
}: {
  registers: { data?: { entries: RegisterEntry[] } }[];
  accounts: Account[];
  onNavigate: (view: "ledger" | "reports") => void;
}) {
  const { t } = useTranslation();
  // Merge entries, tagging each with its account, then dedupe by transaction.
  const seen = new Set<string>();
  const merged: { entry: RegisterEntry; accountName: string }[] = [];
  registers.forEach((r, i) => {
    const name = accounts[i]?.name ?? "";
    (r.data?.entries ?? []).forEach((entry) => {
      if (seen.has(entry.txGuid)) return;
      seen.add(entry.txGuid);
      merged.push({ entry, accountName: name });
    });
  });
  merged.sort((a, b) => b.entry.postDate.localeCompare(a.entry.postDate));
  const recent = merged.slice(0, 6);

  return (
    <div className="card card--span8 card--flush">
      <div className="card__head card__head--padded">
        <div className="card__label">{t("dashboard.recentTransactions")}</div>
        <button className="link-btn" onClick={() => onNavigate("ledger")}>
          {t("common.viewAll")}
        </button>
      </div>
      {recent.length === 0 ? (
        <div className="empty">{t("dashboard.noActivity")}</div>
      ) : (
        <table className="ledger-table ledger-table--flush">
          <thead>
            <tr>
              <th>{t("common.description")}</th>
              <th>{t("common.account")}</th>
              <th className="num">{t("common.amount")}</th>
              <th>{t("common.status")}</th>
            </tr>
          </thead>
          <tbody>
            {recent.map(({ entry, accountName }) => {
              const reconKey = RECON_KEY[entry.reconcile] ?? RECON_KEY.n;
              const reconCls = RECON_CLS[entry.reconcile] ?? RECON_CLS.n;
              return (
                <tr key={entry.splitGuid}>
                  <td>
                    <div className="desc">{entry.description || "—"}</div>
                    <div className="memo">{formatDate(entry.postDate)}</div>
                  </td>
                  <td className="memo">{accountName}</td>
                  <td className={`num${entry.quantity.num < 0 ? " neg" : ""}`}>
                    {formatMoney(entry.quantity)}
                  </td>
                  <td>
                    <span className={`tag ${reconCls}`}>{t(reconKey)}</span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}

// ── Expense breakdown (donut + list) ──────────────────────────────────────────
function ExpenseBreakdown({ income }: { income?: IncomeStatement }) {
  const { t } = useTranslation();
  if (!income) return <div className="card card--span4" />;

  const total = toFloat(income.expense.total);
  const sorted = [...income.expense.lines].sort((a, b) => toFloat(b.balance) - toFloat(a.balance));
  const top = sorted.slice(0, 4);
  const othersTotal = sorted.slice(4).reduce((s, l) => s + toFloat(l.balance), 0);

  const segments = top.map((l, i) => ({
    label: l.account.name,
    value: toFloat(l.balance),
    pct: total > 0 ? (toFloat(l.balance) / total) * 100 : 0,
    color: SEGMENT_COLORS[i],
  }));
  if (othersTotal > 0.005) {
    segments.push({
      label: t("dashboard.others"),
      value: othersTotal,
      pct: total > 0 ? (othersTotal / total) * 100 : 0,
      color: SEGMENT_COLORS[4],
    });
  }

  // Build cumulative dash offsets for the donut (circumference normalised to 100).
  let offset = 0;
  const arcs = segments.map((s) => {
    const arc = { ...s, dash: s.pct, gap: 100 - s.pct, off: offset };
    offset += s.pct;
    return arc;
  });

  return (
    <div className="card card--span4">
      <div className="card__label">{t("dashboard.expenseCategories")}</div>
      {total <= 0 ? (
        <div className="empty">{t("dashboard.noExpenses")}</div>
      ) : (
        <>
          <div className="donut-wrap">
            <svg viewBox="0 0 36 36" className="donut">
              <circle className="donut__bg" cx="18" cy="18" r="15.9155" />
              {arcs.map((a, i) => (
                <circle
                  key={i}
                  className="donut__seg"
                  cx="18"
                  cy="18"
                  r="15.9155"
                  stroke={a.color}
                  strokeDasharray={`${a.dash} ${a.gap}`}
                  strokeDashoffset={-a.off + 25}
                />
              ))}
            </svg>
            <div className="donut__center">
              <span className="donut__center-label">{t("dashboard.total")}</span>
              <span className="mono donut__center-value">{formatMoney(income.expense.total)}</span>
            </div>
          </div>
          <div className="cat-list">
            {segments.map((s, i) => (
              <div className="cat-row" key={i}>
                <span className="cat-dot" style={{ background: s.color }} />
                <span className="cat-name">{s.label}</span>
                <span className="mono cat-pct">{s.pct.toFixed(0)}%</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
