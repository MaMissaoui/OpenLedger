package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type budgetAmountDTO struct {
	AccountGUID string     `json:"accountGuid"`
	PeriodNum   int        `json:"periodNum"`
	Value       numericDTO `json:"value"`
}

type newBudgetDTO struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	PeriodType  string            `json:"periodType"`
	NumPeriods  int               `json:"numPeriods"`
	StartDate   string            `json:"startDate"` // YYYY-MM-DD
	Amounts     []budgetAmountDTO `json:"amounts"`
}

func (dto newBudgetDTO) toDomain(bookGUID, existingGUID string) (domain.Budget, error) {
	start, err := parseDate(dto.StartDate)
	if err != nil {
		return domain.Budget{}, err
	}
	amounts := make([]domain.BudgetAmount, len(dto.Amounts))
	for i, a := range dto.Amounts {
		val, err := a.Value.toDomain()
		if err != nil {
			return domain.Budget{}, err
		}
		amounts[i] = domain.BudgetAmount{
			AccountGUID: a.AccountGUID,
			PeriodNum:   a.PeriodNum,
			Value:       val,
		}
	}
	return domain.Budget{
		GUID:        existingGUID,
		BookGUID:    bookGUID,
		Name:        dto.Name,
		Description: dto.Description,
		PeriodType:  domain.BudgetPeriodType(dto.PeriodType),
		NumPeriods:  dto.NumPeriods,
		StartDate:   start,
		Amounts:     amounts,
	}, nil
}

func budgetToResponse(b domain.Budget) map[string]any {
	amounts := make([]map[string]any, len(b.Amounts))
	for i, a := range b.Amounts {
		amounts[i] = map[string]any{
			"accountGuid": a.AccountGUID,
			"periodNum":   a.PeriodNum,
			"value":       numericAtScale(a.Value, 100),
		}
	}
	return map[string]any{
		"guid":        b.GUID,
		"bookGuid":    b.BookGUID,
		"name":        b.Name,
		"description": b.Description,
		"periodType":  string(b.PeriodType),
		"numPeriods":  b.NumPeriods,
		"startDate":   b.StartDate.Format("2006-01-02"),
		"amounts":     amounts,
	}
}

func (s *Server) handleListBudgets(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	budgets, err := s.budget.List(r.Context(), bookGUID)
	if writeStructureError(w, err) {
		return
	}
	out := make([]map[string]any, len(budgets))
	for i, b := range budgets {
		out[i] = budgetToResponse(b)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "budgets": out})
}

func (s *Server) handleCreateBudget(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto newBudgetDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	b, err := dto.toDomain(bookGUID, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget: "+err.Error())
		return
	}
	created, err := s.budget.Create(r.Context(), b)
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid budget")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create budget")
		return
	}
	writeJSON(w, http.StatusCreated, budgetToResponse(created))
}

func (s *Server) handleGetBudget(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	b, err := s.budget.Get(r.Context(), guid)
	if errors.Is(err, domain.ErrBudgetNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load budget")
		return
	}
	bookGUID, err := s.budget.BookGUIDForBudget(r.Context(), guid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not authorize")
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	writeJSON(w, http.StatusOK, budgetToResponse(b))
}

func (s *Server) handleUpdateBudget(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.budget.BookGUIDForBudget(r.Context(), guid)
	if errors.Is(err, domain.ErrBudgetNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not authorize")
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto newBudgetDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	b, err := dto.toDomain(bookGUID, guid)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget: "+err.Error())
		return
	}
	updated, err := s.budget.Update(r.Context(), b)
	switch {
	case errors.Is(err, domain.ErrBudgetNotFound):
		writeError(w, http.StatusNotFound, "budget not found")
		return
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid budget")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update budget")
		return
	}
	writeJSON(w, http.StatusOK, budgetToResponse(updated))
}

func (s *Server) handleDeleteBudget(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.budget.BookGUIDForBudget(r.Context(), guid)
	if errors.Is(err, domain.ErrBudgetNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not authorize")
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	if err := s.budget.Delete(r.Context(), guid); errors.Is(err, domain.ErrBudgetNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete budget")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleBudgetReport(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.budget.BookGUIDForBudget(r.Context(), guid)
	if errors.Is(err, domain.ErrBudgetNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not authorize")
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	asOf, ok := queryTime(w, r, "asOf", time.Now())
	if !ok {
		return
	}
	report, err := s.budget.Report(r.Context(), guid, asOf)
	if errors.Is(err, domain.ErrBudgetNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate report")
		return
	}
	lines := make([]map[string]any, len(report.Lines))
	for i, l := range report.Lines {
		lines[i] = map[string]any{
			"account":  accountDTO(l.Account),
			"budgeted": numericAtScale(l.Budgeted, 100),
			"actual":   numericAtScale(l.Actual, 100),
			"variance": numericAtScale(l.Variance, 100),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"budgetGuid":    guid,
		"periodNum":     report.PeriodNum,
		"periodLabel":   report.PeriodLabel,
		"periodStart":   report.PeriodStart.Format("2006-01-02"),
		"periodEnd":     report.PeriodEnd.Format("2006-01-02"),
		"lines":         lines,
		"totalBudgeted": numericAtScale(report.TotalBudgeted, 100),
		"totalActual":   numericAtScale(report.TotalActual, 100),
		"totalVariance": numericAtScale(report.TotalVariance, 100),
	})
}
