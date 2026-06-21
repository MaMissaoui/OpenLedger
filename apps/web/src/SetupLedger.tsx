import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "./lib/api";

// Well-known currencies SetupLedger can create on demand.
const PRESET_CURRENCIES = [
  { mnemonic: "USD", fullname: "US Dollar", fraction: 100 },
  { mnemonic: "EUR", fullname: "Euro", fraction: 100 },
  { mnemonic: "GBP", fullname: "Pound Sterling", fraction: 100 },
  { mnemonic: "TND", fullname: "Tunisian Dinar", fraction: 1000 },
  { mnemonic: "MAD", fullname: "Moroccan Dirham", fraction: 100 },
  { mnemonic: "SAR", fullname: "Saudi Riyal", fraction: 100 },
  { mnemonic: "AED", fullname: "UAE Dirham", fraction: 100 },
  { mnemonic: "JPY", fullname: "Japanese Yen", fraction: 1 },
  { mnemonic: "CHF", fullname: "Swiss Franc", fraction: 100 },
  { mnemonic: "CAD", fullname: "Canadian Dollar", fraction: 100 },
  { mnemonic: "AUD", fullname: "Australian Dollar", fraction: 100 },
];

async function resolveOrCreateCurrency(
  mnemonic: string,
  existingCommodities: { guid: string; mnemonic: string; namespace: string }[]
): Promise<string> {
  const existing = existingCommodities.find(
    (c) => c.mnemonic === mnemonic && c.namespace === "CURRENCY"
  );
  if (existing) return existing.guid;
  const preset = PRESET_CURRENCIES.find((p) => p.mnemonic === mnemonic);
  if (!preset) return "";
  const created = await api.createCommodity(preset.mnemonic, preset.fraction, preset.fullname);
  return created.guid;
}

// SetupLedger is the first-run form. Collects company name + home currency,
// then creates the currency commodity (if needed) and the book.
export function SetupLedger() {
  const { t } = useTranslation();
  const qc = useQueryClient();

  const commoditiesQ = useQuery({ queryKey: ["commodities"], queryFn: api.listCommodities });

  const [name, setName] = useState("");
  const [presetMnemonic, setPresetMnemonic] = useState("USD");
  const [useExisting, setUseExisting] = useState(false);
  const [existingGuid, setExistingGuid] = useState("");

  const create = useMutation({
    mutationFn: async () => {
      const allCommodities = commoditiesQ.data ?? [];
      const currencyGuid = useExisting && existingGuid
        ? existingGuid
        : await resolveOrCreateCurrency(presetMnemonic, allCommodities);
      return api.createBook(name.trim() || "My Company", currencyGuid || undefined);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["books"] });
      qc.invalidateQueries({ queryKey: ["commodities"] });
    },
  });

  const currencyCommodities = (commoditiesQ.data ?? []).filter((c) => c.namespace === "CURRENCY");

  return (
    <div className="setup">
      <div className="setup__card">
        <div className="seal">§</div>
        <h1>{t("setup.title")}</h1>
        <p>{t("setup.description")}</p>

        <div className="setup__form">
          <label className="setup__label">
            {t("setup.companyName", "Company name")}
            <input
              className="setup__input"
              type="text"
              placeholder={t("setup.companyNamePlaceholder", "e.g. My Business")}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </label>

          <label className="setup__label">{t("setup.currency", "Home currency")}</label>

          {currencyCommodities.length > 0 && (
            <label className="setup__radio" style={{ marginBottom: 8 }}>
              <input
                type="checkbox"
                checked={useExisting}
                onChange={(e) => setUseExisting(e.target.checked)}
              />{" "}
              {t("setup.useExistingCurrency", "Use an existing currency")}
            </label>
          )}

          {!useExisting && (
            <select
              className="setup__select"
              value={presetMnemonic}
              onChange={(e) => setPresetMnemonic(e.target.value)}
            >
              {PRESET_CURRENCIES.map((p) => (
                <option key={p.mnemonic} value={p.mnemonic}>
                  {p.mnemonic} — {p.fullname}
                </option>
              ))}
            </select>
          )}

          {useExisting && currencyCommodities.length > 0 && (
            <select
              className="setup__select"
              value={existingGuid}
              onChange={(e) => setExistingGuid(e.target.value)}
            >
              <option value="">— select —</option>
              {currencyCommodities.map((c) => (
                <option key={c.guid} value={c.guid}>
                  {c.mnemonic} — {c.fullname || c.mnemonic}
                </option>
              ))}
            </select>
          )}

          <button
            className="btn btn--accent"
            onClick={() => create.mutate()}
            disabled={create.isPending || (useExisting && !existingGuid)}
          >
            {create.isPending ? <span className="spinner" /> : t("setup.createCompany", "Create Company")}
          </button>
        </div>

        {create.error && (
          <p className="error-note" style={{ marginTop: "1rem" }}>
            {create.error instanceof ApiError ? create.error.message : t("setup.setupFailed")}
          </p>
        )}
      </div>
    </div>
  );
}

// NewCompanyDialog is a compact modal for adding additional books when the user
// already has at least one.
export function NewCompanyDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (guid: string) => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();

  const commoditiesQ = useQuery({ queryKey: ["commodities"], queryFn: api.listCommodities });

  const [name, setName] = useState("");
  const [presetMnemonic, setPresetMnemonic] = useState("USD");
  const [useExisting, setUseExisting] = useState(false);
  const [existingGuid, setExistingGuid] = useState("");

  const create = useMutation({
    mutationFn: async () => {
      const allCommodities = commoditiesQ.data ?? [];
      const currencyGuid = useExisting && existingGuid
        ? existingGuid
        : await resolveOrCreateCurrency(presetMnemonic, allCommodities);
      return api.createBook(name.trim() || "New Company", currencyGuid || undefined);
    },
    onSuccess: (book) => {
      qc.invalidateQueries({ queryKey: ["books"] });
      qc.invalidateQueries({ queryKey: ["commodities"] });
      onCreated(book.guid);
    },
  });

  const currencyCommodities = (commoditiesQ.data ?? []).filter((c) => c.namespace === "CURRENCY");

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div className="dialog" style={{ maxWidth: 420 }} onClick={(e) => e.stopPropagation()}>
        <div className="dialog__header">
          <h3 className="dialog__title">{t("setup.newCompany", "New Company")}</h3>
          <button className="dialog__close" onClick={onClose}>✕</button>
        </div>
        <div className="dialog__body">
          <label className="field">
            <span className="field__label">{t("setup.companyName", "Company name")}</span>
            <input
              className="field__input"
              type="text"
              placeholder={t("setup.companyNamePlaceholder", "e.g. My Business")}
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </label>

          <label className="field">
            <span className="field__label">{t("setup.currency", "Home currency")}</span>
          </label>

          {currencyCommodities.length > 0 && (
            <label className="setup__radio" style={{ marginBottom: 8 }}>
              <input
                type="checkbox"
                checked={useExisting}
                onChange={(e) => setUseExisting(e.target.checked)}
              />{" "}
              {t("setup.useExistingCurrency", "Use an existing currency")}
            </label>
          )}

          {!useExisting && (
            <select
              className="field__input"
              value={presetMnemonic}
              onChange={(e) => setPresetMnemonic(e.target.value)}
            >
              {PRESET_CURRENCIES.map((p) => (
                <option key={p.mnemonic} value={p.mnemonic}>
                  {p.mnemonic} — {p.fullname}
                </option>
              ))}
            </select>
          )}

          {useExisting && currencyCommodities.length > 0 && (
            <select
              className="field__input"
              value={existingGuid}
              onChange={(e) => setExistingGuid(e.target.value)}
            >
              <option value="">— select —</option>
              {currencyCommodities.map((c) => (
                <option key={c.guid} value={c.guid}>
                  {c.mnemonic} — {c.fullname || c.mnemonic}
                </option>
              ))}
            </select>
          )}
        </div>
        <div className="dialog__footer">
          <button className="btn" onClick={onClose}>{t("cancel", "Cancel")}</button>
          <button
            className="btn btn--primary"
            onClick={() => create.mutate()}
            disabled={create.isPending || (useExisting && !existingGuid)}
          >
            {create.isPending ? <span className="spinner" /> : t("setup.createCompany", "Create Company")}
          </button>
        </div>
        {create.error && (
          <p className="error-note" style={{ padding: "0 1rem 1rem" }}>
            {create.error instanceof ApiError ? create.error.message : t("setup.setupFailed")}
          </p>
        )}
      </div>
    </div>
  );
}
