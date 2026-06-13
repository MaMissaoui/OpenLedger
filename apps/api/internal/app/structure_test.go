package app

import (
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

func num(t *testing.T, n, d int64) domain.GncNumeric {
	t.Helper()
	v, err := domain.FromNumDenom(n, d)
	if err != nil {
		t.Fatalf("FromNumDenom(%d, %d): %v", n, d, err)
	}
	return v
}

// TestRollUpSubtreeBalances checks that a parent's subtree balance sums its own
// balance plus same-commodity descendants, while a differing-commodity child
// (e.g. a stock account under a currency parent) is left out of the roll-up.
func TestRollUpSubtreeBalances(t *testing.T) {
	const usd, stock = "usd", "stock"
	acct := func(guid, parent, commodity string, balNum int64) AccountWithBalance {
		return AccountWithBalance{
			Account:      domain.Account{GUID: guid, ParentGUID: parent, CommodityGUID: commodity},
			Balance:      num(t, balNum, 100),
			BalanceScale: 100,
		}
	}
	// "root" is the (excluded) book root, so assets/expenses are forest roots.
	accts := []AccountWithBalance{
		acct("assets", "root", usd, 0), // placeholder
		acct("checking", "assets", usd, 10000),
		acct("savings", "assets", usd, 5000),
		acct("invest", "assets", usd, 0),     // placeholder
		acct("broker", "invest", stock, 500), // 5 "shares" — different commodity
		acct("expenses", "root", usd, 0),     // placeholder
		acct("groceries", "expenses", usd, 2000),
	}

	rollUpSubtreeBalances(accts)

	want := map[string]int64{ // expected subtree balance numerator at denom 100
		"checking":  10000,
		"savings":   5000,
		"broker":    500,
		"invest":    0,     // its only child is a different commodity
		"assets":    15000, // 100 + 50 (invest rolls up 0; broker excluded)
		"groceries": 2000,
		"expenses":  2000,
	}
	for _, a := range accts {
		got, err := a.SubtreeBalance.AtDenom(100)
		if err != nil {
			t.Fatalf("%s subtree not exact at denom 100: %v", a.Account.GUID, err)
		}
		if w, ok := want[a.Account.GUID]; ok && got != w {
			t.Errorf("%s subtree = %d, want %d", a.Account.GUID, got, w)
		}
	}
}
