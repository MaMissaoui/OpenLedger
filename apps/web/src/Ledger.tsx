import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./lib/api";
import { SetupLedger } from "./SetupLedger";
import { DashboardView } from "./components/DashboardView";
import { AccountTree } from "./components/AccountTree";
import { RegisterView } from "./components/RegisterView";
import { ReportsCenterView } from "./components/ReportsCenterView";
import { ReportsView } from "./components/ReportsView";
import { CashFlowView } from "./components/CashFlowView";
import { ForecastView } from "./components/ForecastView";
import { PortfolioView } from "./components/PortfolioView";
import { TransactionDialog } from "./components/TransactionDialog";
import { TradeSecurityDialog } from "./components/TradeSecurityDialog";
import { NewAccountDialog } from "./components/NewAccountDialog";
import { ScheduledTransactionsView } from "./components/ScheduledTransactionsView";
import BudgetView from "./components/BudgetView";
import BusinessView from "./components/BusinessView";
import CommoditiesView from "./components/CommoditiesView";
import SettingsView from "./components/SettingsView";
import ImportDialog from "./components/ImportDialog";

type View =
  | "dashboard"
  | "ledger"
  | "reports"
  | "statements"
  | "cash-flow"
  | "forecast"
  | "portfolio"
  | "scheduled"
  | "budget"
  | "business"
  | "commodities"
  | "settings";

type StatementTab = "balance-sheet" | "income-statement";
type BizTab = "ar-aging" | "ap-aging";

// ── icon set (18×18 SVG outlines) ────────────────────────────────────────────
const Icon = {
  dashboard: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="2" width="6" height="6" rx="1" />
      <rect x="10" y="2" width="6" height="4" rx="1" />
      <rect x="2" y="11" width="6" height="5" rx="1" />
      <rect x="10" y="9" width="6" height="7" rx="1" />
    </svg>
  ),
  ledger: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 2h9l3 3v11a1 1 0 01-1 1H3a1 1 0 01-1-1V3a1 1 0 011-1z" />
      <path d="M12 2v4h4M5 9h8M5 12h8M5 15h5" />
    </svg>
  ),
  reports: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M2 16V9M6 16V5M10 16V11M14 16V2" />
      <path d="M1 17h16" />
    </svg>
  ),
  portfolio: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M1 13l5-5 4 3 6-8" />
      <path d="M13 3h4v4" />
    </svg>
  ),
  scheduled: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <rect x="2" y="4" width="14" height="13" rx="1.5" />
      <path d="M2 8h14M6 2v4M12 2v4" />
    </svg>
  ),
  budget: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M9 2v14M5 16h8" />
      <path d="M4 6H2l2 4 2-4zM14 6h2l-2 4-2-4z" />
      <path d="M4 6h10" />
    </svg>
  ),
  business: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="7" width="14" height="10" rx="1" />
      <path d="M6 7V5a3 3 0 016 0v2" />
      <path d="M9 11v2" />
    </svg>
  ),
  commodities: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <ellipse cx="9" cy="4" rx="6" ry="2.2" />
      <path d="M3 4v5c0 1.2 2.7 2.2 6 2.2s6-1 6-2.2V4" />
      <path d="M3 9v5c0 1.2 2.7 2.2 6 2.2s6-1 6-2.2V9" />
    </svg>
  ),
  settings: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="9" cy="9" r="2.4" />
      <path d="M9 1.5v2M9 14.5v2M1.5 9h2M14.5 9h2M3.7 3.7l1.4 1.4M12.9 12.9l1.4 1.4M14.3 3.7l-1.4 1.4M5.1 12.9l-1.4 1.4" />
    </svg>
  ),
  download: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 2v9M5 8l4 4 4-4" />
      <path d="M2 14v2a1 1 0 001 1h12a1 1 0 001-1v-2" />
    </svg>
  ),
  upload: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 12V3M5 7l4-4 4 4" />
      <path d="M2 14v2a1 1 0 001 1h12a1 1 0 001-1v-2" />
    </svg>
  ),
  signout: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M7 17H3a1 1 0 01-1-1V2a1 1 0 011-1h4M12 13l4-4-4-4M16 9H7" />
    </svg>
  ),
  plus: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <path d="M9 3v12M3 9h12" />
    </svg>
  ),
  // Panel-with-chevron glyphs — read clearly as "slide this side panel
  // closed/open." (Lucide panel-left-close / panel-left-open, 24px grid.)
  collapseMenu: (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M9 3v18" />
      <path d="m16 15-3-3 3-3" />
    </svg>
  ),
  expandMenu: (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M9 3v18" />
      <path d="m14 9 3 3-3 3" />
    </svg>
  ),
};

// ── nav item ─────────────────────────────────────────────────────────────────
function NavItem({
  label,
  icon,
  active,
  collapsed,
  onClick,
}: {
  label: string;
  icon: React.ReactNode;
  active: boolean;
  collapsed: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={`sidenav__item${active ? " sidenav__item--active" : ""}`}
      onClick={onClick}
      title={collapsed ? label : undefined}
    >
      <span className="sidenav__icon">{icon}</span>
      <span className="sidenav__label">{label}</span>
    </button>
  );
}

// ── Ledger ────────────────────────────────────────────────────────────────────
export function Ledger() {
  const books = useQuery({ queryKey: ["books"], queryFn: api.listBooks });
  const book = books.data?.[0] ?? null;

  const accounts = useQuery({
    queryKey: ["accounts", book?.guid],
    queryFn: () => api.listAccounts(book!.guid),
    enabled: book !== null,
  });

  const [selectedGuid, setSelectedGuid] = useState<string | null>(null);
  const [showNewTx, setShowNewTx] = useState(false);
  const [editTxGuid, setEditTxGuid] = useState<string | null>(null);
  const [showNewAccount, setShowNewAccount] = useState(false);
  const [showTrade, setShowTrade] = useState(false);
  const [showImport, setShowImport] = useState(false);
  const [view, setView] = useState<View>("dashboard");
  const [collapsed, setCollapsed] = useState(false);
  // Drill-down targets selected from the Reports Center hub.
  const [statementTab, setStatementTab] = useState<StatementTab>("balance-sheet");
  const [businessTab, setBusinessTab] = useState<BizTab | undefined>(undefined);

  function openStatement(tab: StatementTab) {
    setStatementTab(tab);
    setView("statements");
  }
  function openReportView(target: "portfolio" | "budget" | "business", bizTab?: BizTab) {
    setBusinessTab(bizTab);
    setView(target);
  }

  const postable = (accounts.data ?? []).filter((a) => !a.placeholder && a.type !== "ROOT");

  useEffect(() => {
    if (postable.length === 0) {
      setSelectedGuid(null);
    } else if (!postable.some((a) => a.guid === selectedGuid)) {
      setSelectedGuid(postable[0].guid);
    }
  }, [postable, selectedGuid]);

  if (books.isLoading) {
    return (
      <div className="setup">
        <span className="spinner" />
      </div>
    );
  }

  if (!book) return <SetupLedger />;

  const selected = postable.find((a) => a.guid === selectedGuid) ?? null;

  return (
    <div className="shell">
      {/* ── left sidebar ── */}
      <nav className={`sidenav${collapsed ? " sidenav--collapsed" : ""}`}>
        {/* Brand */}
        <div className="sidenav__brand">
          <span className="sidenav__brand-mark">OL</span>
          <div className="sidenav__brand-text">
            <span className="sidenav__brand-name">OpenLedger</span>
            <span className="sidenav__brand-tag">Enterprise Finance</span>
          </div>
          <button
            className="sidenav__toggle"
            onClick={() => setCollapsed((c) => !c)}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {collapsed ? Icon.expandMenu : Icon.collapseMenu}
          </button>
        </div>

        {/* Nav */}
        <div className="sidenav__nav">
          <NavItem label="Dashboard"  icon={Icon.dashboard} active={view === "dashboard"} collapsed={collapsed} onClick={() => setView("dashboard")} />
          <NavItem label="Ledger"     icon={Icon.ledger}    active={view === "ledger"}    collapsed={collapsed} onClick={() => setView("ledger")} />
          <NavItem label="Reports"    icon={Icon.reports}   active={view === "reports" || view === "statements" || view === "cash-flow" || view === "forecast"} collapsed={collapsed} onClick={() => setView("reports")} />
          <NavItem label="Portfolio"  icon={Icon.portfolio} active={view === "portfolio"} collapsed={collapsed} onClick={() => setView("portfolio")} />
          <NavItem label="Scheduled"  icon={Icon.scheduled} active={view === "scheduled"} collapsed={collapsed} onClick={() => setView("scheduled")} />
          <NavItem label="Budget"     icon={Icon.budget}    active={view === "budget"}    collapsed={collapsed} onClick={() => setView("budget")} />
          <NavItem label="Business"   icon={Icon.business}  active={view === "business"}  collapsed={collapsed} onClick={() => { setBusinessTab(undefined); setView("business"); }} />
          <NavItem label="Commodities" icon={Icon.commodities} active={view === "commodities"} collapsed={collapsed} onClick={() => setView("commodities")} />
          <NavItem label="Settings"    icon={Icon.settings}    active={view === "settings"}    collapsed={collapsed} onClick={() => setView("settings")} />
        </div>

        {/* Footer */}
        <div className="sidenav__footer">
          {/* New Transaction CTA */}
          <button
            className="sidenav__cta"
            onClick={() => { setView("ledger"); setShowNewTx(true); }}
            title={collapsed ? "New transaction" : undefined}
          >
            <span className="sidenav__icon">{Icon.plus}</span>
            <span className="sidenav__cta-label">New Transaction</span>
          </button>

          <span className="sidenav__book-id mono">book {book.guid.slice(0, 8)}…</span>
          <button
            className="sidenav__link"
            onClick={() => setShowImport(true)}
            title={collapsed ? "Import GnuCash" : undefined}
          >
            <span className="sidenav__link-icon">{Icon.upload}</span>
            <span className="sidenav__link-text">Import GnuCash</span>
          </button>
          <a
            className="sidenav__link"
            href={api.exportGnuCashUrl(book.guid)}
            download={`${book.guid}.gnucash`}
            title={collapsed ? "Export SQLite" : undefined}
          >
            <span className="sidenav__link-icon">{Icon.download}</span>
            <span className="sidenav__link-text">Export SQLite</span>
          </a>
          <a
            className="sidenav__link"
            href={api.exportGnuCashUrl(book.guid, "xml")}
            download={`${book.guid}.gnucash`}
            title={collapsed ? "Export XML" : undefined}
          >
            <span className="sidenav__link-icon">{Icon.download}</span>
            <span className="sidenav__link-text">Export XML</span>
          </a>
          <a
            className="sidenav__link"
            href={`${import.meta.env.VITE_AUTHELIA_PORTAL_URL ?? "http://auth.openledger.localhost"}/logout`}
            title={collapsed ? "Sign out" : undefined}
          >
            <span className="sidenav__link-icon">{Icon.signout}</span>
            <span className="sidenav__link-text">Sign out</span>
          </a>
        </div>
      </nav>

      {/* ── main content ── */}
      <div className="shell__content">
        {view === "dashboard" ? (
          <DashboardView book={book} onNavigate={setView} />
        ) : view === "reports" ? (
          <ReportsCenterView
            book={book}
            onOpenStatement={openStatement}
            onOpenCashFlow={() => setView("cash-flow")}
            onOpenForecast={() => setView("forecast")}
            onOpenView={openReportView}
          />
        ) : view === "statements" ? (
          <ReportsView book={book} initialTab={statementTab} onBack={() => setView("reports")} />
        ) : view === "cash-flow" ? (
          <CashFlowView book={book} onBack={() => setView("reports")} />
        ) : view === "forecast" ? (
          <ForecastView book={book} onBack={() => setView("reports")} />
        ) : view === "portfolio" ? (
          <PortfolioView book={book} onTrade={() => setShowTrade(true)} />
        ) : view === "scheduled" ? (
          <ScheduledTransactionsView book={book} accounts={accounts.data ?? []} />
        ) : view === "budget" ? (
          <BudgetView bookGuid={book.guid} />
        ) : view === "business" ? (
          <BusinessView bookGuid={book.guid} accounts={accounts.data ?? []} initialTab={businessTab} />
        ) : view === "commodities" ? (
          <CommoditiesView />
        ) : view === "settings" ? (
          <SettingsView bookGuid={book.guid} />
        ) : (
          <div className="workspace">
            <AccountTree
              accounts={accounts.data ?? []}
              rootGuid={book.rootAccountGuid}
              selectedGuid={selectedGuid}
              onSelect={setSelectedGuid}
              onAddAccount={() => setShowNewAccount(true)}
            />
            {selected ? (
              <RegisterView
                account={selected}
                onNewTransaction={() => setShowNewTx(true)}
                onEditTransaction={setEditTxGuid}
              />
            ) : (
              <div className="empty" style={{ alignSelf: "center", margin: "auto" }}>
                {accounts.isLoading ? <span className="spinner" /> : "Add an account to begin."}
              </div>
            )}
          </div>
        )}
      </div>

      {/* ── dialogs ── */}
      {showNewTx && selected && (
        <TransactionDialog
          accounts={postable}
          defaultToGuid={selected.guid}
          onClose={() => setShowNewTx(false)}
        />
      )}
      {editTxGuid && (
        <TransactionDialog
          accounts={postable}
          defaultToGuid={selected?.guid ?? null}
          editGuid={editTxGuid}
          onClose={() => setEditTxGuid(null)}
        />
      )}
      {showNewAccount && (
        <NewAccountDialog
          bookGuid={book.guid}
          accounts={accounts.data ?? []}
          onClose={() => setShowNewAccount(false)}
        />
      )}
      {showTrade && (
        <TradeSecurityDialog accounts={postable} onClose={() => setShowTrade(false)} />
      )}
      {showImport && <ImportDialog onClose={() => setShowImport(false)} />}
    </div>
  );
}
