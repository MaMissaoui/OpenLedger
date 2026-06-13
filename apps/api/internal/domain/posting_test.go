package domain

import (
	"errors"
	"testing"
)

// twoSplit builds a simple same-currency transaction: +value to debit account,
// -value to credit account.
func twoSplit(value GncNumeric) Transaction {
	return Transaction{
		CurrencyGUID: "USD",
		Splits: []Split{
			{AccountGUID: "assets:checking", Value: value, Quantity: value},
			{AccountGUID: "expenses:groceries", Value: value.Neg(), Quantity: value.Neg()},
		},
	}
}

func TestBalancedTransactionPasses(t *testing.T) {
	tx := twoSplit(MustFromNumDenom(5000, 100)) // $50.00
	if err := tx.ValidateBalanced(); err != nil {
		t.Fatalf("balanced transaction rejected: %v", err)
	}
	if !tx.Imbalance().IsZero() {
		t.Fatalf("imbalance = %s, want 0", tx.Imbalance())
	}
}

func TestUnbalancedTransactionFails(t *testing.T) {
	tx := twoSplit(MustFromNumDenom(5000, 100))
	tx.Splits[1].Value = MustFromNumDenom(-4900, 100) // off by $1.00
	err := tx.ValidateBalanced()
	if err == nil {
		t.Fatal("unbalanced transaction was accepted")
	}
	if !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("error = %v, want ErrUnbalanced", err)
	}
	if want := MustFromNumDenom(100, 100); !tx.Imbalance().Equal(want) {
		t.Fatalf("imbalance = %s, want %s", tx.Imbalance(), want)
	}
}

func TestSingleSplitFails(t *testing.T) {
	tx := Transaction{
		CurrencyGUID: "USD",
		Splits:       []Split{{AccountGUID: "assets:checking", Value: Zero()}},
	}
	if err := tx.ValidateBalanced(); !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("error = %v, want ErrUnbalanced", err)
	}
}

func TestMultiSplitBalanced(t *testing.T) {
	// Paycheck: +1000 checking, -1200 income(gross), +200 taxes expense.
	tx := Transaction{
		CurrencyGUID: "USD",
		Splits: []Split{
			{AccountGUID: "assets:checking", Value: MustFromNumDenom(100000, 100)},
			{AccountGUID: "income:salary", Value: MustFromNumDenom(-120000, 100)},
			{AccountGUID: "expenses:taxes", Value: MustFromNumDenom(20000, 100)},
		},
	}
	if err := tx.ValidateBalanced(); err != nil {
		t.Fatalf("balanced multi-split rejected: %v", err)
	}
}
