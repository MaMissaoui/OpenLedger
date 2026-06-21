package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

const (
	defaultRegisterLimit = 50
	maxRegisterLimit     = 200
)

type registerEntryDTO struct {
	SplitGUID   string     `json:"splitGuid"`
	TxGUID      string     `json:"txGuid"`
	PostDate    time.Time  `json:"postDate"`
	Description string     `json:"description"`
	Memo        string     `json:"memo"`
	Reconcile   string     `json:"reconcile"`
	Value       numericDTO `json:"value"`
	Quantity    numericDTO `json:"quantity"`
	Balance     numericDTO `json:"balance"`
}

type registerPageDTO struct {
	AccountGUID string             `json:"accountGuid"`
	Total       int64              `json:"total"`
	Limit       int                `json:"limit"`
	Offset      int                `json:"offset"`
	Entries     []registerEntryDTO `json:"entries"`
}

func (s *Server) handleAccountRegister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeAccount(w, r, id, app.AccessRead) {
		return
	}
	limit := clampQueryInt(r, "limit", defaultRegisterLimit, 1, maxRegisterLimit)
	offset := clampQueryInt(r, "offset", 0, 0, 1<<31-1)

	page, err := s.Ledger.AccountRegister(r.Context(), id, limit, offset)
	switch {
	case errors.Is(err, app.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load register")
		return
	}

	writeJSON(w, http.StatusOK, toRegisterPageDTO(page))
}

func toRegisterPageDTO(p app.RegisterPage) registerPageDTO {
	entries := make([]registerEntryDTO, 0, len(p.Entries))
	for _, e := range p.Entries {
		entries = append(entries, registerEntryDTO{
			SplitGUID:   e.SplitGUID,
			TxGUID:      e.TxGUID,
			PostDate:    e.PostDate,
			Description: e.Description,
			Memo:        e.Memo,
			Reconcile:   string(e.Reconcile),
			Value:       numericAtScale(e.Value, e.ValueScale),
			Quantity:    numericAtScale(e.Quantity, e.QuantityScale),
			Balance:     numericAtScale(e.Balance, e.QuantityScale),
		})
	}
	return registerPageDTO{
		AccountGUID: p.AccountGUID,
		Total:       p.Total,
		Limit:       p.Limit,
		Offset:      p.Offset,
		Entries:     entries,
	}
}

// numericAtScale renders an amount at the given denominator (the commodity
// fraction), so e.g. $-62.34 emits as {-6234, 100} rather than the reduced
// {-3117, 50}. If the amount is not exact at that scale (or scale is unset), it
// falls back to the reduced exact form; the underlying value is never lost.
func numericAtScale(n domain.GncNumeric, scale int64) numericDTO {
	if scale > 0 {
		if num, err := n.AtDenom(scale); err == nil {
			return numericDTO{Num: num, Denom: scale}
		}
	}
	num, denom, err := n.NumDenom()
	if err != nil {
		return numericDTO{Num: 0, Denom: 1}
	}
	return numericDTO{Num: num, Denom: denom}
}

func clampQueryInt(r *http.Request, key string, def, lo, hi int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
