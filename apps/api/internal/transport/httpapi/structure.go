package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type newCommodityDTO struct {
	Namespace string `json:"namespace"`
	Mnemonic  string `json:"mnemonic"`
	Fullname  string `json:"fullname"`
	Fraction  int64  `json:"fraction"`
}

func (s *Server) handleCreateCommodity(w http.ResponseWriter, r *http.Request) {
	var dto newCommodityDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	c, err := s.structure.CreateCommodity(r.Context(), domain.Commodity{
		Namespace: dto.Namespace,
		Mnemonic:  dto.Mnemonic,
		Fullname:  dto.Fullname,
		Fraction:  dto.Fraction,
	})
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, commodityDTO(c))
}

func (s *Server) handleListCommodities(w http.ResponseWriter, r *http.Request) {
	commodities, err := s.structure.ListCommodities(r.Context())
	if writeStructureError(w, err) {
		return
	}
	out := make([]map[string]any, 0, len(commodities))
	for _, c := range commodities {
		out = append(out, commodityDTO(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"commodities": out})
}

func commodityDTO(c domain.Commodity) map[string]any {
	return map[string]any{
		"guid":      c.GUID,
		"namespace": c.Namespace,
		"mnemonic":  c.Mnemonic,
		"fraction":  c.Fraction,
	}
}

func (s *Server) handleCreateBook(w http.ResponseWriter, r *http.Request) {
	actor := actorFromContext(r.Context())
	book, err := s.structure.CreateBook(r.Context(), actor.UserID)
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"guid":            book.GUID,
		"rootAccountGuid": book.RootAccountGUID,
	})
}

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	actor := actorFromContext(r.Context())
	books, err := s.structure.ListBooks(r.Context(), actor.UserID)
	if writeStructureError(w, err) {
		return
	}
	out := make([]map[string]any, 0, len(books))
	for _, b := range books {
		out = append(out, map[string]any{
			"guid":            b.GUID,
			"rootAccountGuid": b.RootAccountGUID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"books": out})
}

type newAccountDTO struct {
	BookGUID      string `json:"bookGuid"`
	ParentGUID    string `json:"parentGuid"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	CommodityGUID string `json:"commodityGuid"`
	Code          string `json:"code"`
	Description   string `json:"description"`
	Placeholder   bool   `json:"placeholder"`
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var dto newAccountDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !s.authorizeBook(w, r, dto.BookGUID, app.AccessWrite) {
		return
	}
	a, err := s.structure.CreateAccount(r.Context(), dto.BookGUID, domain.Account{
		Name:          dto.Name,
		Type:          domain.AccountType(dto.Type),
		CommodityGUID: dto.CommodityGUID,
		ParentGUID:    dto.ParentGUID,
		Code:          dto.Code,
		Description:   dto.Description,
		Placeholder:   dto.Placeholder,
	})
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, accountDTO(a))
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	accounts, err := s.structure.ListAccounts(r.Context(), bookGUID)
	if writeStructureError(w, err) {
		return
	}
	out := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, accountWithBalanceDTO(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid": bookGUID,
		"accounts": out,
	})
}

func accountDTO(a domain.Account) map[string]any {
	return map[string]any{
		"guid":          a.GUID,
		"name":          a.Name,
		"type":          string(a.Type),
		"commodityGuid": a.CommodityGUID,
		"parentGuid":    a.ParentGUID,
		"code":          a.Code,
		"description":   a.Description,
		"placeholder":   a.Placeholder,
	}
}

// accountWithBalanceDTO is accountDTO plus the account's own balance and its
// subtree roll-up (own + same-commodity descendants), both rendered at the
// commodity fraction (like the register), used by the chart-of-accounts list.
func accountWithBalanceDTO(a app.AccountWithBalance) map[string]any {
	dto := accountDTO(a.Account)
	dto["balance"] = numericAtScale(a.Balance, a.BalanceScale)
	dto["subtreeBalance"] = numericAtScale(a.SubtreeBalance, a.BalanceScale)
	return dto
}

// writeStructureError maps a StructureService error to an HTTP response and
// reports whether it handled one (so callers return early). A nil error is a
// no-op returning false.
func writeStructureError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, app.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book not found")
	case errors.Is(err, app.ErrCommodityNotFound):
		writeError(w, http.StatusNotFound, "commodity not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
	return true
}
