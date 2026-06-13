import type { Account } from "../lib/types";
import { TOP_LEVEL_ORDER } from "../lib/types";
import { formatMoney } from "../lib/money";

const BUCKET_LABEL: Record<string, string> = {
  ASSET: "Assets",
  LIABILITY: "Liabilities",
  EQUITY: "Equity",
  INCOME: "Income",
  EXPENSE: "Expenses",
};

// bucket maps a GnuCash account type onto one of the five top-level sections a
// chart of accounts rolls up into.
function bucket(type: string): string {
  if (["LIABILITY", "CREDIT", "PAYABLE"].includes(type)) return "LIABILITY";
  if (type === "EQUITY") return "EQUITY";
  if (type === "INCOME") return "INCOME";
  if (type === "EXPENSE") return "EXPENSE";
  return "ASSET"; // ASSET, BANK, CASH, STOCK, MUTUAL, RECEIVABLE, …
}

interface Props {
  accounts: Account[];
  selectedGuid: string | null;
  onSelect: (guid: string) => void;
  onAddAccount: () => void;
}

export function AccountTree({ accounts, selectedGuid, onSelect, onAddAccount }: Props) {
  // Only postable (non-placeholder) accounts are selectable rows; placeholder
  // "group" accounts are represented by the section headers.
  const postable = accounts.filter((a) => !a.placeholder && a.type !== "ROOT");

  return (
    <nav className="sidebar">
      <div className="sidebar__head">
        <h2>Accounts</h2>
        <button className="btn btn--ghost btn--sm" onClick={onAddAccount}>
          + Account
        </button>
      </div>

      {TOP_LEVEL_ORDER.map((b) => {
        const inBucket = postable
          .filter((a) => bucket(a.type) === b)
          .sort((x, y) => (x.code || x.name).localeCompare(y.code || y.name));
        if (inBucket.length === 0) return null;
        return (
          <div className="acct-group" key={b}>
            <div className="acct-group__label">{BUCKET_LABEL[b]}</div>
            {inBucket.map((a) => (
              <button
                key={a.guid}
                className={`acct acct--child${a.guid === selectedGuid ? " acct--active" : ""}`}
                onClick={() => onSelect(a.guid)}
              >
                {a.code && <span className="acct__code">{a.code}</span>}
                <span className="acct__name">{a.name}</span>
                {a.balance && (
                  <span className="acct__balance">{formatMoney(a.balance)}</span>
                )}
              </button>
            ))}
          </div>
        );
      })}
    </nav>
  );
}
