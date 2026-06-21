import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "./lib/api";

// SetupLedger is the first-run empty state. One click scaffolds a complete demo
// book — a USD commodity, a small chart of accounts, and an opening-balance
// transaction — so the register is immediately populated. It exercises the full
// create -> post -> register slice from the browser.
export function SetupLedger() {
  const { t } = useTranslation();
  const qc = useQueryClient();

  const scaffold = useMutation({
    mutationFn: async () => {
      const book = await api.createBook();
      const usd = await api.createCommodity("USD", 100, "US Dollar");

      const group = (name: string, type: string) =>
        api.createAccount({
          bookGuid: book.guid,
          name,
          type,
          commodityGuid: usd.guid,
          placeholder: true,
        });
      const leaf = (name: string, type: string, parentGuid: string) =>
        api.createAccount({ bookGuid: book.guid, name, type, commodityGuid: usd.guid, parentGuid });

      const assets = await group("Assets", "ASSET");
      const income = await group("Income", "INCOME");
      const expenses = await group("Expenses", "EXPENSE");
      const equity = await group("Equity", "EQUITY");

      const checking = await leaf("Checking", "BANK", assets.guid);
      await leaf("Salary", "INCOME", income.guid);
      await leaf("Groceries", "EXPENSE", expenses.guid);
      const opening = await leaf("Opening Balances", "EQUITY", equity.guid);

      await api.postTransaction({
        currencyGuid: usd.guid,
        description: "Opening balance",
        splits: [
          { accountGuid: checking.guid, value: { num: 100000, denom: 100 }, quantity: { num: 100000, denom: 100 } },
          { accountGuid: opening.guid, value: { num: -100000, denom: 100 }, quantity: { num: -100000, denom: 100 } },
        ],
      });
      return book;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["books"] }),
  });

  return (
    <div className="setup">
      <div className="setup__card">
        <div className="seal">§</div>
        <h1>{t("setup.title")}</h1>
        <p>{t("setup.description")}</p>
        <button className="btn btn--accent" onClick={() => scaffold.mutate()} disabled={scaffold.isPending}>
          {scaffold.isPending ? <span className="spinner" /> : t("setup.createDemo")}
        </button>
        {scaffold.error && (
          <p className="error-note" style={{ marginTop: "1rem" }}>
            {scaffold.error instanceof ApiError ? scaffold.error.message : t("setup.setupFailed")}
          </p>
        )}
      </div>
    </div>
  );
}
