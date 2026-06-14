import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "../lib/api";
import type { Account } from "../lib/types";
import { parseAmount, toFloat } from "../lib/money";

interface Props {
  accounts: Account[]; // postable accounts
  onClose: () => void;
}

// TradeSecurityDialog posts a security buy or sell as a two-split transaction
// whose security leg carries shares as its quantity and cash as its value (they
// differ, unlike a plain transfer). The trading-accounts engine on the server
// balances the per-commodity quantities, so the book stays balanced. Cost-basis
// / capital-gains accounting on a sell arrives with lots (PR 3b); for now a sell
// is a straight reduction of shares against cash proceeds.
export function TradeSecurityDialog({ accounts, onClose }: Props) {
  const qc = useQueryClient();
  const securities = accounts.filter((a) => a.type === "STOCK" || a.type === "MUTUAL");
  const cashAccounts = accounts.filter(
    (a) => a.type === "BANK" || a.type === "CASH" || a.type === "ASSET",
  );
  const commodities = useQuery({ queryKey: ["commodities"], queryFn: api.listCommodities });
  const fractionOf = (commodityGuid: string) =>
    commodities.data?.find((c) => c.guid === commodityGuid)?.fraction ?? 100;

  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [securityGuid, setSecurityGuid] = useState(securities[0]?.guid ?? "");
  const [cashGuid, setCashGuid] = useState(cashAccounts[0]?.guid ?? "");
  const [shares, setShares] = useState("");
  const [cash, setCash] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState("");

  const security = securities.find((a) => a.guid === securityGuid) ?? null;
  const cashAcct = cashAccounts.find((a) => a.guid === cashGuid) ?? null;
  const shareFraction = security ? fractionOf(security.commodityGuid) : 1;
  const cashFraction = cashAcct ? fractionOf(cashAcct.commodityGuid) : 100;

  const parsedShares = parseAmount(shares, shareFraction);
  const parsedCash = parseAmount(cash, cashFraction);
  const valid =
    security !== null &&
    cashAcct !== null &&
    parsedShares !== null &&
    parsedShares.num > 0 &&
    parsedCash !== null &&
    parsedCash.num > 0;

  const perShare =
    parsedShares && parsedCash && parsedShares.num > 0
      ? toFloat(parsedCash) / toFloat(parsedShares)
      : null;

  const save = useMutation({
    mutationFn: async () => {
      // The server opens/consumes cost-basis lots and builds the balanced splits;
      // the dialog just sends shares and total cash.
      const input = {
        securityAccountGuid: securityGuid,
        cashAccountGuid: cashGuid,
        shares: parsedShares!,
        cash: parsedCash!,
        description: description.trim(),
      };
      return side === "buy" ? api.buySecurity(input) : api.sellSecurity(input);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["register"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["portfolio"] });
      qc.invalidateQueries({ queryKey: ["capital-gains"] });
      qc.invalidateQueries({ queryKey: ["balance-sheet"] });
      qc.invalidateQueries({ queryKey: ["income-statement"] });
      onClose();
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Could not record the trade"),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (valid) save.mutate();
  }

  return (
    <div className="dialog-backdrop" onMouseDown={onClose}>
      <div className="dialog" onMouseDown={(e) => e.stopPropagation()}>
        <h2>Trade security</h2>
        {securities.length === 0 ? (
          <>
            <p className="sub">
              You have no investment accounts yet. Create a STOCK or MUTUAL account first, then come
              back to buy into it.
            </p>
            <div className="dialog__actions">
              <button type="button" className="btn btn--ghost" onClick={onClose}>
                Close
              </button>
            </div>
          </>
        ) : (
          <form className="dialog__grid" onSubmit={submit}>
            <div className="report__tabs">
              <button
                type="button"
                className={`btn btn--sm ${side === "buy" ? "btn--accent" : "btn--ghost"}`}
                onClick={() => setSide("buy")}
              >
                Buy
              </button>
              <button
                type="button"
                className={`btn btn--sm ${side === "sell" ? "btn--accent" : "btn--ghost"}`}
                onClick={() => setSide("sell")}
              >
                Sell
              </button>
            </div>

            <div className="dialog__row">
              <div className="field">
                <label htmlFor="trade-security">Security</label>
                <select
                  id="trade-security"
                  value={securityGuid}
                  onChange={(e) => setSecurityGuid(e.target.value)}
                >
                  {securities.map((a) => (
                    <option key={a.guid} value={a.guid}>
                      {a.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label htmlFor="trade-cash">{side === "buy" ? "Pay from" : "Deposit to"}</label>
                <select id="trade-cash" value={cashGuid} onChange={(e) => setCashGuid(e.target.value)}>
                  {cashAccounts.map((a) => (
                    <option key={a.guid} value={a.guid}>
                      {a.name}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="dialog__row">
              <div className="field">
                <label htmlFor="trade-shares">Shares</label>
                <input
                  id="trade-shares"
                  className="mono"
                  inputMode="decimal"
                  value={shares}
                  onChange={(e) => setShares(e.target.value)}
                  placeholder="0"
                />
              </div>
              <div className="field">
                <label htmlFor="trade-cash-amount">
                  {side === "buy" ? "Total cost" : "Proceeds"}
                </label>
                <input
                  id="trade-cash-amount"
                  className="mono"
                  inputMode="decimal"
                  value={cash}
                  onChange={(e) => setCash(e.target.value)}
                  placeholder="0.00"
                />
              </div>
            </div>

            <div className="field">
              <label htmlFor="trade-desc">Description</label>
              <input
                id="trade-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={side === "buy" ? "e.g. Buy AAPL" : "e.g. Sell AAPL"}
              />
            </div>

            <div>
              <span className={`balance-pill ${valid ? "balance-pill--ok" : "balance-pill--off"}`}>
                {valid && perShare !== null
                  ? `≈ ${perShare.toLocaleString(undefined, { maximumFractionDigits: 4 })} per share`
                  : "Enter shares and amount"}
              </span>
            </div>

            <div className="error-note">{error}</div>

            <div className="dialog__actions">
              <button type="button" className="btn btn--ghost" onClick={onClose}>
                Cancel
              </button>
              <button type="submit" className="btn btn--accent" disabled={!valid || save.isPending}>
                {save.isPending ? <span className="spinner" /> : side === "buy" ? "Buy" : "Sell"}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
