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
  const [view, setView] = useState<"ledger" | "reports" | "portfolio" | "scheduled" | "budget">("ledger");

  const postable = (accounts.data ?? []).filter((a) => !a.placeholder && a.type !== "ROOT");

  // Default the selection to the first postable account, and keep it valid as
  // the chart changes.
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
      <header className="topbar">
        <div className="topbar__brand">
          <span className="name">OpenLedger</span>
          <span className="tag">double-entry</span>
        </div>
        <nav className="topbar__nav">
          <button
            className={`topbar__tab${view === "ledger" ? " topbar__tab--active" : ""}`}
            onClick={() => setView("ledger")}
          >
            Ledger
          </button>
          <button
            className={`topbar__tab${view === "reports" ? " topbar__tab--active" : ""}`}
            onClick={() => setView("reports")}
          >
            Reports
          </button>
          <button
            className={`topbar__tab${view === "portfolio" ? " topbar__tab--active" : ""}`}
            onClick={() => setView("portfolio")}
          >
            Portfolio
          </button>
          <button
            className={`topbar__tab${view === "scheduled" ? " topbar__tab--active" : ""}`}
            onClick={() => setView("scheduled")}
          >
            Scheduled
          </button>
          <button
            className={`topbar__tab${view === "budget" ? " topbar__tab--active" : ""}`}
            onClick={() => setView("budget")}
          >
            Budget
          </button>
        </nav>
        <div className="topbar__right">
          <span className="topbar__user mono">book {book.guid.slice(0, 8)}…</span>
          <a
            className="btn btn--ghost btn--sm"
            href={api.exportGnuCashUrl(book.guid)}
            download={`${book.guid}.gnucash`}
          >
            Export SQLite
          </a>
          <a
            className="btn btn--ghost btn--sm"
            href={api.exportGnuCashUrl(book.guid, "xml")}
            download={`${book.guid}.gnucash`}
          >
            Export XML
          </a>
          <a
            className="btn btn--ghost btn--sm"
            href={`${import.meta.env.VITE_AUTHELIA_PORTAL_URL ?? "http://auth.openledger.localhost"}/logout`}
          >
            Sign out
          </a>
        </div>
      </header>

      {view === "reports" ? (
        <ReportsView book={book} />
      ) : view === "portfolio" ? (
        <PortfolioView book={book} onTrade={() => setShowTrade(true)} />
      ) : view === "scheduled" ? (
        <ScheduledTransactionsView book={book} accounts={accounts.data ?? []} />
      ) : view === "budget" ? (
        <BudgetView bookGuid={book.guid} />
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
