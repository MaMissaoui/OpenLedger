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
