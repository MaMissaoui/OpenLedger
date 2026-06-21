import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "../lib/api";
import type { Account, Numeric } from "../lib/types";
import { formatMoney } from "../lib/money";
import ImportStatementDialog from "./ImportStatementDialog";

// Account types that hold a currency balance and so accept OFX/QIF statement
// imports. Income/expense and security accounts are excluded.
const STATEMENT_TYPES = new Set(["BANK", "CASH", "CREDIT", "ASSET", "LIABILITY"]);

interface Props {
  account: Account;
  onNewTransaction: () => void;
  onEditTransaction: (txGuid: string) => void;
}

function amountCell(n: Numeric) {
  const cls = `num${n.num < 0 ? " neg" : ""}`;
  return <td className={cls}>{n.num === 0 ? "—" : formatMoney(n)}</td>;
}

// Reconcile flags cycle n → c → y on click. Each maps to a glyph.
// Titles are now from the i18n catalog.
const RECONCILE_CYCLE: Record<string, string> = { n: "c", c: "y", y: "n" };
const RECONCILE_GLYPH: Record<string, string> = { n: "○", c: "c", y: "✓" };
const RECONCILE_TITLE_KEY: Record<string, "register.reconTitles.n" | "register.reconTitles.c" | "register.reconTitles.y"> = {
  n: "register.reconTitles.n",
  c: "register.reconTitles.c",
  y: "register.reconTitles.y",
};

function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "2-digit" });
}

export function RegisterView({ account, onNewTransaction, onEditTransaction }: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [showImport, setShowImport] = useState(false);
  const { data, isLoading } = useQuery({
    queryKey: ["register", account.guid],
    queryFn: () => api.getRegister(account.guid),
  });

  const del = useMutation({
    mutationFn: (txGuid: string) => api.deleteTransaction(txGuid),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["register"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["balance-sheet"] });
      qc.invalidateQueries({ queryKey: ["income-statement"] });
    },
  });

  const recon = useMutation({
    mutationFn: ({ splitGuid, state }: { splitGuid: string; state: string }) =>
      api.reconcileSplit(splitGuid, state),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["register", account.guid] }),
  });

  function confirmDelete(txGuid: string, description: string) {
    const msg = t("register.deleteConfirm", { description: description || t("register.deleteDefault") });
    if (window.confirm(msg)) {
      del.mutate(txGuid);
    }
  }

  const entries = data?.entries ?? [];
  const currentBalance = entries.length > 0 ? entries[entries.length - 1].balance : null;

  return (
    <section className="register">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">
            {account.type}
            {account.code ? ` · ${account.code}` : ""}
          </div>
          <h1>{account.name}</h1>
        </div>

        <div style={{ display: "flex", alignItems: "flex-end", gap: "1.4rem" }}>
          {currentBalance && (
            <div className="register__balance">
              <div className="eyebrow">{t("register.balance")}</div>
              <div className={`amt${currentBalance.num < 0 ? " neg" : ""}`}>
                {formatMoney(currentBalance)}
              </div>
            </div>
          )}
          <div className="register__actions">
            {STATEMENT_TYPES.has(account.type) && (
              <button className="btn btn--ghost" onClick={() => setShowImport(true)}>
                {t("register.importStatement")}
              </button>
            )}
            <button className="btn btn--primary" onClick={onNewTransaction}>
              {t("register.newTransaction")}
            </button>
          </div>
        </div>
      </header>

      {isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : entries.length === 0 ? (
        <div className="empty">
          {t("register.noEntries")}
        </div>
      ) : (
        <table className="ledger-table">
          <thead>
            <tr>
              <th>{t("common.date")}</th>
              <th>{t("common.description")}</th>
              <th className="num">{t("common.amount")}</th>
              <th className="num">{t("common.balance")}</th>
              <th className="recon-col" title={t("register.reconciledTitle")}>R</th>
              <th className="row-actions" aria-label="Actions" />
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.splitGuid}>
                <td className="date">{formatDate(e.postDate)}</td>
                <td>
                  <div className="desc">{e.description || "—"}</div>
                  {e.memo && <div className="memo">{e.memo}</div>}
                </td>
                {amountCell(e.quantity)}
                <td className={`num balance${e.balance.num < 0 ? " neg" : ""}`}>
                  {formatMoney(e.balance)}
                </td>
                <td className="recon-col">
                  <button
                    className={`recon recon--${e.reconcile}`}
                    onClick={() =>
                      recon.mutate({
                        splitGuid: e.splitGuid,
                        state: RECONCILE_CYCLE[e.reconcile] ?? "c",
                      })
                    }
                    disabled={recon.isPending}
                    title={t(RECONCILE_TITLE_KEY[e.reconcile] ?? "register.reconTitles.n")}
                  >
                    {RECONCILE_GLYPH[e.reconcile] ?? e.reconcile}
                  </button>
                </td>
                <td className="row-actions">
                  <button
                    className="row-actions__btn"
                    onClick={() => onEditTransaction(e.txGuid)}
                    title={t("register.editTransaction")}
                  >
                    {t("register.edit")}
                  </button>
                  <button
                    className="row-actions__btn row-actions__btn--danger"
                    onClick={() => confirmDelete(e.txGuid, e.description)}
                    disabled={del.isPending}
                    title={t("register.deleteTransaction")}
                  >
                    {t("register.delete")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {showImport && (
        <ImportStatementDialog account={account} onClose={() => setShowImport(false)} />
      )}
    </section>
  );
}
