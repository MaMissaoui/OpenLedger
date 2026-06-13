import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./lib/api";
import { SetupLedger } from "./SetupLedger";
import { AccountTree } from "./components/AccountTree";
import { RegisterView } from "./components/RegisterView";
import { NewTransactionDialog } from "./components/NewTransactionDialog";
import { NewAccountDialog } from "./components/NewAccountDialog";

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
  const [showNewAccount, setShowNewAccount] = useState(false);

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
        <div className="topbar__right">
          <span className="topbar__user mono">book {book.guid.slice(0, 8)}…</span>
          <a
            className="btn btn--ghost btn--sm"
            href={`${import.meta.env.VITE_AUTHELIA_PORTAL_URL ?? "http://auth.openledger.localhost"}/logout`}
          >
            Sign out
          </a>
        </div>
      </header>

      <div className="workspace">
        <AccountTree
          accounts={accounts.data ?? []}
          selectedGuid={selectedGuid}
          onSelect={setSelectedGuid}
          onAddAccount={() => setShowNewAccount(true)}
        />

        {selected ? (
          <RegisterView account={selected} onNewTransaction={() => setShowNewTx(true)} />
        ) : (
          <div className="empty" style={{ alignSelf: "center", margin: "auto" }}>
            {accounts.isLoading ? <span className="spinner" /> : "Add an account to begin."}
          </div>
        )}
      </div>

      {showNewTx && selected && (
        <NewTransactionDialog
          accounts={postable}
          defaultToGuid={selected.guid}
          onClose={() => setShowNewTx(false)}
        />
      )}
      {showNewAccount && (
        <NewAccountDialog
          bookGuid={book.guid}
          accounts={accounts.data ?? []}
          onClose={() => setShowNewAccount(false)}
        />
      )}
    </div>
  );
}
