import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "../lib/api";
import type { Account } from "../lib/types";
import { formatMoney, negate, parseAmount } from "../lib/money";

interface Props {
  accounts: Account[]; // postable accounts
  defaultToGuid: string | null;
  onClose: () => void;
}

// NewTransactionDialog posts a balanced two-split transfer: money leaves the
// "from" account and arrives in the "to" account. The two splits sum to zero by
// construction, which the server re-checks before persisting.
export function NewTransactionDialog({ accounts, defaultToGuid, onClose }: Props) {
  const qc = useQueryClient();
  const [description, setDescription] = useState("");
  const [amount, setAmount] = useState("");
  const [toGuid, setToGuid] = useState(defaultToGuid ?? accounts[0]?.guid ?? "");
  const [fromGuid, setFromGuid] = useState(
    accounts.find((a) => a.guid !== defaultToGuid)?.guid ?? "",
  );
  const [error, setError] = useState("");

  const parsed = parseAmount(amount);
  const valid =
    parsed !== null && parsed.num > 0 && toGuid !== "" && fromGuid !== "" && toGuid !== fromGuid;

  const post = useMutation({
    mutationFn: async () => {
      const to = accounts.find((a) => a.guid === toGuid)!;
      const value = parsed!;
      return api.postTransaction({
        currencyGuid: to.commodityGuid,
        description: description.trim() || "Transfer",
        splits: [
          { accountGuid: toGuid, value, quantity: value },
          { accountGuid: fromGuid, value: negate(value), quantity: negate(value) },
        ],
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["register"] });
      onClose();
    },
    onError: (err) =>
      setError(err instanceof ApiError ? err.message : "Could not post transaction"),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (valid) post.mutate();
  }

  return (
    <div className="dialog-backdrop" onMouseDown={onClose}>
      <div className="dialog" onMouseDown={(e) => e.stopPropagation()}>
        <h2>New transaction</h2>
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
                  <option key={a.guid} value={a.guid}>
                    {a.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label htmlFor="tx-to">To (money in)</label>
              <select id="tx-to" value={toGuid} onChange={(e) => setToGuid(e.target.value)}>
                {accounts.map((a) => (
                  <option key={a.guid} value={a.guid}>
                    {a.name}
                  </option>
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
            <button type="submit" className="btn btn--accent" disabled={!valid || post.isPending}>
              {post.isPending ? <span className="spinner" /> : "Post transaction"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
