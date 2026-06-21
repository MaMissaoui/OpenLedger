import { useTranslation } from "react-i18next";
import type { Account, Numeric } from "../lib/types";
import { TOP_LEVEL_ORDER } from "../lib/types";
import { formatMoney, sumBalances } from "../lib/money";

const BUCKET_KEY: Record<string, string> = {
  ASSET:     "reports.balanceSheet.assets",
  LIABILITY: "reports.balanceSheet.liabilities",
  EQUITY:    "reports.balanceSheet.equity",
  INCOME:    "reports.incomeStatement.income",
  EXPENSE:   "reports.incomeStatement.expenses",
};

function bucket(type: string): string {
  if (["LIABILITY", "CREDIT", "PAYABLE"].includes(type)) return "LIABILITY";
  if (type === "EQUITY") return "EQUITY";
  if (type === "INCOME") return "INCOME";
  if (type === "EXPENSE") return "EXPENSE";
  return "ASSET";
}

interface Props {
  accounts: Account[];
  rootGuid: string;
  selectedGuid: string | null;
  onSelect: (guid: string) => void;
  onAddAccount: () => void;
}

export function AccountTree({ accounts, rootGuid, selectedGuid, onSelect, onAddAccount }: Props) {
  const { t } = useTranslation();
  const postable = accounts.filter((a) => !a.placeholder && a.type !== "ROOT");

  const sectionTotal = (b: string): Numeric | null => {
    const tops = accounts.filter(
      (a) => a.parentGuid === rootGuid && bucket(a.type) === b && a.subtreeBalance,
    );
    return sumBalances(tops.map((a) => a.subtreeBalance!));
  };

  return (
    <nav className="sidebar">
      <div className="sidebar__head">
        <h2>{t("accounts.title")}</h2>
        <button className="btn btn--ghost btn--sm" onClick={onAddAccount}>
          {t("accounts.addAccount")}
        </button>
      </div>

      {TOP_LEVEL_ORDER.map((b) => {
        const inBucket = postable
          .filter((a) => bucket(a.type) === b)
          .sort((x, y) => (x.code || x.name).localeCompare(y.code || y.name));
        if (inBucket.length === 0) return null;
        const total = sectionTotal(b);
        return (
          <div className="acct-group" key={b}>
            <div className="acct-group__label">
              <span>{t(BUCKET_KEY[b] ?? b)}</span>
              {total && <span className="acct-group__total">{formatMoney(total)}</span>}
            </div>
            {inBucket.map((a) => (
              <button
                key={a.guid}
                className={`acct acct--child${a.guid === selectedGuid ? " acct--active" : ""}`}
                onClick={() => onSelect(a.guid)}
              >
                {a.code && <span className="acct__code">{a.code}</span>}
                <span className="acct__name">{a.name}</span>
                {a.balance && (
                  <span className={`acct__balance${a.balance.num < 0 ? " neg" : ""}`}>
                    {formatMoney(a.balance)}
                  </span>
                )}
              </button>
            ))}
          </div>
        );
      })}
    </nav>
  );
}
