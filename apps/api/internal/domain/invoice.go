package domain

import (
	"errors"
	"time"
)

var (
	ErrInvoiceNotFound      = errors.New("invoice not found")
	ErrEntryNotFound        = errors.New("entry not found")
	ErrInvoiceAlreadyPosted = errors.New("invoice is already posted")
	ErrInvoiceNoEntries     = errors.New("invoice has no entries")
	ErrInvoiceNotPosted     = errors.New("invoice has not been posted")
	ErrInvoiceAlreadyPaid   = errors.New("invoice is already paid")
)

type InvoiceType string

const (
	InvoiceTypeCustomer InvoiceType = "invoice"
	InvoiceTypeBill     InvoiceType = "bill"
	InvoiceTypeVoucher  InvoiceType = "expense_voucher" // employee expense reimbursement
)

// InvoiceEntry is one line item on an invoice or bill.
type InvoiceEntry struct {
	GUID         string
	InvoiceGUID  string
	Date         time.Time
	Description  string
	Action       string
	Notes        string
	Quantity     GncNumeric
	AccountGUID  string
	Price        GncNumeric
	Taxable      bool
	TaxTableGUID string // tax table applied when Taxable; empty = no tax
	CreatedAt    time.Time
}

func (e InvoiceEntry) LineTotal() GncNumeric {
	return e.Quantity.Mul(e.Price)
}

// Invoice is a customer invoice (A/R) or vendor bill (A/P).
type Invoice struct {
	GUID         string
	BookGUID     string
	ID           string      // display number, e.g. "INV-0001"
	Type         InvoiceType // "invoice" or "bill"
	OwnerGUID    string      // customer or vendor guid
	DateOpened   time.Time
	DatePosted   *time.Time // nil = draft
	DateDue      *time.Time
	Notes        string
	Active       bool
	CurrencyGUID string
	PostTxnGUID  string // set when posted
	PostAccGUID  string // A/R or A/P account used when posted
	TermsGUID    string
	JobGUID      string     // optional: groups the invoice under a job
	PaidAt       *time.Time // nil = unpaid
	PaidTxnGUID  string     // set when paid
	CreatedAt    time.Time
	Entries      []InvoiceEntry // loaded on demand
}

func (inv Invoice) IsPosted() bool {
	return inv.DatePosted != nil
}

func (inv Invoice) IsPaid() bool {
	return inv.PaidAt != nil
}

func (inv Invoice) Total() GncNumeric {
	t := Zero()
	for _, e := range inv.Entries {
		t = t.Add(e.LineTotal())
	}
	return t
}
