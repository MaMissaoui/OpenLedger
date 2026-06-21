package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type taxTableEntryDTO struct {
	AccountGUID string     `json:"accountGuid"`
	Type        string     `json:"type"` // "percentage" or "value"
	Amount      numericDTO `json:"amount"`
}

type taxTableDTO struct {
	Name    string             `json:"name"`
	Entries []taxTableEntryDTO `json:"entries"`
}

func (dto taxTableDTO) toDomain(bookGUID, existingGUID string) (domain.TaxTable, error) {
	entries := make([]domain.TaxTableEntry, len(dto.Entries))
	for i, e := range dto.Entries {
		amount, err := e.Amount.toDomain()
		if err != nil {
			return domain.TaxTable{}, err
		}
		entries[i] = domain.TaxTableEntry{
			AccountGUID: e.AccountGUID,
			Type:        domain.TaxEntryType(e.Type),
			Amount:      amount,
		}
	}
	return domain.TaxTable{
		GUID:     existingGUID,
		BookGUID: bookGUID,
		Name:     dto.Name,
		Entries:  entries,
	}, nil
}

func taxTableToResponse(tt domain.TaxTable) map[string]any {
	entries := make([]map[string]any, len(tt.Entries))
	for i, e := range tt.Entries {
		entries[i] = map[string]any{
			"accountGuid": e.AccountGUID,
			"type":        string(e.Type),
			"amount":      numericAtScale(e.Amount, 0), // reduced exact ratio
		}
	}
	return map[string]any{
		"guid":     tt.GUID,
		"bookGuid": tt.BookGUID,
		"name":     tt.Name,
		"entries":  entries,
	}
}

// writeTaxTableError maps domain validation/not-found errors to status codes.
func writeTaxTableError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, domain.ErrTaxTableNotFound):
		writeError(w, http.StatusNotFound, "tax table not found")
	case errors.Is(err, domain.ErrInvalidTaxTable):
		writeError(w, http.StatusBadRequest, "invalid tax table: "+err.Error())
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid tax table")
	default:
		writeError(w, http.StatusInternalServerError, "tax table request failed")
	}
	return true
}

func (s *Server) handleListTaxTables(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	tables, err := s.TaxTable.List(r.Context(), bookGUID)
	if writeTaxTableError(w, err) {
		return
	}
	out := make([]map[string]any, len(tables))
	for i, tt := range tables {
		out[i] = taxTableToResponse(tt)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "taxTables": out})
}

func (s *Server) handleCreateTaxTable(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto taxTableDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	tt, err := dto.toDomain(bookGUID, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tax table: "+err.Error())
		return
	}
	created, err := s.TaxTable.Create(r.Context(), tt)
	if writeTaxTableError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, taxTableToResponse(created))
}

func (s *Server) handleGetTaxTable(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	tt, err := s.TaxTable.Get(r.Context(), guid)
	if writeTaxTableError(w, err) {
		return
	}
	if !s.authorizeBook(w, r, tt.BookGUID, app.AccessRead) {
		return
	}
	writeJSON(w, http.StatusOK, taxTableToResponse(tt))
}

func (s *Server) handleUpdateTaxTable(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.TaxTable.BookGUIDForTaxTable(r.Context(), guid)
	if writeTaxTableError(w, err) {
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	var dto taxTableDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	tt, err := dto.toDomain(bookGUID, guid)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tax table: "+err.Error())
		return
	}
	updated, err := s.TaxTable.Update(r.Context(), tt)
	if writeTaxTableError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, taxTableToResponse(updated))
}

func (s *Server) handleDeleteTaxTable(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	bookGUID, err := s.TaxTable.BookGUIDForTaxTable(r.Context(), guid)
	if writeTaxTableError(w, err) {
		return
	}
	if !s.authorizeBook(w, r, bookGUID, app.AccessWrite) {
		return
	}
	if err := s.TaxTable.Delete(r.Context(), guid); writeTaxTableError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
