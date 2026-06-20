import { useEffect, useState } from "react";

import { api } from "../lib/api";
import { parseAmount, toFloat } from "../lib/money";
import type { Commodity, Price } from "../lib/types";

// Price rates need more precision than money (two places), so quotes are stored
// and parsed at six decimals — enough for FX rates and share prices.
const PRICE_DENOM = 1_000_000;

const NAMESPACES = ["CURRENCY", "STOCK", "MUTUAL", "FUND"] as const;

function formatRate(p: Price): string {
  return toFloat(p.value).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  });
}

// ── New commodity dialog ──────────────────────────────────────────────────────

function NewCommodityDialog({
  onSaved,
  onClose,
}: {
  onSaved: (c: Commodity) => void;
  onClose: () => void;
}) {
  const [namespace, setNamespace] = useState<string>("CURRENCY");
  const [mnemonic, setMnemonic] = useState("");
  const [fullname, setFullname] = useState("");
  // Currencies use 100 (cents); securities are commonly tracked to 10000.
  const [fraction, setFraction] = useState("100");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSave() {
    setError(null);
    if (!mnemonic.trim()) { setError("Symbol is required."); return; }
    const frac = parseInt(fraction, 10);
    if (!Number.isFinite(frac) || frac < 1) { setError("Fraction must be a positive integer."); return; }
    setSaving(true);
    try {
      const c = await api.createCommodity(mnemonic.trim(), frac, fullname.trim() || undefined, namespace);
      onSaved(c);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>New Commodity</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
          <label className="field">
            <span className="field__label">Type</span>
            <select
              value={namespace}
              onChange={(e) => {
                const ns = e.target.value;
                setNamespace(ns);
                setFraction(ns === "CURRENCY" ? "100" : "10000");
              }}
            >
              {NAMESPACES.map((ns) => (
                <option key={ns} value={ns}>{ns}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span className="field__label">Symbol</span>
            <input
              value={mnemonic}
              onChange={(e) => setMnemonic(e.target.value)}
              placeholder={namespace === "CURRENCY" ? "EUR" : "AAPL"}
              autoFocus
            />
          </label>
          <label className="field">
            <span className="field__label">Full name</span>
            <input
              value={fullname}
              onChange={(e) => setFullname(e.target.value)}
              placeholder={namespace === "CURRENCY" ? "Euro" : "Apple Inc."}
            />
          </label>
          <label className="field">
            <span className="field__label">Fraction (smallest unit)</span>
            <input
              value={fraction}
              onChange={(e) => setFraction(e.target.value)}
              inputMode="numeric"
              style={{ width: "8rem" }}
            />
          </label>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── New price dialog ──────────────────────────────────────────────────────────

function NewPriceDialog({
  commodity,
  currencies,
  onSaved,
  onClose,
}: {
  commodity: Commodity;
  currencies: Commodity[];
  onSaved: () => void;
  onClose: () => void;
}) {
  // Default the quote currency to the first one that isn't the commodity itself.
  const defaultCurrency = currencies.find((c) => c.guid !== commodity.guid)?.guid ?? "";
  const [currencyGuid, setCurrencyGuid] = useState(defaultCurrency);
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10));
  const [rate, setRate] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSave() {
    setError(null);
    if (!currencyGuid) { setError("Pick a quote currency."); return; }
    const value = parseAmount(rate, PRICE_DENOM);
    if (value === null || value.num <= 0) { setError("Enter a positive rate."); return; }
    setSaving(true);
    try {
      await api.createPrice({
        commodityGuid: commodity.guid,
        currencyGuid,
        value,
        date: date ? new Date(date).toISOString() : undefined,
        source: "user",
      });
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>New Price for {commodity.mnemonic}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
          <label className="field">
            <span className="field__label">Date</span>
            <input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </label>
          <label className="field">
            <span className="field__label">Rate (1 {commodity.mnemonic} =)</span>
            <input
              value={rate}
              onChange={(e) => setRate(e.target.value)}
              placeholder="1.085"
              inputMode="decimal"
              autoFocus
            />
          </label>
          <label className="field">
            <span className="field__label">Quote currency</span>
            <select value={currencyGuid} onChange={(e) => setCurrencyGuid(e.target.value)}>
              <option value="">— currency —</option>
              {currencies.map((c) => (
                <option key={c.guid} value={c.guid}>{c.mnemonic}</option>
              ))}
            </select>
          </label>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Fetch online quote dialog ─────────────────────────────────────────────────

function FetchQuoteDialog({
  commodity,
  currencies,
  onSaved,
  onClose,
}: {
  commodity: Commodity;
  currencies: Commodity[];
  onSaved: () => void;
  onClose: () => void;
}) {
  // Can't quote a currency against itself.
  const others = currencies.filter((c) => c.guid !== commodity.guid);
  const [currencyGuid, setCurrencyGuid] = useState(others[0]?.guid ?? "");
  const [fetching, setFetching] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleFetch() {
    setError(null);
    if (!currencyGuid) { setError("Pick a quote currency."); return; }
    setFetching(true);
    try {
      await api.fetchPrice(commodity.guid, currencyGuid);
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Fetch failed");
    } finally {
      setFetching(false);
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h2>Fetch Online Quote — {commodity.mnemonic}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
          <p style={{ margin: 0, fontSize: "0.85rem", color: "var(--ink-soft)" }}>
            Pulls today's reference rate from the server's quote provider and saves it as a price.
          </p>
          <label className="field">
            <span className="field__label">Quote currency (1 {commodity.mnemonic} =)</span>
            <select value={currencyGuid} onChange={(e) => setCurrencyGuid(e.target.value)}>
              <option value="">— currency —</option>
              {others.map((c) => (
                <option key={c.guid} value={c.guid}>{c.mnemonic}</option>
              ))}
            </select>
          </label>
        </div>
        <div className="dialog__footer">
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handleFetch} disabled={fetching || !currencyGuid}>
            {fetching ? "Fetching…" : "Fetch"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Prices panel ──────────────────────────────────────────────────────────────

function PricesPanel({
  commodity,
  currencyMap,
  reloadKey,
}: {
  commodity: Commodity;
  currencyMap: Record<string, string>;
  reloadKey: number;
}) {
  const [prices, setPrices] = useState<Price[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setPrices(null);
    setError(null);
    api.listPrices(commodity.guid)
      .then(setPrices)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load prices"));
  }, [commodity.guid, reloadKey]);

  if (error) return <p className="error" style={{ margin: "0.5rem 0" }}>{error}</p>;
  if (prices === null) return <div className="empty"><span className="spinner" /></div>;
  if (prices.length === 0) {
    return <div className="empty">No quotes recorded for {commodity.mnemonic} yet.</div>;
  }

  return (
    <table className="ledger-table">
      <thead>
        <tr>
          <th>Date</th>
          <th style={{ textAlign: "right" }}>Rate</th>
          <th>Currency</th>
          <th>Source</th>
        </tr>
      </thead>
      <tbody>
        {prices.map((p) => (
          <tr key={p.guid}>
            <td>{p.date.slice(0, 10)}</td>
            <td style={{ textAlign: "right" }} className="mono">{formatRate(p)}</td>
            <td>{currencyMap[p.currencyGuid] ?? "?"}</td>
            <td style={{ color: "var(--ink-soft)" }}>{p.source || "—"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// ── Commodities & Prices view ─────────────────────────────────────────────────

export default function CommoditiesView() {
  const [commodities, setCommodities] = useState<Commodity[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedGuid, setSelectedGuid] = useState<string | null>(null);
  const [showNewCommodity, setShowNewCommodity] = useState(false);
  const [showNewPrice, setShowNewPrice] = useState(false);
  const [showFetch, setShowFetch] = useState(false);
  // Bumped to force the prices panel to reload after adding a quote.
  const [pricesReload, setPricesReload] = useState(0);

  function reload() {
    setError(null);
    api.listCommodities()
      .then(setCommodities)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load commodities"));
  }
  useEffect(reload, []);

  const list = commodities ?? [];
  const currencies = list.filter((c) => c.namespace === "CURRENCY");
  const currencyMap = Object.fromEntries(list.map((c) => [c.guid, c.mnemonic]));
  const selected = list.find((c) => c.guid === selectedGuid) ?? null;

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">Reference Data</div>
          <h1>Commodities &amp; Prices</h1>
        </div>
        <div className="register__actions">
          <button className="btn btn--primary btn--sm" onClick={() => setShowNewCommodity(true)}>
            + New Commodity
          </button>
        </div>
      </header>

      {error && (
        <div style={{ padding: "0.75rem 1.5rem" }}>
          <p className="error" style={{ margin: 0 }}>{error}</p>
        </div>
      )}

      <div className="commodities">
        <div className="commodities__list">
          {commodities === null ? (
            <div className="empty"><span className="spinner" /></div>
          ) : list.length === 0 ? (
            <div className="empty">No commodities yet. Add a currency or security to begin.</div>
          ) : (
            <table className="ledger-table">
              <thead>
                <tr><th>Type</th><th>Symbol</th><th style={{ textAlign: "right" }}>Fraction</th></tr>
              </thead>
              <tbody>
                {list.map((c) => (
                  <tr
                    key={c.guid}
                    onClick={() => setSelectedGuid(c.guid)}
                    style={{
                      cursor: "pointer",
                      background: c.guid === selectedGuid ? "var(--surface-container-low)" : undefined,
                    }}
                  >
                    <td style={{ color: "var(--ink-soft)", fontSize: "0.8rem" }}>{c.namespace}</td>
                    <td className="mono">{c.mnemonic}</td>
                    <td style={{ textAlign: "right" }}>{c.fraction}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="commodities__detail">
          {selected === null ? (
            <div className="empty">Select a commodity to see its price history.</div>
          ) : (
            <>
              <div className="commodities__detail-head">
                <h2 style={{ margin: 0 }}>
                  {selected.mnemonic}{" "}
                  <span style={{ color: "var(--ink-soft)", fontWeight: 400, fontSize: "0.85rem" }}>
                    {selected.namespace}
                  </span>
                </h2>
                <div style={{ display: "flex", gap: "0.5rem" }}>
                  {selected.namespace === "CURRENCY" && (
                    <button
                      className="btn btn--ghost btn--sm"
                      onClick={() => setShowFetch(true)}
                      disabled={currencies.length < 2}
                      title={currencies.length < 2 ? "Add another currency to quote against" : "Fetch today's rate online"}
                    >
                      Fetch online
                    </button>
                  )}
                  <button
                    className="btn btn--primary btn--sm"
                    onClick={() => setShowNewPrice(true)}
                    disabled={currencies.length === 0}
                    title={currencies.length === 0 ? "Add a currency first" : undefined}
                  >
                    + Add Price
                  </button>
                </div>
              </div>
              <PricesPanel commodity={selected} currencyMap={currencyMap} reloadKey={pricesReload} />
            </>
          )}
        </div>
      </div>

      {showNewCommodity && (
        <NewCommodityDialog
          onClose={() => setShowNewCommodity(false)}
          onSaved={(c) => {
            setShowNewCommodity(false);
            setSelectedGuid(c.guid);
            reload();
          }}
        />
      )}
      {showNewPrice && selected && (
        <NewPriceDialog
          commodity={selected}
          currencies={currencies}
          onClose={() => setShowNewPrice(false)}
          onSaved={() => {
            setShowNewPrice(false);
            setPricesReload((n) => n + 1);
          }}
        />
      )}
      {showFetch && selected && (
        <FetchQuoteDialog
          commodity={selected}
          currencies={currencies}
          onClose={() => setShowFetch(false)}
          onSaved={() => {
            setShowFetch(false);
            setPricesReload((n) => n + 1);
          }}
        />
      )}
    </section>
  );
}
