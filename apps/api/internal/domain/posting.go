package domain

import (
	"errors"
	"fmt"
)

// ErrUnbalanced is returned when a transaction's splits do not sum to zero in
// the transaction currency. It is the central accounting invariant: the posting
// service must reject any transaction that fails ValidateBalanced.
var ErrUnbalanced = errors.New("transaction does not balance to zero")

// Imbalance returns the sum of all split values in the transaction currency. A
// balanced transaction has a zero imbalance; the UI can offer this (negated) as
// the auto-balancing split, mirroring GnuCash's behaviour.
func (t Transaction) Imbalance() GncNumeric {
	vals := make([]GncNumeric, len(t.Splits))
	for i, s := range t.Splits {
		vals[i] = s.Value
	}
	return Sum(vals...)
}

// ValidateBalanced checks the double-entry invariant: a transaction needs at
// least two splits and their values must sum to exactly zero.
func (t Transaction) ValidateBalanced() error {
	if len(t.Splits) < 2 {
		return fmt.Errorf("%w: a transaction needs at least two splits, got %d", ErrUnbalanced, len(t.Splits))
	}
	if imb := t.Imbalance(); !imb.IsZero() {
		return fmt.Errorf("%w: imbalance of %s in currency %s", ErrUnbalanced, imb.String(), t.CurrencyGUID)
	}
	return nil
}
