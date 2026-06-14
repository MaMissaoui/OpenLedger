package domain

import (
	"errors"
	"time"
)

// RecurrencePeriod names the temporal unit for a scheduled transaction.
type RecurrencePeriod string

// Supported recurrence periods.
const (
	PeriodOnce    RecurrencePeriod = "once"
	PeriodDaily   RecurrencePeriod = "daily"
	PeriodWeekly  RecurrencePeriod = "weekly"
	PeriodMonthly RecurrencePeriod = "monthly"
	PeriodYearly  RecurrencePeriod = "yearly"
)

// IsValid reports whether p is a supported recurrence period.
func (p RecurrencePeriod) IsValid() bool {
	switch p {
	case PeriodOnce, PeriodDaily, PeriodWeekly, PeriodMonthly, PeriodYearly:
		return true
	default:
		return false
	}
}

// ScheduledSplit is one template leg of a scheduled transaction. It carries a
// value in the transaction currency (no quantity/commodity distinction — the
// actual split's quantity is filled in at post time for same-currency accounts).
type ScheduledSplit struct {
	GUID        string
	AccountGUID string
	Memo        string
	Value       GncNumeric
}

// ScheduledTransaction defines a recurring posting. When Enabled is true and
// the next due date is on or before a given date, PostingService creates a real
// transaction from the template splits.
type ScheduledTransaction struct {
	GUID           string
	BookGUID       string
	Name           string
	Description    string
	Enabled        bool
	CurrencyGUID   string
	Period         RecurrencePeriod
	Every          int       // multiplier on Period (e.g. Every=2 + PeriodWeekly → biweekly)
	StartDate      time.Time // date component only; time is ignored
	EndDate        time.Time // zero = no end
	LastPostedDate time.Time // zero = never posted
	Splits         []ScheduledSplit
}

// NextDueDate returns the earliest date on which this scheduled transaction is
// due that is after LastPostedDate (or equal to StartDate if never posted). It
// returns the zero time when the transaction is disabled, has been fully
// consumed (PeriodOnce already posted), or its EndDate has passed.
func (s ScheduledTransaction) NextDueDate() time.Time {
	if !s.Enabled {
		return time.Time{}
	}
	every := s.Every
	if every <= 0 {
		every = 1
	}
	// Start from StartDate (date-only, UTC midnight).
	cur := s.StartDate.UTC().Truncate(24 * time.Hour)
	for {
		if !s.EndDate.IsZero() && cur.After(s.EndDate.UTC()) {
			return time.Time{}
		}
		if s.LastPostedDate.IsZero() || cur.After(s.LastPostedDate.UTC()) {
			return cur
		}
		switch s.Period {
		case PeriodOnce:
			return time.Time{} // already posted
		case PeriodDaily:
			cur = cur.AddDate(0, 0, every)
		case PeriodWeekly:
			cur = cur.AddDate(0, 0, every*7)
		case PeriodMonthly:
			cur = cur.AddDate(0, every, 0)
		case PeriodYearly:
			cur = cur.AddDate(every, 0, 0)
		default:
			return time.Time{}
		}
	}
}

// IsDue reports whether the next due date is on or before asOf.
func (s ScheduledTransaction) IsDue(asOf time.Time) bool {
	next := s.NextDueDate()
	if next.IsZero() {
		return false
	}
	return !next.After(asOf.UTC())
}

// ErrScheduleNotBalanced is returned when the template splits do not sum to zero.
var ErrScheduleNotBalanced = errors.New("scheduled transaction splits must balance to zero")

// ValidateBalanced checks that the template split values sum to zero in the
// transaction currency.
func (s ScheduledTransaction) ValidateBalanced() error {
	var sum GncNumeric
	for _, sp := range s.Splits {
		sum = sum.Add(sp.Value)
	}
	if !sum.IsZero() {
		return ErrScheduleNotBalanced
	}
	return nil
}
