package domain

import (
	"errors"
	"time"
)

// ErrCustomerNotFound is returned when a customer lookup finds no matching row.
var ErrCustomerNotFound = errors.New("customer not found")

// ErrVendorNotFound is returned when a vendor lookup finds no matching row.
var ErrVendorNotFound = errors.New("vendor not found")

// Address is a postal + contact address shared by Customer (billing/shipping)
// and Vendor. All fields are optional; empty string means not set.
type Address struct {
	Name  string
	Addr1 string
	Addr2 string
	Phone string
	Email string
}

// Customer is a business contact on the accounts-receivable side.
type Customer struct {
	GUID         string
	BookGUID     string
	Name         string
	ID           string // display number, e.g. "CUST-0001"
	Notes        string
	Active       bool
	CurrencyGUID string
	Addr         Address
	CreditLimit  GncNumeric
	TermsGUID    string
	CreatedAt    time.Time
}

// Vendor is a business contact on the accounts-payable side.
type Vendor struct {
	GUID         string
	BookGUID     string
	Name         string
	ID           string
	Notes        string
	Active       bool
	CurrencyGUID string
	Addr         Address
	TermsGUID    string
	CreatedAt    time.Time
}
