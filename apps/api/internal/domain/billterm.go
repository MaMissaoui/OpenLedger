package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidBillTerm is returned by BillTerm.Validate for a malformed term.
var ErrInvalidBillTerm = errors.New("invalid bill term")

// ErrBillTermNotFound is returned when a payment term GUID does not resolve.
var ErrBillTermNotFound = errors.New("bill term not found")

// BillTermType selects how a BillTerm computes an invoice's due date.
type BillTermType string

const (
	// BillTermDays is a "net N days" term: due DueDays after the invoice date.
	BillTermDays BillTermType = "days"
	// BillTermProximo is a "due on a fixed day of a later month" term. DueDays
	// is the day-of-month the payment is due; Cutoff decides whether that day
	// falls in next month or the month after.
	BillTermProximo BillTermType = "proximo"
)

// BillTerm describes payment terms for an invoice or bill, mirroring GnuCash's
// billterms. It computes due and discount dates from an invoice date.
type BillTerm struct {
	GUID         string
	BookGUID     string
	Name         string
	Description  string
	Type         BillTermType
	DueDays      int        // days: net days after the invoice date; proximo: due day-of-month
	Cutoff       int        // proximo: invoices dated after this day-of-month roll to the month after
	DiscountDays int        // early-payment window, in days after the invoice date
	Discount     GncNumeric // early-payment discount as a fraction, e.g. 2/100 for 2%
}

// Validate reports whether the term is well-formed.
func (t BillTerm) Validate() error {
	switch t.Type {
	case BillTermDays:
		if t.DueDays < 0 {
			return fmt.Errorf("%w: due days must not be negative", ErrInvalidBillTerm)
		}
	case BillTermProximo:
		if t.DueDays < 1 || t.DueDays > 31 {
			return fmt.Errorf("%w: proximo due day must be 1..31", ErrInvalidBillTerm)
		}
		if t.Cutoff < 1 || t.Cutoff > 31 {
			return fmt.Errorf("%w: proximo cutoff must be 1..31", ErrInvalidBillTerm)
		}
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidBillTerm, t.Type)
	}
	if t.DiscountDays < 0 {
		return fmt.Errorf("%w: discount days must not be negative", ErrInvalidBillTerm)
	}
	if t.Discount.Sign() < 0 {
		return fmt.Errorf("%w: discount must not be negative", ErrInvalidBillTerm)
	}
	return nil
}

// DueDate returns the date payment is due for an invoice dated invoiceDate.
func (t BillTerm) DueDate(invoiceDate time.Time) time.Time {
	if t.Type == BillTermProximo {
		monthsAhead := 1
		if invoiceDate.Day() > t.Cutoff {
			monthsAhead = 2
		}
		firstOfMonth := time.Date(invoiceDate.Year(), invoiceDate.Month(), 1, 0, 0, 0, 0, invoiceDate.Location())
		target := firstOfMonth.AddDate(0, monthsAhead, 0)
		daysInMonth := target.AddDate(0, 1, -1).Day()
		day := min(t.DueDays, daysInMonth)
		return time.Date(target.Year(), target.Month(), day, 0, 0, 0, 0, target.Location())
	}
	return invoiceDate.AddDate(0, 0, t.DueDays)
}

// ResolveDueDate decides an invoice's due date when it is posted. An explicit
// caller-supplied date always wins; otherwise the date derives from the term, if
// any; with neither, the due date is left unset (nil).
func ResolveDueDate(explicit *time.Time, term *BillTerm, invoiceDate time.Time) *time.Time {
	if explicit != nil {
		return explicit
	}
	if term != nil {
		d := term.DueDate(invoiceDate)
		return &d
	}
	return nil
}

// DiscountDate returns the last date an early-payment discount applies for an
// invoice dated invoiceDate. The second return is false when the term carries no
// discount, in which case the date is meaningless.
func (t BillTerm) DiscountDate(invoiceDate time.Time) (time.Time, bool) {
	if t.Discount.IsZero() {
		return time.Time{}, false
	}
	return invoiceDate.AddDate(0, 0, t.DiscountDays), true
}
