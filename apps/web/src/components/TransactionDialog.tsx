import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "../lib/api";
import type { Account, Transaction } from "../lib/types";
import { formatMoney, negate, parseAmount } from "../lib/money";

interface Props {
  accounts: Account[];
  defaultToGuid: string | null;
  editGuid?: string;
  onClose: () => void;
}

interface Initial {
  description: string;
  amount: string;
  fromGuid: string;
  toGuid: string;
}

// TransactionDialog creates or edits a balanced two-split transfer.
export function TransactionDialog({ accounts, defaultToGuid, editGuid, onClose }: Props) {
  if (!editGuid) {
    return (
      <TransactionForm
        accounts={accounts}
        onClose={onClose}
        initial={{
          description: "",
          amount: "",
          toGuid: defaultToGuid ?? accounts[0]?.guid ?? "",
          fromGuid: accounts.find((a) => a.guid !== defaultToGuid)?.guid ?? "",
        }}
      />
    );
  }
  return <EditLoader accounts={accounts} editGuid={editGuid} onClose={onClose} />;
}

function EditLoader({ accounts, editGuid, onClose }: { accounts: Account[]; editGuid: string; onClose: () => void }) {
  const { t } = useTranslation();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["transaction", editGuid],
    queryFn: () => api.getTransaction(editGuid),
  });

  if (isLoading) {
    return (
      <Shell onClose={onClose} title={t("transaction.editTitle")}>
        <div className="empty"><span className="spinner" /></div>
      </Shell>
    );
  }
  if (isError || !data) {
    return (
      <Shell onClose={onClose} title={t("transaction.editTitle")}>
        <div className="error-note">{t("transaction.loadError")}</div>
        <DialogClose onClose={onClose} />
      </Shell>
    );
  }
  if (data.splits.length !== 2) {
    return (
      <Shell onClose={onClose} title={t("transaction.editTitle")}>
        <p className="sub">{t("transaction.splitsNote", { count: data.splits.length })}</p>
        <DialogClose onClose={onClose} />
      </Shell>
    );
  }
  return <TransactionForm accounts={accounts} editGuid={editGuid} onClose={onClose} initial={initialFromTransaction(data)} />;
}

function initialFromTransaction(tx: Transaction): Initial {
  const inflow  = tx.splits.find((s) => s.value.num > 0) ?? tx.splits[0];
  const outflow = tx.splits.find((s) => s.value.num < 0) ?? tx.splits[1];
  const amt = Math.abs(inflow.value.num) / inflow.value.denom;
  return { description: tx.description, amount: String(amt), toGuid: inflow.accountGuid, fromGuid: outflow.accountGuid };
}

function TransactionForm({
  accounts, initial, editGuid, onClose,
}: { accounts: Account[]; initial: Initial; editGuid?: string; onClose: () => void }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [description, setDescription] = useState(initial.description);
  const [amount, setAmount]           = useState(initial.amount);
  const [toGuid, setToGuid]           = useState(initial.toGuid);
  const [fromGuid, setFromGuid]       = useState(initial.fromGuid);
  const [error, setError]             = useState("");

  const parsed = parseAmount(amount);
  const valid  =
    parsed !== null && parsed.num > 0 && toGuid !== "" && fromGuid !== "" && toGuid !== fromGuid;

  const save = useMutation({
    mutationFn: async () => {
      const to    = accounts.find((a) => a.guid === toGuid)!;
      const value = parsed!;
      const input = {
        currencyGuid: to.commodityGuid,
        description:  description.trim() || "Transfer",
        splits: [
          { accountGuid: toGuid,   value,          quantity: value },
          { accountGuid: fromGuid, value: negate(value), quantity: negate(value) },
        ],
      };
      return editGuid ? api.updateTransaction(editGuid, input) : api.postTransaction(input);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["register"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["balance-sheet"] });
      qc.invalidateQueries({ queryKey: ["income-statement"] });
      onClose();
    },
    onError: (err) =>
      setError(err instanceof ApiError ? err.message : t("transaction.saveFailed")),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (valid) save.mutate();
  }

  return (
    <Shell onClose={onClose} title={editGuid ? t("transaction.editTitle") : t("transaction.newTitle")}>
      <p className="sub">{t("transaction.subtitle")}</p>

      <form className="dialog__grid" onSubmit={submit}>
        <div className="field">
          <label htmlFor="tx-desc">{t("common.description")}</label>
          <input
            id="tx-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t("transaction.descriptionPlaceholder")}
          />
        </div>

        <div className="dialog__row">
          <div className="field">
            <label htmlFor="tx-from">{t("transaction.from")}</label>
            <select id="tx-from" value={fromGuid} onChange={(e) => setFromGuid(e.target.value)}>
              {accounts.map((a) => (
                <option key={a.guid} value={a.guid}>{a.name}</option>
              ))}
            </select>
          </div>
          <div className="field">
            <label htmlFor="tx-to">{t("transaction.to")}</label>
            <select id="tx-to" value={toGuid} onChange={(e) => setToGuid(e.target.value)}>
              {accounts.map((a) => (
                <option key={a.guid} value={a.guid}>{a.name}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="field">
          <label htmlFor="tx-amount">{t("common.amount")}</label>
          <input
            id="tx-amount"
            className="mono"
            inputMode="decimal"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder={t("transaction.amountPlaceholder")}
          />
        </div>

        <div>
          <span className={`balance-pill ${valid ? "balance-pill--ok" : "balance-pill--off"}`}>
            {valid ? (
              <>{t("transaction.balanced")} · {formatMoney({ num: 0, denom: parsed!.denom })}</>
            ) : toGuid === fromGuid && toGuid !== "" ? (
              t("transaction.mustDiffer")
            ) : (
              t("transaction.enterAmount")
            )}
          </span>
        </div>

        <div className="error-note">{error}</div>

        <div className="dialog__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>
            {t("transaction.cancel")}
          </button>
          <button type="submit" className="btn btn--primary" disabled={!valid || save.isPending}>
            {save.isPending ? <span className="spinner" /> : editGuid ? t("transaction.saveChanges") : t("transaction.postTransaction")}
          </button>
        </div>
      </form>
    </Shell>
  );
}

function Shell({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="dialog-backdrop" onMouseDown={onClose}>
      <div className="dialog" onMouseDown={(e) => e.stopPropagation()}>
        <h2>{title}</h2>
        {children}
      </div>
    </div>
  );
}

function DialogClose({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="dialog__actions">
      <button type="button" className="btn btn--ghost" onClick={onClose}>{t("common.close")}</button>
    </div>
  );
}
