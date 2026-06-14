package domain

import "errors"

// ErrInsufficientShares is returned when a sale tries to dispose of more shares
// than the account's open lots hold.
var ErrInsufficientShares = errors.New("insufficient shares")

// Lot is a persisted lot row: a grouping of a security account's splits for
// cost-basis tracking. It mirrors GnuCash's lots table.
type Lot struct {
	GUID        string
	AccountGUID string
	IsClosed    bool
}

// OpenLot is an open (or partially consumed) purchase lot for a security
// account: the shares still open and the cost basis attached to them. Lots are
// matched oldest-first (FIFO) when shares are sold; the caller supplies them in
// that order.
type OpenLot struct {
	GUID      string
	Remaining GncNumeric // shares still open (> 0)
	Cost      GncNumeric // cost basis attached to the remaining shares
}

// LotConsumption records how many shares a sale drew from one lot and the cost
// basis those shares carried out.
type LotConsumption struct {
	LotGUID   string
	Quantity  GncNumeric // shares consumed from the lot (> 0)
	Cost      GncNumeric // cost basis removed from the lot
	ClosesLot bool       // true when this consumes the lot's last open share
}

// FIFOResult is the outcome of matching a sale against open lots: which lots
// were consumed and the total cost basis removed.
type FIFOResult struct {
	Consumptions []LotConsumption
	TotalCost    GncNumeric
}

// MatchFIFO allocates a sale of qtySold shares against openLots, oldest first.
// For each lot it removes shares until the sale is filled, apportioning the
// lot's cost basis pro-rata for a partial consumption (cost × consumed/remaining)
// and removing the whole remaining cost when a lot is fully consumed — so the
// final share of a lot always carries its exact residual cost and no fraction of
// a cent is stranded. It errors if qtySold is not positive or exceeds the total
// open shares.
func MatchFIFO(openLots []OpenLot, qtySold GncNumeric) (FIFOResult, error) {
	if qtySold.Sign() <= 0 {
		return FIFOResult{}, errors.New("sale quantity must be positive")
	}
	remaining := qtySold
	res := FIFOResult{TotalCost: Zero()}
	for _, lot := range openLots {
		if remaining.Sign() <= 0 {
			break
		}
		if lot.Remaining.Sign() <= 0 {
			continue
		}
		take := lot.Remaining
		closes := true
		if remaining.Cmp(lot.Remaining) < 0 {
			take = remaining
			closes = false
		}
		cost := lot.Cost
		if !closes {
			// Partial: cost × (take / lot.Remaining), exact rational.
			frac, err := take.Div(lot.Remaining)
			if err != nil {
				return FIFOResult{}, err
			}
			cost = lot.Cost.Mul(frac)
		}
		res.Consumptions = append(res.Consumptions, LotConsumption{
			LotGUID:   lot.GUID,
			Quantity:  take,
			Cost:      cost,
			ClosesLot: closes,
		})
		res.TotalCost = res.TotalCost.Add(cost)
		remaining = remaining.Sub(take)
	}
	if remaining.Sign() > 0 {
		return FIFOResult{}, ErrInsufficientShares
	}
	return res, nil
}
