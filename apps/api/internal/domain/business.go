package domain

import (
	"errors"
	"time"
)

// ErrCustomerNotFound is returned when a customer lookup finds no matching row.
var ErrCustomerNotFound = errors.New("customer not found")

// ErrVendorNotFound is returned when a vendor lookup finds no matching row.
var ErrVendorNotFound = errors.New("vendor not found")

// ErrEmployeeNotFound is returned when an employee lookup finds no matching row.
var ErrEmployeeNotFound = errors.New("employee not found")

// ErrJobNotFound is returned when a job lookup finds no matching row.
var ErrJobNotFound = errors.New("job not found")

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

// Employee is a person reimbursed via expense vouchers. Rate is the default
// hourly billing rate in the employee's currency.
type Employee struct {
	GUID         string
	BookGUID     string
	Name         string
	Username     string
	ID           string
	Notes        string
	Active       bool
	CurrencyGUID string
	Addr         Address
	Rate         GncNumeric
	CreatedAt    time.Time
}

// Job groups invoices or bills under one customer or vendor. OwnerType is
// "customer" or "vendor" and OwnerGUID points at that row.
type Job struct {
	GUID      string
	BookGUID  string
	Name      string
	ID        string
	Reference string
	Active    bool
	OwnerType string
	OwnerGUID string
	CreatedAt time.Time
}
