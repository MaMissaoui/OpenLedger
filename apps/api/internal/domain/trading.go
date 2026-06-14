package domain

import "sort"

// TradingSplit is a trading-account posting the engine wants to add to keep a
// transaction balanced per-commodity. It names the commodity to balance and the
// value (in the transaction currency) and quantity (in that commodity) to post;
// the caller maps the commodity to a concrete Trading:NAMESPACE:MNEMONIC
// account.
type TradingSplit struct {
	CommodityGUID string
	Value         GncNumeric // in the transaction currency
	Quantity      GncNumeric // in the commodity
}

// ComputeTradingSplits returns the trading-account splits that make a
// transaction balance per-commodity: after they are added, every commodity nets
// to zero in both quantity and value across the whole transaction. This mirrors
// GnuCash's trading-accounts mechanism, which keeps the book balanced in every
// commodity, not just the transaction currency.
//
// commodityOf maps each split's account GUID to its commodity GUID; isTrading
// marks accounts that are themselves trading accounts, which are excluded from
// the net so the function is idempotent across edits. It returns nil when every
// involved commodity already nets to zero (e.g. a single-currency transaction),
// so it adds nothing in the common case.
//
// It assumes the transaction already balances in value (the caller validates
// that first). Each trading split's value is the negated per-commodity net
// value, so when the transaction balances in value the trading splits also sum
// to zero in value and the overall balance is preserved.
func ComputeTradingSplits(splits []Split, commodityOf map[string]string, isTrading map[string]bool) []TradingSplit {
	type net struct{ value, quantity GncNumeric }
	nets := make(map[string]*net)
	for _, s := range splits {
		if isTrading[s.AccountGUID] {
			continue
		}
		commodity := commodityOf[s.AccountGUID]
		if commodity == "" {
			continue // unknown commodity; nothing to balance against
		}
		n, ok := nets[commodity]
		if !ok {
			n = &net{value: Zero(), quantity: Zero()}
			nets[commodity] = n
		}
		n.value = n.value.Add(s.Value)
		n.quantity = n.quantity.Add(s.Quantity)
	}

	// Emit in a deterministic (commodity GUID) order so a transaction's trading
	// splits are stable across posts.
	commodities := make([]string, 0, len(nets))
	for c := range nets {
		commodities = append(commodities, c)
	}
	sort.Strings(commodities)

	var out []TradingSplit
	for _, c := range commodities {
		n := nets[c]
		if n.value.IsZero() && n.quantity.IsZero() {
			continue
		}
		out = append(out, TradingSplit{
			CommodityGUID: c,
			Value:         n.value.Neg(),
			Quantity:      n.quantity.Neg(),
		})
	}
	return out
}
