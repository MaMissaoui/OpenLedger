package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type billTermDTO struct {
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Type         string      `json:"type"` // "days" or "proximo"
	DueDays      int         `json:"dueDays"`
	DiscountDays int         `json:"discountDays"`
	Discount     *numericDTO `json:"discount"` // omitted/null = no discount
	Cutoff       int         `json:"cutoff"`
}

func (dto billTermDTO) toDomain(bookGUID, existingGUID string) (domain.BillTerm, error) {
	discount := domain.Zero()
	if dto.Discount != nil {
		d, err := dto.Discount.toDomain()
		if err != nil {
			return domain.BillTerm{}, err
		}
		discount = d
	}
	return domain.BillTerm{
		GUID:         existingGUID,
		BookGUID:     bookGUID,
		Name:         dto.Name,
		Description:  dto.Description,
		Type:         domain.BillTermType(dto.Type),
		DueDays:      dto.DueDays,
		DiscountDays: dto.DiscountDays,
		Discount:     discount,
		Cutoff:       dto.Cutoff,
	}, nil
}

func billTermToResponse(t domain.BillTerm) map[string]any {
	return map[string]any{
		"guid":         t.GUID,
		"bookGuid":     t.BookGUID,
		"name":         t.Name,
		"description":  t.Description,
		"type":         string(t.Type),
		"dueDays":      t.DueDays,
		"discountDays": t.DiscountDays,
		"discount":     numericAtScale(t.Discount, 0), // reduced exact ratio
		"cutoff":       t.Cutoff,
	}
}

// writeBillTermError maps domain validation/not-found errors to status codes.
func writeBillTermError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, domain.ErrBillTermNotFound):
		writeError(w, http.StatusNotFound, "bill term not found")
	case errors.Is(err, domain.ErrInvalidBillTerm):
		writeError(w, http.StatusBadRequest, "invalid bill term: "+err.Error())
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid bill term")
	default:
		writeError(w, http.StatusInternalServerError, "bill term request failed")
	}
	return true
}

func (s *Server) handleListBillTerms(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	terms, err := s.BillTerm.List(r.Context(), bookGUID)
	if writeBillTermError(w, err) {
		return
	}
	out := make([]map[string]any, len(terms))
	for i, t := range terms {
		out[i] = billTermToResponse(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "billTerms": out})
}

func (s *Server) handleCreateBillTerm(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto billTermDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	t, err := dto.toDomain(bookGUID, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bill term: "+err.Error())
		return
	}
	created, err := s.BillTerm.Create(r.Context(), t)
	if writeBillTermError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, billTermToResponse(created))
}

func (s *Server) handleGetBillTerm(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	t, err := s.BillTerm.Get(r.Context(), guid)
	if writeBillTermError(w, err) {
		return
	}
	if !s.authorizeBook(w, r, t.BookGUID, app.AccessRead) {
		return
	}
	writeJSON(w, http.StatusOK, billTermToResponse(t))
}

func (s *Server) handleUpdateBillTerm(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.BillTerm.BookGUIDForBillTerm(r.Context(), guid)
	if writeBillTermError(w, err) {
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto billTermDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	t, err := dto.toDomain(bookGUID, guid)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bill term: "+err.Error())
		return
	}
	updated, err := s.BillTerm.Update(r.Context(), t)
	if writeBillTermError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, billTermToResponse(updated))
}

func (s *Server) handleDeleteBillTerm(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.BillTerm.BookGUIDForBillTerm(r.Context(), guid)
	if writeBillTermError(w, err) {
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	if err := s.BillTerm.Delete(r.Context(), guid); writeBillTermError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
