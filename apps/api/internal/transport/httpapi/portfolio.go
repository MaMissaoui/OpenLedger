package httpapi

import (
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// handlePortfolio returns the book's security-holdings report: each STOCK/MUTUAL
// position with its shares, cost basis, and (when a price quote exists) a market
// valuation. Money fields are rendered as reduced exact rationals; shares at the
// commodity fraction.
func (s *Server) handlePortfolio(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	p, err := s.portfolio.Portfolio(r.Context(), bookGUID)
	if writeStructureError(w, err) {
		return
	}

	holdings := make([]map[string]any, 0, len(p.Holdings))
	for _, h := range p.Holdings {
		dto := map[string]any{
			"account":   accountDTO(h.Account),
			"shares":    numericAtScale(h.Shares, h.ShareScale),
			"costBasis": numericAtScale(h.CostBasis, 0),
			"hasPrice":  h.HasPrice,
		}
		if h.HasPrice {
			dto["price"] = numericAtScale(h.Price, 0)
			dto["priceCurrencyGuid"] = h.PriceCurrency
			dto["marketValue"] = numericAtScale(h.MarketValue, 0)
			dto["unrealizedGain"] = numericAtScale(h.UnrealizedGain, 0)
		}
		holdings = append(holdings, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid": bookGUID,
		"holdings": holdings,
	})
}
