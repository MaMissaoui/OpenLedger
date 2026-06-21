package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// tradeDTO is the request body for a security buy or sell. shares is in the
// security's commodity; cash is the total paid (buy) or received (sell) in the
// cash account's currency.
type tradeDTO struct {
	SecurityAccountGUID string     `json:"securityAccountGuid"`
	CashAccountGUID     string     `json:"cashAccountGuid"`
	Shares              numericDTO `json:"shares"`
	Cash                numericDTO `json:"cash"`
	Description         string     `json:"description"`
	PostDate            *time.Time `json:"postDate"`
}

func (s *Server) handleBuySecurity(w http.ResponseWriter, r *http.Request) {
	s.handleTrade(w, r, false)
}
func (s *Server) handleSellSecurity(w http.ResponseWriter, r *http.Request) {
	s.handleTrade(w, r, true)
}

// handleTrade posts a security purchase or sale. It authorizes write access on
// both accounts, then delegates to the TradeService, which maintains the
// purchase lots and (on a sale) realizes a FIFO capital gain.
func (s *Server) handleTrade(w http.ResponseWriter, r *http.Request, sell bool) {
	var dto tradeDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	shares, err := dto.Shares.toDomain()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shares amount")
		return
	}
	cash, err := dto.Cash.toDomain()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cash amount")
		return
	}
	if !s.authorizeAccounts(w, r, []string{dto.SecurityAccountGUID, dto.CashAccountGUID}, app.AccessWrite) {
		return
	}

	trade := app.Trade{
		SecurityAccountGUID: dto.SecurityAccountGUID,
		CashAccountGUID:     dto.CashAccountGUID,
		Shares:              shares,
		Cash:                cash,
		Description:         dto.Description,
	}
	if dto.PostDate != nil {
		trade.PostDate = *dto.PostDate
	}

	actor := actorFromContext(r.Context())
	var res app.TradeResult
	if sell {
		res, err = s.Trade.Sell(r.Context(), trade, actor)
	} else {
		res, err = s.Trade.Buy(r.Context(), trade, actor)
	}
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, domain.ErrInsufficientShares):
		writeError(w, http.StatusUnprocessableEntity, "selling more shares than held")
		return
	case errors.Is(err, domain.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, app.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not record the trade")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"transactionGuid": res.TransactionGUID,
		"realizedGain":    numericAtScale(res.RealizedGain, 0),
	})
}
