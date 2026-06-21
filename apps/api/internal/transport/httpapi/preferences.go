package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

type preferencesDTO struct {
	DefaultCommodityGUID *string `json:"defaultCommodityGuid"`
	// FiscalYearStart is the month (1–12) the fiscal year begins; 0 in the
	// response means "not set / calendar year (January)".
	FiscalYearStart int `json:"fiscalYearStart"`
}

func prefsToDTO(p app.BookPreferences) preferencesDTO {
	dto := preferencesDTO{FiscalYearStart: p.FiscalYearStart}
	if p.DefaultCommodityGUID != "" {
		dto.DefaultCommodityGUID = &p.DefaultCommodityGUID
	}
	return dto
}

func (s *Server) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	prefs, err := s.Preferences.GetPreferences(r.Context(), userID, bookGUID)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, prefsToDTO(prefs))
}

func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto preferencesDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	prefs := app.BookPreferences{FiscalYearStart: dto.FiscalYearStart}
	if dto.DefaultCommodityGUID != nil {
		prefs.DefaultCommodityGUID = *dto.DefaultCommodityGUID
	}
	if err := s.Preferences.UpdatePreferences(r.Context(), userID, bookGUID, prefs); err != nil {
		writeAuthzError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
