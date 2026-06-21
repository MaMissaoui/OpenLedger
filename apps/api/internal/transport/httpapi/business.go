package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type addressDTO struct {
	Name  string `json:"name"`
	Addr1 string `json:"addr1"`
	Addr2 string `json:"addr2"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

type customerBodyDTO struct {
	Name         string      `json:"name"`
	ID           string      `json:"id"`
	Notes        string      `json:"notes"`
	Active       bool        `json:"active"`
	CurrencyGUID string      `json:"currencyGuid"`
	Addr         addressDTO  `json:"addr"`
	CreditLimit  *numericDTO `json:"creditLimit"`
	TermsGUID    string      `json:"termsGuid"`
}

type vendorBodyDTO struct {
	Name         string     `json:"name"`
	ID           string     `json:"id"`
	Notes        string     `json:"notes"`
	Active       bool       `json:"active"`
	CurrencyGUID string     `json:"currencyGuid"`
	Addr         addressDTO `json:"addr"`
	TermsGUID    string     `json:"termsGuid"`
}

func customerToResponse(c domain.Customer) map[string]any {
	return map[string]any{
		"guid":         c.GUID,
		"bookGuid":     c.BookGUID,
		"name":         c.Name,
		"id":           c.ID,
		"notes":        c.Notes,
		"active":       c.Active,
		"currencyGuid": c.CurrencyGUID,
		"addr": map[string]any{
			"name":  c.Addr.Name,
			"addr1": c.Addr.Addr1,
			"addr2": c.Addr.Addr2,
			"phone": c.Addr.Phone,
			"email": c.Addr.Email,
		},
		"creditLimit": numericAtScale(c.CreditLimit, 100),
		"termsGuid":   c.TermsGUID,
		"createdAt":   c.CreatedAt,
	}
}

func vendorToResponse(v domain.Vendor) map[string]any {
	return map[string]any{
		"guid":         v.GUID,
		"bookGuid":     v.BookGUID,
		"name":         v.Name,
		"id":           v.ID,
		"notes":        v.Notes,
		"active":       v.Active,
		"currencyGuid": v.CurrencyGUID,
		"addr": map[string]any{
			"name":  v.Addr.Name,
			"addr1": v.Addr.Addr1,
			"addr2": v.Addr.Addr2,
			"phone": v.Addr.Phone,
			"email": v.Addr.Email,
		},
		"termsGuid": v.TermsGUID,
		"createdAt": v.CreatedAt,
	}
}

func parseCreditLimit(dto *numericDTO) (domain.GncNumeric, error) {
	if dto == nil {
		return domain.Zero(), nil
	}
	return dto.toDomain()
}

// ── Customer handlers ─────────────────────────────────────────────────────────

func (s *Server) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	activeOnly := r.URL.Query().Get("active") == "true"
	list, err := s.Customer.ListCustomers(r.Context(), bookGUID, userID, activeOnly)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	out := make([]map[string]any, len(list))
	for i, c := range list {
		out[i] = customerToResponse(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "customers": out})
}

func (s *Server) handleCreateCustomer(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto customerBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	credit, err := parseCreditLimit(dto.CreditLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid creditLimit")
		return
	}
	c := domain.Customer{
		BookGUID:     bookGUID,
		Name:         dto.Name,
		ID:           dto.ID,
		Notes:        dto.Notes,
		Active:       dto.Active,
		CurrencyGUID: dto.CurrencyGUID,
		Addr:         domain.Address{Name: dto.Addr.Name, Addr1: dto.Addr.Addr1, Addr2: dto.Addr.Addr2, Phone: dto.Addr.Phone, Email: dto.Addr.Email},
		CreditLimit:  credit,
		TermsGUID:    dto.TermsGUID,
	}
	created, err := s.Customer.CreateCustomer(r.Context(), userID, c)
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusCreated, customerToResponse(created))
	}
}

func (s *Server) handleGetCustomer(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	c, err := s.Customer.GetCustomer(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrCustomerNotFound):
		writeError(w, http.StatusNotFound, "customer not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, customerToResponse(c))
	}
}

func (s *Server) handleUpdateCustomer(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto customerBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	credit, err := parseCreditLimit(dto.CreditLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid creditLimit")
		return
	}
	c := domain.Customer{
		GUID:         guid,
		Name:         dto.Name,
		ID:           dto.ID,
		Notes:        dto.Notes,
		Active:       dto.Active,
		CurrencyGUID: dto.CurrencyGUID,
		Addr:         domain.Address{Name: dto.Addr.Name, Addr1: dto.Addr.Addr1, Addr2: dto.Addr.Addr2, Phone: dto.Addr.Phone, Email: dto.Addr.Email},
		CreditLimit:  credit,
		TermsGUID:    dto.TermsGUID,
	}
	updated, err := s.Customer.UpdateCustomer(r.Context(), userID, c)
	switch {
	case errors.Is(err, domain.ErrCustomerNotFound):
		writeError(w, http.StatusNotFound, "customer not found")
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, customerToResponse(updated))
	}
}

func (s *Server) handleDeleteCustomer(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	err := s.Customer.DeleteCustomer(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrCustomerNotFound):
		writeError(w, http.StatusNotFound, "customer not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// ── Vendor handlers ───────────────────────────────────────────────────────────

func (s *Server) handleListVendors(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	activeOnly := r.URL.Query().Get("active") == "true"
	list, err := s.Vendor.ListVendors(r.Context(), bookGUID, userID, activeOnly)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	out := make([]map[string]any, len(list))
	for i, v := range list {
		out[i] = vendorToResponse(v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "vendors": out})
}

func (s *Server) handleCreateVendor(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto vendorBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	v := domain.Vendor{
		BookGUID:     bookGUID,
		Name:         dto.Name,
		ID:           dto.ID,
		Notes:        dto.Notes,
		Active:       dto.Active,
		CurrencyGUID: dto.CurrencyGUID,
		Addr:         domain.Address{Name: dto.Addr.Name, Addr1: dto.Addr.Addr1, Addr2: dto.Addr.Addr2, Phone: dto.Addr.Phone, Email: dto.Addr.Email},
		TermsGUID:    dto.TermsGUID,
	}
	created, err := s.Vendor.CreateVendor(r.Context(), userID, v)
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusCreated, vendorToResponse(created))
	}
}

func (s *Server) handleGetVendor(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	v, err := s.Vendor.GetVendor(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrVendorNotFound):
		writeError(w, http.StatusNotFound, "vendor not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, vendorToResponse(v))
	}
}

func (s *Server) handleUpdateVendor(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto vendorBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	v := domain.Vendor{
		GUID:         guid,
		Name:         dto.Name,
		ID:           dto.ID,
		Notes:        dto.Notes,
		Active:       dto.Active,
		CurrencyGUID: dto.CurrencyGUID,
		Addr:         domain.Address{Name: dto.Addr.Name, Addr1: dto.Addr.Addr1, Addr2: dto.Addr.Addr2, Phone: dto.Addr.Phone, Email: dto.Addr.Email},
		TermsGUID:    dto.TermsGUID,
	}
	updated, err := s.Vendor.UpdateVendor(r.Context(), userID, v)
	switch {
	case errors.Is(err, domain.ErrVendorNotFound):
		writeError(w, http.StatusNotFound, "vendor not found")
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, vendorToResponse(updated))
	}
}

func (s *Server) handleDeleteVendor(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	err := s.Vendor.DeleteVendor(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrVendorNotFound):
		writeError(w, http.StatusNotFound, "vendor not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
