package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
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

// RefreshStatus tracks the last automatic price-refresh run, shared between
// the background goroutine (writer) and the status HTTP handler (reader).
type RefreshStatus struct {
	mu            sync.RWMutex
	Enabled       bool
	IntervalHours int
	LastRunAt     *time.Time
	LastFetched   int
	LastFailed    int
}

// Record updates the status after a completed refresh run.
func (s *RefreshStatus) Record(fetched, failed int) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastRunAt = &now
	s.LastFetched = fetched
	s.LastFailed = failed
}

// Snapshot returns a point-in-time copy for serialisation.
func (s *RefreshStatus) Snapshot() (enabled bool, hours int, lastAt *time.Time, fetched, failed int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Enabled, s.IntervalHours, s.LastRunAt, s.LastFetched, s.LastFailed
}

// handleGetRefreshStatus returns the current auto-refresh configuration and
// the result of the most recent run (or null if it has never run).
func (s *Server) handleGetRefreshStatus(w http.ResponseWriter, r *http.Request) {
	if s.RefreshStatus == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false, "intervalHours": 0,
			"lastRunAt": nil, "lastFetched": 0, "lastFailed": 0,
		})
		return
	}
	enabled, hours, lastAt, fetched, failed := s.RefreshStatus.Snapshot()
	var lastAtStr any
	if lastAt != nil {
		lastAtStr = lastAt.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": enabled, "intervalHours": hours,
		"lastRunAt": lastAtStr, "lastFetched": fetched, "lastFailed": failed,
	})
}

// handleRefreshNow triggers an immediate price refresh and returns the result.
func (s *Server) handleRefreshNow(w http.ResponseWriter, r *http.Request) {
	if s.Quote == nil {
		writeError(w, http.StatusServiceUnavailable, "online price quotes are not configured")
		return
	}
	result, err := s.Quote.RefreshAll(context.Background())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.RefreshStatus != nil {
		s.RefreshStatus.Record(result.Fetched, result.Failed)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fetched": result.Fetched,
		"skipped": result.Skipped,
		"failed":  result.Failed,
	})
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
