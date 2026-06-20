package bankimport

import (
	"strings"
	"testing"
)

func TestQIFRead(t *testing.T) {
	const data = `!Type:Bank
D6/19'24
T-50.00
PSafeway
MGroceries
^
D6/20/2024
T1,000.00
PEmployer
^
`
	txns, err := QIF{}.Read(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(txns) != 2 {
		t.Fatalf("got %d txns, want 2", len(txns))
	}

	if got := txns[0].Date.Format("2006-01-02"); got != "2024-06-19" {
		t.Errorf("txn0 date = %s, want 2024-06-19", got)
	}
	if got := txns[0].Amount.DecimalString(2); got != "-50.00" {
		t.Errorf("txn0 amount = %s, want -50.00", got)
	}
	if txns[0].Memo != "Safeway — Groceries" {
		t.Errorf("txn0 memo = %q, want 'Safeway — Groceries'", txns[0].Memo)
	}

	// Comma grouping must parse, and the credit keeps its sign.
	if got := txns[1].Amount.DecimalString(2); got != "1000.00" {
		t.Errorf("txn1 amount = %s, want 1000.00", got)
	}
	if got := txns[1].Date.Format("2006-01-02"); got != "2024-06-20" {
		t.Errorf("txn1 date = %s, want 2024-06-20", got)
	}
}

func TestQIFReadEmptyIsError(t *testing.T) {
	if _, err := (QIF{}).Read(strings.NewReader("not a qif file")); err == nil {
		t.Fatal("expected an error for a non-QIF file")
	}
}

func TestQIFRecordMissingAmountIsError(t *testing.T) {
	const data = "!Type:Bank\nD6/19'24\nPSafeway\n^\n"
	if _, err := (QIF{}).Read(strings.NewReader(data)); err == nil {
		t.Fatal("expected an error when a record has no amount")
	}
}
