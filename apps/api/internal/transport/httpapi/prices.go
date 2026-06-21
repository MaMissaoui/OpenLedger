package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type newPriceDTO struct {
	CommodityGUID string     `json:"commodityGuid"`
	CurrencyGUID  string     `json:"currencyGuid"`
	Date          *time.Time `json:"date"`
	Source        string     `json:"source"`
	Type          string     `json:"type"`
	Value         numericDTO `json:"value"`
}

// handleCreatePrice records a quote (one unit of commodity in currency). Prices
// are shared reference data, so there is no book authorization.
func (s *Server) handleCreatePrice(w http.ResponseWriter, r *http.Request) {
	var dto newPriceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	value, err := dto.Value.toDomain()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	price := domain.Price{
		CommodityGUID: dto.CommodityGUID,
		CurrencyGUID:  dto.CurrencyGUID,
		Source:        dto.Source,
		Type:          dto.Type,
		Value:         value,
	}
	if dto.Date != nil {
		price.Date = *dto.Date
	}

	created, err := s.Price.CreatePrice(r.Context(), price)
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, priceDTO(created))
}

// handleListPrices returns a commodity's quotes, most recent first. The
// commodity query parameter is required.
func (s *Server) handleListPrices(w http.ResponseWriter, r *http.Request) {
	commodity := r.URL.Query().Get("commodity")
	prices, err := s.Price.ListPrices(r.Context(), commodity)
	if writeStructureError(w, err) {
		return
	}
	out := make([]map[string]any, 0, len(prices))
	for _, p := range prices {
		out = append(out, priceDTO(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"prices": out})
}

type fetchPriceDTO struct {
	CommodityGUID string `json:"commodityGuid"`
	CurrencyGUID  string `json:"currencyGuid"`
}

// handleFetchPrice fetches a live exchange rate from the configured quote
// provider (one unit of commodity in currency) and records it as a price.
// Prices are shared reference data, so there is no book authorization. Upstream
// provider failures map to 502; an unknown commodity to 404.
func (s *Server) handleFetchPrice(w http.ResponseWriter, r *http.Request) {
	if s.Quote == nil {
		writeError(w, http.StatusServiceUnavailable, "online price quotes are not configured")
		return
	}
	var dto fetchPriceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	price, err := s.Quote.FetchAndStore(r.Context(), dto.CommodityGUID, dto.CurrencyGUID)
	if errors.Is(err, app.ErrQuoteUnavailable) {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, priceDTO(price))
}

func priceDTO(p domain.Price) map[string]any {
	return map[string]any{
		"guid":          p.GUID,
		"commodityGuid": p.CommodityGUID,
		"currencyGuid":  p.CurrencyGUID,
		"date":          p.Date,
		"source":        p.Source,
		"type":          p.Type,
		"value":         numericAtScale(p.Value, 0), // reduced exact ratio
	}
}
