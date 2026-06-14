package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// numericDTO is the wire form of a monetary amount: an exact num/denom pair,
// never a JSON float.
type numericDTO struct {
	Num   int64 `json:"num"`
	Denom int64 `json:"denom"`
}

func (n numericDTO) toDomain() (domain.GncNumeric, error) {
	return domain.FromNumDenom(n.Num, n.Denom)
}

type newSplitDTO struct {
	AccountGUID string     `json:"accountGuid"`
	Memo        string     `json:"memo"`
	Action      string     `json:"action"`
	Value       numericDTO `json:"value"`
	Quantity    numericDTO `json:"quantity"`
}

type newTransactionDTO struct {
	CurrencyGUID string        `json:"currencyGuid"`
	Num          string        `json:"num"`
	PostDate     *time.Time    `json:"postDate"`
	Description  string        `json:"description"`
	Splits       []newSplitDTO `json:"splits"`
}

func (s *Server) handlePostTransaction(w http.ResponseWriter, r *http.Request) {
	var dto newTransactionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	tx, err := dto.toDomain()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	accountGUIDs := make([]string, 0, len(tx.Splits))
	for _, sp := range tx.Splits {
		accountGUIDs = append(accountGUIDs, sp.AccountGUID)
	}
	if !s.authorizeAccounts(w, r, accountGUIDs, app.AccessWrite) {
		return
	}

	posted, err := s.posting.Post(r.Context(), tx, actorFromContext(r.Context()))
	switch {
	case errors.Is(err, domain.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not post transaction")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"guid":   posted.GUID,
		"splits": len(posted.Splits),
	})
}

// handleGetTransaction returns a transaction with all its splits. The edit UI
// needs every split, not just the one shown in a single account's register.
func (s *Server) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")

	accounts, err := s.posting.TransactionAccounts(r.Context(), guid)
	switch {
	case errors.Is(err, app.ErrTransactionNotFound):
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load transaction")
		return
	}
	if !s.authorizeAccounts(w, r, accounts, app.AccessRead) {
		return
	}

	tx, err := s.ledger.GetTransaction(r.Context(), guid)
	switch {
	case errors.Is(err, app.ErrTransactionNotFound):
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load transaction")
		return
	}
	writeJSON(w, http.StatusOK, transactionDTO(tx))
}

// transactionDTO renders a full transaction with its splits. Money fields use
// the stored scale (currency fraction for value, account fraction for quantity).
func transactionDTO(t domain.Transaction) map[string]any {
	splits := make([]map[string]any, 0, len(t.Splits))
	for _, s := range t.Splits {
		splits = append(splits, map[string]any{
			"guid":        s.GUID,
			"accountGuid": s.AccountGUID,
			"memo":        s.Memo,
			"action":      s.Action,
			"value":       numericAtScale(s.Value, 100),
			"quantity":    numericAtScale(s.Quantity, 100),
		})
	}
	return map[string]any{
		"guid":         t.GUID,
		"currencyGuid": t.CurrencyGUID,
		"num":          t.Num,
		"postDate":     t.PostDate,
		"description":  t.Description,
		"splits":       splits,
	}
}

// handleUpdateTransaction replaces a transaction wholesale (its fields and all
// splits) and re-validates the balance invariant. The request body has the same
// shape as a new transaction.
func (s *Server) handleUpdateTransaction(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")

	// Authorize against the transaction's *current* accounts first (404 if it
	// does not exist), so a caller can only edit transactions in books they may
	// write to.
	existing, err := s.posting.TransactionAccounts(r.Context(), guid)
	switch {
	case errors.Is(err, app.ErrTransactionNotFound):
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load transaction")
		return
	}
	if !s.authorizeAccounts(w, r, existing, app.AccessWrite) {
		return
	}

	var dto newTransactionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	tx, err := dto.toDomain()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx.GUID = guid

	// Also authorize against the new accounts, so splits cannot be moved into a
	// book the caller may not write to.
	newAccounts := make([]string, 0, len(tx.Splits))
	for _, sp := range tx.Splits {
		newAccounts = append(newAccounts, sp.AccountGUID)
	}
	if !s.authorizeAccounts(w, r, newAccounts, app.AccessWrite) {
		return
	}

	updated, err := s.posting.Update(r.Context(), tx, actorFromContext(r.Context()))
	switch {
	case errors.Is(err, domain.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, app.ErrTransactionNotFound):
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update transaction")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"guid":   updated.GUID,
		"splits": len(updated.Splits),
	})
}

// handleDeleteTransaction removes a transaction and its splits.
func (s *Server) handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")

	existing, err := s.posting.TransactionAccounts(r.Context(), guid)
	switch {
	case errors.Is(err, app.ErrTransactionNotFound):
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load transaction")
		return
	}
	if !s.authorizeAccounts(w, r, existing, app.AccessWrite) {
		return
	}

	switch err := s.posting.Delete(r.Context(), guid, actorFromContext(r.Context())); {
	case errors.Is(err, app.ErrTransactionNotFound):
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not delete transaction")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// toDomain validates request-shape requirements and maps the DTO to a domain
// transaction. The accounting balance check happens later in PostingService.
func (dto newTransactionDTO) toDomain() (domain.Transaction, error) {
	if dto.CurrencyGUID == "" {
		return domain.Transaction{}, errors.New("currencyGuid is required")
	}
	if len(dto.Splits) < 2 {
		return domain.Transaction{}, errors.New("at least two splits are required")
	}

	tx := domain.Transaction{
		CurrencyGUID: dto.CurrencyGUID,
		Num:          dto.Num,
		Description:  dto.Description,
	}
	if dto.PostDate != nil {
		tx.PostDate = *dto.PostDate
	}

	for i, sd := range dto.Splits {
		if sd.AccountGUID == "" {
			return domain.Transaction{}, fmt.Errorf("split %d: accountGuid is required", i)
		}
		value, err := sd.Value.toDomain()
		if err != nil {
			return domain.Transaction{}, fmt.Errorf("split %d value: %w", i, err)
		}
		quantity, err := sd.Quantity.toDomain()
		if err != nil {
			return domain.Transaction{}, fmt.Errorf("split %d quantity: %w", i, err)
		}
		tx.Splits = append(tx.Splits, domain.Split{
			AccountGUID: sd.AccountGUID,
			Memo:        sd.Memo,
			Action:      sd.Action,
			Value:       value,
			Quantity:    quantity,
		})
	}
	return tx, nil
}
