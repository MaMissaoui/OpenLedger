import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
  const { data, isLoading, isError } = useQuery({
    queryKey: ["transaction", editGuid],
    queryFn: () => api.getTransaction(editGuid),
  });

  if (isLoading) {
    return (
      <Shell onClose={onClose} title="Edit transaction">
        <div className="empty"><span className="spinner" /></div>
      </Shell>
    );
  }
  if (isError || !data) {
    return (
      <Shell onClose={onClose} title="Edit transaction">
        <div className="error-note">Could not load this transaction.</div>
        <DialogClose onClose={onClose} />
      </Shell>
    );
  }
  if (data.splits.length !== 2) {
    return (
      <Shell onClose={onClose} title="Edit transaction">
        <p className="sub">
          This transaction has {data.splits.length} splits and can't be edited here. Delete it from the register.
        </p>
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
      setError(err instanceof ApiError ? err.message : "Could not save transaction"),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (valid) save.mutate();
  }

  return (
    <Shell onClose={onClose} title={editGuid ? "Edit transaction" : "New transaction"}>
      <p className="sub">A balanced transfer between two accounts.</p>

      <form className="dialog__grid" onSubmit={submit}>
        <div className="field">
          <label htmlFor="tx-desc">Description</label>
          <input
            id="tx-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="e.g. Weekly groceries"
          />
        </div>

        <div className="dialog__row">
          <div className="field">
            <label htmlFor="tx-from">From (money out)</label>
            <select id="tx-from" value={fromGuid} onChange={(e) => setFromGuid(e.target.value)}>
              {accounts.map((a) => (
                <option key={a.guid} value={a.guid}>{a.name}</option>
              ))}
            </select>
          </div>
          <div className="field">
            <label htmlFor="tx-to">To (money in)</label>
            <select id="tx-to" value={toGuid} onChange={(e) => setToGuid(e.target.value)}>
              {accounts.map((a) => (
                <option key={a.guid} value={a.guid}>{a.name}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="field">
          <label htmlFor="tx-amount">Amount</label>
          <input
            id="tx-amount"
            className="mono"
            inputMode="decimal"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder="0.00"
          />
        </div>

        <div>
          <span className={`balance-pill ${valid ? "balance-pill--ok" : "balance-pill--off"}`}>
            {valid ? (
              <>✓ Balanced · {formatMoney({ num: 0, denom: parsed!.denom })}</>
            ) : toGuid === fromGuid && toGuid !== "" ? (
              "From and To must differ"
            ) : (
              "Enter an amount"
            )}
          </span>
        </div>

        <div className="error-note">{error}</div>

        <div className="dialog__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn btn--primary" disabled={!valid || save.isPending}>
            {save.isPending ? <span className="spinner" /> : editGuid ? "Save changes" : "Post transaction"}
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
  return (
    <div className="dialog__actions">
      <button type="button" className="btn btn--ghost" onClick={onClose}>Close</button>
    </div>
  );
}
