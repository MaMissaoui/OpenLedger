import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import type { Book } from "../lib/types";
import { formatMoney } from "../lib/money";

interface Props {
  book: Book;
  onTrade: () => void;
}

// PortfolioView lists the book's security holdings with their shares, cost
// basis, and a market valuation from the latest price quote. Positions without
// a quote show a dash for the priced columns.
export function PortfolioView({ book, onTrade }: Props) {
  const q = useQuery({
    queryKey: ["portfolio", book.guid],
    queryFn: () => api.getPortfolio(book.guid),
  });

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          <div className="eyebrow">Investments</div>
          <h1>Portfolio</h1>
        </div>
        <button className="btn btn--accent btn--sm" onClick={onTrade}>
          Trade
        </button>
      </header>

      {q.isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : q.isError ? (
        <div className="empty">Could not load the portfolio.</div>
      ) : !q.data || q.data.holdings.length === 0 ? (
        <div className="empty">
          No security holdings yet. Create a STOCK account and use Trade to buy into it.
        </div>
      ) : (
        <table className="ledger-table report-section">
          <thead>
            <tr>
              <th>Security</th>
              <th className="num">Shares</th>
              <th className="num">Last price</th>
              <th className="num">Market value</th>
              <th className="num">Cost basis</th>
              <th className="num">Unrealized gain</th>
            </tr>
          </thead>
          <tbody>
            {q.data.holdings.map((h) => (
              <tr key={h.account.guid}>
                <td className="desc">{h.account.name}</td>
                <td className="num">{formatMoney(h.shares)}</td>
                <td className="num">{h.hasPrice && h.price ? formatMoney(h.price) : "—"}</td>
                <td className="num">
                  {h.hasPrice && h.marketValue ? formatMoney(h.marketValue) : "—"}
                </td>
                <td className="num">{formatMoney(h.costBasis)}</td>
                <td
                  className={`num${
                    h.hasPrice && h.unrealizedGain && h.unrealizedGain.num < 0 ? " neg" : ""
                  }`}
                >
                  {h.hasPrice && h.unrealizedGain ? formatMoney(h.unrealizedGain) : "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <p className="report__note">
        Market values use each commodity's latest price quote and are not converted across
        currencies.
      </p>
    </section>
  );
}
