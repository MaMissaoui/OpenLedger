package domain

import "time"

// ReconcileState mirrors GnuCash's single-character split reconcile flags.
type ReconcileState rune

// The GnuCash reconcile-state flags.
const (
	ReconcileNew     ReconcileState = 'n'
	ReconcileCleared ReconcileState = 'c'
	ReconcileYes     ReconcileState = 'y'
	ReconcileFrozen  ReconcileState = 'f'
	ReconcileVoid    ReconcileState = 'v'
)

// Transaction is a dated economic event denominated in one currency. Its splits
// must balance to zero in that currency (see ValidateBalanced). It corresponds
// to a row in GnuCash's transactions table plus its child splits.
type Transaction struct {
	GUID         string
	CurrencyGUID string
	Num          string
	PostDate     time.Time
	EnterDate    time.Time
	Description  string
	Splits       []Split
}

// Split is one leg of a transaction posted to a single account. Value is
// expressed in the transaction's currency; Quantity is in the account's own
// commodity. For same-currency accounts the two are equal; for foreign-currency
// or security accounts they differ, and their ratio is the implied rate/price.
type Split struct {
	GUID        string
	AccountGUID string
	Memo        string
	Action      string
	Reconcile   ReconcileState
	Value       GncNumeric
	Quantity    GncNumeric
	LotGUID     string
}
