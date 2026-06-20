package bankimport

import (
	"strings"
	"testing"
)

const sampleOFX = `OFXHEADER:100
DATA:OFXSGML
VERSION:102

<OFX>
<BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<TRNTYPE>DEBIT
<DTPOSTED>20240619120000[-5:EST]
<TRNAMT>-50.00
<FITID>FIT-1
<NAME>SAFEWAY
<MEMO>GROCERIES
</STMTTRN>
<STMTTRN>
<TRNTYPE>CREDIT
<DTPOSTED>20240620
<TRNAMT>1000.00
<FITID>FIT-2
<NAME>EMPLOYER
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1>
</OFX>
`

func TestOFXRead(t *testing.T) {
	txns, err := OFX{}.Read(strings.NewReader(sampleOFX))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(txns) != 2 {
		t.Fatalf("got %d txns, want 2", len(txns))
	}

	if got := txns[0].Date.Format("2006-01-02"); got != "2024-06-19" {
		t.Errorf("txn0 date = %s, want 2024-06-19 (DTPOSTED truncated to date)", got)
	}
	if got := txns[0].Amount.DecimalString(2); got != "-50.00" {
		t.Errorf("txn0 amount = %s, want -50.00", got)
	}
	if txns[0].FITID != "FIT-1" {
		t.Errorf("txn0 FITID = %q, want FIT-1", txns[0].FITID)
	}
	if txns[0].Memo != "SAFEWAY — GROCERIES" {
		t.Errorf("txn0 memo = %q, want 'SAFEWAY — GROCERIES'", txns[0].Memo)
	}

	if got := txns[1].Amount.DecimalString(2); got != "1000.00" {
		t.Errorf("txn1 amount = %s, want 1000.00", got)
	}
	if txns[1].FITID != "FIT-2" || txns[1].Memo != "EMPLOYER" {
		t.Errorf("txn1 = %+v, want FITID FIT-2 and memo EMPLOYER", txns[1])
	}
}

func TestOFXReadNoTransactionsIsError(t *testing.T) {
	if _, err := (OFX{}).Read(strings.NewReader("<OFX></OFX>")); err == nil {
		t.Fatal("expected an error when there are no STMTTRN blocks")
	}
}
