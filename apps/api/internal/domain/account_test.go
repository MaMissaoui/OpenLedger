package domain

import "testing"

func TestNaturalBalance(t *testing.T) {
	pos := MustFromNumDenom(10000, 100)  // +100.00 raw (a debit)
	neg := MustFromNumDenom(-10000, 100) // -100.00 raw (a credit)

	tests := []struct {
		typ  AccountType
		raw  GncNumeric
		want GncNumeric
	}{
		// Debit-normal: the raw sign is the natural sign.
		{AccountAsset, pos, pos},
		{AccountBank, neg, neg},
		{AccountExpense, pos, pos},
		// Credit-normal: a stored credit (negative) reads as a positive balance.
		{AccountIncome, neg, pos},
		{AccountLiability, neg, pos},
		{AccountEquity, neg, pos},
		{AccountPayable, neg, pos},
	}
	for _, tc := range tests {
		if got := tc.typ.NaturalBalance(tc.raw); !got.Equal(tc.want) {
			t.Errorf("%s.NaturalBalance(%s) = %s, want %s", tc.typ, tc.raw, got, tc.want)
		}
	}
}
