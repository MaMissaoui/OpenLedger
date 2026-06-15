package domain

import (
	"errors"
	"time"
)

// BudgetPeriodType names the length of one budget period.
type BudgetPeriodType string

// Supported budget period types.
const (
	BudgetMonthly   BudgetPeriodType = "monthly"
	BudgetQuarterly BudgetPeriodType = "quarterly"
	BudgetYearly    BudgetPeriodType = "yearly"
)

// IsValid reports whether p is a supported period type.
func (p BudgetPeriodType) IsValid() bool {
	switch p {
	case BudgetMonthly, BudgetQuarterly, BudgetYearly:
		return true
	default:
		return false
	}
}

// BudgetAmount is the planned spending or income for one account in one period.
// Value is in the account's commodity (natural sign: positive for the account
// type's normal direction).
type BudgetAmount struct {
	AccountGUID string
	PeriodNum   int
	Value       GncNumeric
}

// Budget defines a multi-period spending plan for a book.
type Budget struct {
	GUID        string
	BookGUID    string
	Name        string
	Description string
	PeriodType  BudgetPeriodType
	NumPeriods  int
	StartDate   time.Time
	Amounts     []BudgetAmount
}

// ErrBudgetNotFound is returned when a budget lookup finds no matching row.
var ErrBudgetNotFound = errors.New("budget not found")

// PeriodStart returns the first moment of period i (UTC midnight).
func (b Budget) PeriodStart(i int) time.Time {
	s := b.StartDate.UTC().Truncate(24 * time.Hour)
	switch b.PeriodType {
	case BudgetMonthly:
		return s.AddDate(0, i, 0)
	case BudgetQuarterly:
		return s.AddDate(0, i*3, 0)
	case BudgetYearly:
		return s.AddDate(i, 0, 0)
	default:
		return s.AddDate(0, i, 0)
	}
}

// PeriodEnd returns the last moment of period i (one second before the next
// period starts, so it aligns with the <= comparison in AccountBalances).
func (b Budget) PeriodEnd(i int) time.Time {
	return b.PeriodStart(i + 1).Add(-time.Second)
}

// PeriodLabel returns a human-readable label for period i ("Jan 2024", "Q1 2024", "2024").
func (b Budget) PeriodLabel(i int) string {
	s := b.PeriodStart(i)
	switch b.PeriodType {
	case BudgetMonthly:
		return s.Format("Jan 2006")
	case BudgetQuarterly:
		q := (int(s.Month())-1)/3 + 1
		return "Q" + string(rune('0'+q)) + " " + s.Format("2006")
	case BudgetYearly:
		return s.Format("2006")
	default:
		return s.Format("Jan 2006")
	}
}

// PeriodIndex returns the 0-based index of the period that contains d, or -1
// if d is outside the budget's date range (before start or after the last period).
func (b Budget) PeriodIndex(d time.Time) int {
	s := b.PeriodStart(0)
	t := d.UTC().Truncate(24 * time.Hour)
	if t.Before(s) {
		return -1
	}
	var idx int
	switch b.PeriodType {
	case BudgetMonthly:
		years := t.Year() - s.Year()
		months := int(t.Month()) - int(s.Month())
		idx = years*12 + months
	case BudgetQuarterly:
		years := t.Year() - s.Year()
		months := int(t.Month()) - int(s.Month())
		idx = (years*12 + months) / 3
	case BudgetYearly:
		idx = t.Year() - s.Year()
	}
	if idx >= b.NumPeriods {
		return -1
	}
	return idx
}
