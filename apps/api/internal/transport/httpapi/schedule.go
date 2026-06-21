package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// scheduledSplitDTO is the wire form of one template split.
type scheduledSplitDTO struct {
	GUID        string     `json:"guid,omitempty"`
	AccountGUID string     `json:"accountGuid"`
	Memo        string     `json:"memo"`
	Value       numericDTO `json:"value"`
}

// scheduledTxDTO is the request body for creating/updating a scheduled
// transaction.
type scheduledTxDTO struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Enabled      bool                `json:"enabled"`
	CurrencyGUID string              `json:"currencyGuid"`
	Period       string              `json:"period"`
	Every        int                 `json:"every"`
	StartDate    string              `json:"startDate"` // YYYY-MM-DD
	EndDate      string              `json:"endDate,omitempty"`
	Splits       []scheduledSplitDTO `json:"splits"`
}

func (dto scheduledTxDTO) toDomain(bookGUID, existingGUID string) (domain.ScheduledTransaction, error) {
	start, err := parseDate(dto.StartDate)
	if err != nil {
		return domain.ScheduledTransaction{}, err
	}
	var end time.Time
	if dto.EndDate != "" {
		end, err = parseDate(dto.EndDate)
		if err != nil {
			return domain.ScheduledTransaction{}, err
		}
	}
	splits := make([]domain.ScheduledSplit, len(dto.Splits))
	for i, sp := range dto.Splits {
		val, err := sp.Value.toDomain()
		if err != nil {
			return domain.ScheduledTransaction{}, err
		}
		splits[i] = domain.ScheduledSplit{
			GUID:        sp.GUID,
			AccountGUID: sp.AccountGUID,
			Memo:        sp.Memo,
			Value:       val,
		}
	}
	return domain.ScheduledTransaction{
		GUID:         existingGUID,
		BookGUID:     bookGUID,
		Name:         dto.Name,
		Description:  dto.Description,
		Enabled:      dto.Enabled,
		CurrencyGUID: dto.CurrencyGUID,
		Period:       domain.RecurrencePeriod(dto.Period),
		Every:        dto.Every,
		StartDate:    start,
		EndDate:      end,
		Splits:       splits,
	}, nil
}

func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.UTC)
}

func scheduleToResponse(s domain.ScheduledTransaction) map[string]any {
	splits := make([]map[string]any, len(s.Splits))
	for i, sp := range s.Splits {
		splits[i] = map[string]any{
			"guid":        sp.GUID,
			"accountGuid": sp.AccountGUID,
			"memo":        sp.Memo,
			"value":       numericAtScale(sp.Value, 100),
		}
	}
	resp := map[string]any{
		"guid":         s.GUID,
		"bookGuid":     s.BookGUID,
		"name":         s.Name,
		"description":  s.Description,
		"enabled":      s.Enabled,
		"currencyGuid": s.CurrencyGUID,
		"period":       string(s.Period),
		"every":        s.Every,
		"startDate":    s.StartDate.Format("2006-01-02"),
		"splits":       splits,
	}
	if !s.EndDate.IsZero() {
		resp["endDate"] = s.EndDate.Format("2006-01-02")
	}
	next := s.NextDueDate()
	if !next.IsZero() {
		resp["nextDueDate"] = next.Format("2006-01-02")
	}
	if !s.LastPostedDate.IsZero() {
		resp["lastPostedDate"] = s.LastPostedDate.Format("2006-01-02")
	}
	return resp
}

func (s *Server) handleListScheduledTransactions(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	scheds, err := s.Schedule.List(r.Context(), bookGUID)
	if writeStructureError(w, err) {
		return
	}
	out := make([]map[string]any, len(scheds))
	for i, sc := range scheds {
		out[i] = scheduleToResponse(sc)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "scheduledTransactions": out})
}

func (s *Server) handleCreateScheduledTransaction(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto scheduledTxDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sched, err := dto.toDomain(bookGUID, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule: "+err.Error())
		return
	}
	created, err := s.Schedule.Create(r.Context(), sched, actorFromContext(r.Context()))
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid scheduled transaction")
		return
	case errors.Is(err, domain.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, "template splits must balance to zero")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create scheduled transaction")
		return
	}
	writeJSON(w, http.StatusCreated, scheduleToResponse(created))
}

func (s *Server) handleGetScheduledTransaction(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	sched, err := s.Schedule.Get(r.Context(), guid)
	if errors.Is(err, app.ErrScheduleNotFound) {
		writeError(w, http.StatusNotFound, "scheduled transaction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load scheduled transaction")
		return
	}
	bookGUID, err := s.Schedule.BookGUIDForSchedule(r.Context(), guid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not authorize")
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	writeJSON(w, http.StatusOK, scheduleToResponse(sched))
}

func (s *Server) handleUpdateScheduledTransaction(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.Schedule.BookGUIDForSchedule(r.Context(), guid)
	if errors.Is(err, app.ErrScheduleNotFound) {
		writeError(w, http.StatusNotFound, "scheduled transaction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not authorize")
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto scheduledTxDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sched, err := dto.toDomain(bookGUID, guid)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule: "+err.Error())
		return
	}
	updated, err := s.Schedule.Update(r.Context(), sched)
	switch {
	case errors.Is(err, app.ErrScheduleNotFound):
		writeError(w, http.StatusNotFound, "scheduled transaction not found")
		return
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid scheduled transaction")
		return
	case errors.Is(err, domain.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, "template splits must balance to zero")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update scheduled transaction")
		return
	}
	writeJSON(w, http.StatusOK, scheduleToResponse(updated))
}

func (s *Server) handleDeleteScheduledTransaction(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.Schedule.BookGUIDForSchedule(r.Context(), guid)
	if errors.Is(err, app.ErrScheduleNotFound) {
		writeError(w, http.StatusNotFound, "scheduled transaction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not authorize")
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	if err := s.Schedule.Delete(r.Context(), guid); errors.Is(err, app.ErrScheduleNotFound) {
		writeError(w, http.StatusNotFound, "scheduled transaction not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete scheduled transaction")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePostDueSchedules(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	asOf, ok := queryTime(w, r, "asOf", time.Now())
	if !ok {
		return
	}
	posted, err := s.Schedule.PostDue(r.Context(), bookGUID, asOf, actorFromContext(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not post due scheduled transactions: "+err.Error())
		return
	}
	lines := make([]map[string]any, len(posted))
	for i, p := range posted {
		lines[i] = map[string]any{
			"scheduleGuid": p.ScheduleGUID,
			"name":         p.Name,
			"postDate":     p.PostDate.Format("2006-01-02"),
			"txGuid":       p.TxGUID,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid": bookGUID,
		"posted":   lines,
	})
}
