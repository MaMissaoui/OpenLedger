package domain

import "testing"

// perCommodityNets sums value and quantity by commodity across the given splits,
// so a test can assert every commodity balances to zero once trading splits are
// added.
func perCommodityNets(splits []Split, commodityOf map[string]string) map[string][2]GncNumeric {
	nets := make(map[string][2]GncNumeric)
	for _, s := range splits {
		c := commodityOf[s.AccountGUID]
		cur := nets[c]
		nets[c] = [2]GncNumeric{cur[0].Add(s.Value), cur[1].Add(s.Quantity)}
	}
	return nets
}

func TestComputeTradingSplitsSingleCurrencyIsNoOp(t *testing.T) {
	// A same-currency transaction: value == quantity on every split, so each
	// commodity already nets to zero. No trading splits should be produced.
	splits := []Split{
		{AccountGUID: "chk", Value: MustFromNumDenom(-5000, 100), Quantity: MustFromNumDenom(-5000, 100)},
		{AccountGUID: "grocery", Value: MustFromNumDenom(5000, 100), Quantity: MustFromNumDenom(5000, 100)},
	}
	commodityOf := map[string]string{"chk": "usd", "grocery": "usd"}

	got := ComputeTradingSplits(splits, commodityOf, nil)
	if got != nil {
		t.Errorf("expected no trading splits for single-currency tx, got %+v", got)
	}
}

func TestComputeTradingSplitsForeignExchange(t *testing.T) {
	// Buy €100 for $110 (rate 1.10); transaction currency is USD.
	//   EUR asset: quantity +100 EUR, value +110 USD
	//   USD asset: quantity -110 USD, value -110 USD
	splits := []Split{
		{AccountGUID: "eur", Value: MustFromNumDenom(11000, 100), Quantity: MustFromNumDenom(10000, 100)},
		{AccountGUID: "usd", Value: MustFromNumDenom(-11000, 100), Quantity: MustFromNumDenom(-11000, 100)},
	}
	commodityOf := map[string]string{"eur": "EUR", "usd": "USD"}

	trading := ComputeTradingSplits(splits, commodityOf, nil)
	if len(trading) != 2 {
		t.Fatalf("expected 2 trading splits, got %d: %+v", len(trading), trading)
	}

	// Map the trading splits onto accounts and assert every commodity nets to
	// zero in both value and quantity.
	all := append([]Split{}, splits...)
	for i, ts := range trading {
		acct := "trading-" + ts.CommodityGUID
		commodityOf[acct] = ts.CommodityGUID
		all = append(all, Split{AccountGUID: acct, Value: ts.Value, Quantity: ts.Quantity})
		_ = i
	}

	for commodity, net := range perCommodityNets(all, commodityOf) {
		if !net[0].IsZero() {
			t.Errorf("commodity %s value net = %s, want 0", commodity, net[0])
		}
		if !net[1].IsZero() {
			t.Errorf("commodity %s quantity net = %s, want 0", commodity, net[1])
		}
	}

	// The trading splits themselves must also sum to zero in value, preserving
	// the overall transaction balance.
	total := Zero()
	for _, ts := range trading {
		total = total.Add(ts.Value)
	}
	if !total.IsZero() {
		t.Errorf("trading split values sum = %s, want 0", total)
	}
}

func TestComputeTradingSplitsExcludesExistingTradingAccounts(t *testing.T) {
	// Re-running over a transaction that already has trading splits must produce
	// the same result (idempotence): the prior trading splits are ignored.
	splits := []Split{
		{AccountGUID: "eur", Value: MustFromNumDenom(11000, 100), Quantity: MustFromNumDenom(10000, 100)},
		{AccountGUID: "usd", Value: MustFromNumDenom(-11000, 100), Quantity: MustFromNumDenom(-11000, 100)},
	}
	commodityOf := map[string]string{"eur": "EUR", "usd": "USD"}
	first := ComputeTradingSplits(splits, commodityOf, nil)

	withTrading := append([]Split{}, splits...)
	isTrading := map[string]bool{}
	for _, ts := range first {
		acct := "trading-" + ts.CommodityGUID
		commodityOf[acct] = ts.CommodityGUID
		isTrading[acct] = true
		withTrading = append(withTrading, Split{AccountGUID: acct, Value: ts.Value, Quantity: ts.Quantity})
	}

	second := ComputeTradingSplits(withTrading, commodityOf, isTrading)
	if len(second) != len(first) {
		t.Fatalf("idempotence broken: first %d, second %d", len(first), len(second))
	}
	for i := range first {
		if first[i].CommodityGUID != second[i].CommodityGUID ||
			!first[i].Value.Equal(second[i].Value) ||
			!first[i].Quantity.Equal(second[i].Quantity) {
			t.Errorf("trading split %d differs: %+v vs %+v", i, first[i], second[i])
		}
	}
}
