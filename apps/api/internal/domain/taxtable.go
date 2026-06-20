package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidTaxTable is returned by TaxTable.Validate for a malformed table.
var ErrInvalidTaxTable = errors.New("invalid tax table")

// ErrTaxTableNotFound is returned when a tax table GUID does not resolve.
var ErrTaxTableNotFound = errors.New("tax table not found")

// TaxEntryType selects how a TaxTableEntry charges tax.
type TaxEntryType string

const (
	// TaxPercentage charges Amount percent of the taxed base.
	TaxPercentage TaxEntryType = "percentage"
	// TaxValue charges a flat Amount regardless of the base.
	TaxValue TaxEntryType = "value"
)

// TaxTableEntry is one component of a tax table: a charge posted to one account.
type TaxTableEntry struct {
	AccountGUID string
	Amount      GncNumeric // percentage: the rate (e.g. 10 for 10%)
	Type        TaxEntryType
}

// TaxCharge is a computed tax amount destined for one account.
type TaxCharge struct {
	AccountGUID string
	Amount      GncNumeric
}

// TaxTable is a named set of tax entries applied to a taxable amount.
type TaxTable struct {
	GUID     string
	BookGUID string
	Name     string
	Entries  []TaxTableEntry
}

var hundred = MustFromNumDenom(100, 1)

// Validate reports whether the tax table is well-formed: it has at least one
// entry, and every entry has a known type, an account, and a non-negative amount.
func (tt TaxTable) Validate() error {
	if len(tt.Entries) == 0 {
		return fmt.Errorf("%w: must have at least one entry", ErrInvalidTaxTable)
	}
	for i, e := range tt.Entries {
		switch e.Type {
		case TaxPercentage, TaxValue:
		default:
			return fmt.Errorf("%w: entry %d has unknown type %q", ErrInvalidTaxTable, i, e.Type)
		}
		if e.AccountGUID == "" {
			return fmt.Errorf("%w: entry %d has no account", ErrInvalidTaxTable, i)
		}
		if e.Amount.Sign() < 0 {
			return fmt.Errorf("%w: entry %d amount must not be negative", ErrInvalidTaxTable, i)
		}
	}
	return nil
}

// ComputeTax returns the tax charges for a pre-tax base amount, one per entry.
func (tt TaxTable) ComputeTax(base GncNumeric) []TaxCharge {
	charges := make([]TaxCharge, 0, len(tt.Entries))
	for _, e := range tt.Entries {
		amount := e.Amount // TaxValue: flat
		if e.Type == TaxPercentage {
			amount, _ = base.Mul(e.Amount).Div(hundred)
		}
		charges = append(charges, TaxCharge{AccountGUID: e.AccountGUID, Amount: amount})
	}
	return charges
}

// TaxableLine pairs a pre-tax base amount with the tax table to apply to it.
// A nil Table means the line is not taxed.
type TaxableLine struct {
	Base  GncNumeric
	Table *TaxTable
}

// AggregateTax sums tax across taxable lines, merging charges by account (in
// first-seen order) and returning the grand total. Lines without a table add no
// tax. Used to build invoice tax splits and extend the A/R or A/P total.
func AggregateTax(lines []TaxableLine) (charges []TaxCharge, total GncNumeric) {
	total = Zero()
	idxByAccount := make(map[string]int)
	for _, line := range lines {
		if line.Table == nil {
			continue
		}
		for _, c := range line.Table.ComputeTax(line.Base) {
			total = total.Add(c.Amount)
			if i, ok := idxByAccount[c.AccountGUID]; ok {
				charges[i].Amount = charges[i].Amount.Add(c.Amount)
				continue
			}
			idxByAccount[c.AccountGUID] = len(charges)
			charges = append(charges, c)
		}
	}
	return charges, total
}

// TotalTax returns the sum of all tax charges for a pre-tax base amount.
func (tt TaxTable) TotalTax(base GncNumeric) GncNumeric {
	total := Zero()
	for _, c := range tt.ComputeTax(base) {
		total = total.Add(c.Amount)
	}
	return total
}
