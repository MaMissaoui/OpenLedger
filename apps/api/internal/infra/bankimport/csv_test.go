package bankimport

import (
	"strings"
	"testing"
)

func TestCSVSingleAmount(t *testing.T) {
	const data = `Date,Amount,Description
2024-06-19,-50.00,Safeway
2024-06-20,"1,000.00",Employer Inc
`
	r := CSV{Mapping: CSVMapping{
		HasHeader: true, DateCol: 0, DateFormat: "2006-01-02",
		AmountCol: 1, DebitCol: -1, CreditCol: -1, DescCols: []int{2},
	}}
	txns, err := r.Read(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(txns) != 2 {
		t.Fatalf("got %d txns, want 2", len(txns))
	}
	if got := txns[0].Amount.DecimalString(2); got != "-50.00" {
		t.Errorf("txn0 amount = %s, want -50.00", got)
	}
	if txns[0].Date.Format("2006-01-02") != "2024-06-19" || txns[0].Memo != "Safeway" {
		t.Errorf("txn0 = %+v", txns[0])
	}
	// Quoted thousands separator must parse.
	if got := txns[1].Amount.DecimalString(2); got != "1000.00" {
		t.Errorf("txn1 amount = %s, want 1000.00", got)
	}
}

func TestCSVDebitCredit(t *testing.T) {
	const data = `Date,Debit,Credit,Memo
06/19/2024,50.00,,Coffee
06/20/2024,,2500.00,Payroll
`
	r := CSV{Mapping: CSVMapping{
		HasHeader: true, DateCol: 0, DateFormat: "01/02/2006",
		AmountCol: -1, DebitCol: 1, CreditCol: 2, DescCols: []int{3},
	}}
	txns, err := r.Read(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Debit is money out (negative); credit money in.
	if got := txns[0].Amount.DecimalString(2); got != "-50.00" {
		t.Errorf("debit row amount = %s, want -50.00", got)
	}
	if got := txns[1].Amount.DecimalString(2); got != "2500.00" {
		t.Errorf("credit row amount = %s, want 2500.00", got)
	}
}

func TestCSVInvertAndAccountingNegatives(t *testing.T) {
	// Outflows listed as positive, plus accounting parentheses and a currency
	// symbol — Invert flips the sign of the single amount column.
	const data = `2024-06-19,"($1,234.56)"
2024-06-20,200.00
`
	r := CSV{Mapping: CSVMapping{
		HasHeader: false, DateCol: 0, DateFormat: "2006-01-02",
		AmountCol: 1, DebitCol: -1, CreditCol: -1, Invert: true,
	}}
	txns, err := r.Read(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// ($1,234.56) parses to -1234.56, inverted to +1234.56.
	if got := txns[0].Amount.DecimalString(2); got != "1234.56" {
		t.Errorf("row0 amount = %s, want 1234.56", got)
	}
	// 200.00 inverted to -200.00.
	if got := txns[1].Amount.DecimalString(2); got != "-200.00" {
		t.Errorf("row1 amount = %s, want -200.00", got)
	}
}

func TestCSVAutoDateAndBlankRows(t *testing.T) {
	const data = `2024-06-19,-50.00,A

2024-06-20,10.00,B
`
	r := CSV{Mapping: CSVMapping{
		DateCol: 0, AmountCol: 1, DebitCol: -1, CreditCol: -1, DescCols: []int{2},
	}}
	txns, err := r.Read(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(txns) != 2 {
		t.Fatalf("got %d txns, want 2 (blank row skipped)", len(txns))
	}
}

func TestCSVEmptyIsError(t *testing.T) {
	r := CSV{Mapping: CSVMapping{DateCol: 0, AmountCol: 1, DebitCol: -1, CreditCol: -1}}
	if _, err := r.Read(strings.NewReader("\n\n")); err == nil {
		t.Fatal("expected an error for a CSV with no data rows")
	}
}

func TestPreviewCSV(t *testing.T) {
	const data = `Date,Amount,Description
2024-06-19,-50.00,Safeway
2024-06-20,1000.00,Employer
`
	p, err := PreviewCSV(strings.NewReader(data), 11)
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	if p.TotalRows != 3 || p.Columns != 3 {
		t.Errorf("preview totals = %d rows / %d cols, want 3/3", p.TotalRows, p.Columns)
	}
	if len(p.Rows) != 3 || p.Rows[0][0] != "Date" || p.Rows[1][1] != "-50.00" {
		t.Errorf("preview rows = %v", p.Rows)
	}
}
