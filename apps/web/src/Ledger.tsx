import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./lib/api";
import { SetupLedger } from "./SetupLedger";
import { AccountTree } from "./components/AccountTree";
import { RegisterView } from "./components/RegisterView";
import { ReportsView } from "./components/ReportsView";
import { PortfolioView } from "./components/PortfolioView";
import { TransactionDialog } from "./components/TransactionDialog";
import { TradeSecurityDialog } from "./components/TradeSecurityDialog";
import { NewAccountDialog } from "./components/NewAccountDialog";
import { ScheduledTransactionsView } from "./components/ScheduledTransactionsView";
import BudgetView from "./components/BudgetView";
import BusinessView from "./components/BusinessView";

type View = "ledger" | "reports" | "portfolio" | "scheduled" | "budget" | "business";

// ── icon set (simple 18×18 SVG outlines) ──────────────────────────────────────
const Icon = {
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
  download: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 2v9M5 8l4 4 4-4" />
      <path d="M2 14v2a1 1 0 001 1h12a1 1 0 001-1v-2" />
    </svg>
  ),
  signout: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M7 17H3a1 1 0 01-1-1V2a1 1 0 011-1h4M12 13l4-4-4-4M16 9H7" />
    </svg>
  ),
  collapseMenu: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
      <path d="M7 5l-3 4 3 4" />
      <path d="M10 5h4M10 9h4M10 13h4" />
    </svg>
  ),
  expandMenu: (
    <svg viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 5h4M4 9h4M4 13h4" />
      <path d="M11 5l3 4-3 4" />
    </svg>
  ),
};

// ── nav item ──────────────────────────────────────────────────────────────────
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
  const [view, setView] = useState<View>("ledger");
  const [collapsed, setCollapsed] = useState(false);

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
        <div className="sidenav__brand">
          <span className="sidenav__brand-mark">OL</span>
          <div className="sidenav__brand-text">
            <span className="sidenav__brand-name">OpenLedger</span>
            <span className="sidenav__brand-tag">double-entry</span>
          </div>
          <button
            className="sidenav__toggle"
            onClick={() => setCollapsed((c) => !c)}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {collapsed ? Icon.expandMenu : Icon.collapseMenu}
          </button>
        </div>

        <div className="sidenav__nav">
          <NavItem label="Ledger" icon={Icon.ledger} active={view === "ledger"} collapsed={collapsed} onClick={() => setView("ledger")} />
          <NavItem label="Reports" icon={Icon.reports} active={view === "reports"} collapsed={collapsed} onClick={() => setView("reports")} />
          <NavItem label="Portfolio" icon={Icon.portfolio} active={view === "portfolio"} collapsed={collapsed} onClick={() => setView("portfolio")} />
          <NavItem label="Scheduled" icon={Icon.scheduled} active={view === "scheduled"} collapsed={collapsed} onClick={() => setView("scheduled")} />
          <NavItem label="Budget" icon={Icon.budget} active={view === "budget"} collapsed={collapsed} onClick={() => setView("budget")} />
          <NavItem label="Business" icon={Icon.business} active={view === "business"} collapsed={collapsed} onClick={() => setView("business")} />
        </div>

        <div className="sidenav__footer">
          <span className="sidenav__book-id mono">book {book.guid.slice(0, 8)}…</span>
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
        {view === "reports" ? (
          <ReportsView book={book} />
        ) : view === "portfolio" ? (
          <PortfolioView book={book} onTrade={() => setShowTrade(true)} />
        ) : view === "scheduled" ? (
          <ScheduledTransactionsView book={book} accounts={accounts.data ?? []} />
        ) : view === "budget" ? (
          <BudgetView bookGuid={book.guid} />
        ) : view === "business" ? (
          <BusinessView bookGuid={book.guid} />
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
    </div>
  );
}
