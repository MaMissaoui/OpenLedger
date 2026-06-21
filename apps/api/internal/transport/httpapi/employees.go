package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type employeeBodyDTO struct {
	Name         string      `json:"name"`
	Username     string      `json:"username"`
	ID           string      `json:"id"`
	Notes        string      `json:"notes"`
	Active       bool        `json:"active"`
	CurrencyGUID string      `json:"currencyGuid"`
	Addr         addressDTO  `json:"addr"`
	Rate         *numericDTO `json:"rate"`
}

func employeeToResponse(e domain.Employee) map[string]any {
	return map[string]any{
		"guid":         e.GUID,
		"bookGuid":     e.BookGUID,
		"name":         e.Name,
		"username":     e.Username,
		"id":           e.ID,
		"notes":        e.Notes,
		"active":       e.Active,
		"currencyGuid": e.CurrencyGUID,
		"addr": map[string]any{
			"name":  e.Addr.Name,
			"addr1": e.Addr.Addr1,
			"addr2": e.Addr.Addr2,
			"phone": e.Addr.Phone,
			"email": e.Addr.Email,
		},
		"rate":      numericAtScale(e.Rate, 100),
		"createdAt": e.CreatedAt,
	}
}

func (dto employeeBodyDTO) toDomain(guid, bookGUID string) (domain.Employee, error) {
	rate := domain.Zero()
	if dto.Rate != nil {
		r, err := dto.Rate.toDomain()
		if err != nil {
			return domain.Employee{}, err
		}
		rate = r
	}
	return domain.Employee{
		GUID:         guid,
		BookGUID:     bookGUID,
		Name:         dto.Name,
		Username:     dto.Username,
		ID:           dto.ID,
		Notes:        dto.Notes,
		Active:       dto.Active,
		CurrencyGUID: dto.CurrencyGUID,
		Addr:         domain.Address{Name: dto.Addr.Name, Addr1: dto.Addr.Addr1, Addr2: dto.Addr.Addr2, Phone: dto.Addr.Phone, Email: dto.Addr.Email},
		Rate:         rate,
	}, nil
}

func (s *Server) handleListEmployees(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	activeOnly := r.URL.Query().Get("active") == "true"
	list, err := s.Employee.ListEmployees(r.Context(), bookGUID, userID, activeOnly)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	out := make([]map[string]any, len(list))
	for i, e := range list {
		out[i] = employeeToResponse(e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "employees": out})
}

func (s *Server) handleCreateEmployee(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto employeeBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	e, err := dto.toDomain("", bookGUID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rate")
		return
	}
	created, err := s.Employee.CreateEmployee(r.Context(), userID, e)
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusCreated, employeeToResponse(created))
	}
}

func (s *Server) handleGetEmployee(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	e, err := s.Employee.GetEmployee(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrEmployeeNotFound):
		writeError(w, http.StatusNotFound, "employee not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, employeeToResponse(e))
	}
}

func (s *Server) handleUpdateEmployee(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto employeeBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	e, err := dto.toDomain(guid, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rate")
		return
	}
	updated, err := s.Employee.UpdateEmployee(r.Context(), userID, e)
	switch {
	case errors.Is(err, domain.ErrEmployeeNotFound):
		writeError(w, http.StatusNotFound, "employee not found")
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, employeeToResponse(updated))
	}
}

func (s *Server) handleDeleteEmployee(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	err := s.Employee.DeleteEmployee(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrEmployeeNotFound):
		writeError(w, http.StatusNotFound, "employee not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
