package gnucash

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// sampleXML is a minimal but realistic GnuCash XML book: a USD currency (with no
// explicit fraction, exercising the cents default), a root + two postable
// accounts, and one balanced transaction. Element names are namespaced exactly
// as GnuCash writes them.
const sampleXML = `<?xml version="1.0" encoding="utf-8" ?>
<gnc-v2
  xmlns:gnc="http://www.gnucash.org/XML/gnc"
  xmlns:act="http://www.gnucash.org/XML/act"
  xmlns:book="http://www.gnucash.org/XML/book"
  xmlns:cmdty="http://www.gnucash.org/XML/cmdty"
  xmlns:trn="http://www.gnucash.org/XML/trn"
  xmlns:split="http://www.gnucash.org/XML/split"
  xmlns:ts="http://www.gnucash.org/XML/ts"
  xmlns:slot="http://www.gnucash.org/XML/slot">
<gnc:book version="2.0.0">
<book:id type="guid">bookguid</book:id>
<gnc:commodity version="2.0.0">
  <cmdty:space>CURRENCY</cmdty:space>
  <cmdty:id>USD</cmdty:id>
  <cmdty:name>US Dollar</cmdty:name>
</gnc:commodity>
<gnc:account version="2.0.0">
  <act:name>Root Account</act:name>
  <act:id type="guid">rootguid</act:id>
  <act:type>ROOT</act:type>
</gnc:account>
<gnc:account version="2.0.0">
  <act:name>Checking</act:name>
  <act:id type="guid">chkguid</act:id>
  <act:type>BANK</act:type>
  <act:commodity><cmdty:space>CURRENCY</cmdty:space><cmdty:id>USD</cmdty:id></act:commodity>
  <act:code>1010</act:code>
  <act:description>Main bank</act:description>
  <act:parent type="guid">rootguid</act:parent>
</gnc:account>
<gnc:account version="2.0.0">
  <act:name>Salary</act:name>
  <act:id type="guid">salguid</act:id>
  <act:type>INCOME</act:type>
  <act:commodity><cmdty:space>CURRENCY</cmdty:space><cmdty:id>USD</cmdty:id></act:commodity>
  <act:parent type="guid">rootguid</act:parent>
  <act:slots>
    <slot><slot:key>hidden</slot:key><slot:value type="string">true</slot:value></slot>
  </act:slots>
</gnc:account>
<gnc:transaction version="2.0.0">
  <trn:id type="guid">txguid</trn:id>
  <trn:currency><cmdty:space>CURRENCY</cmdty:space><cmdty:id>USD</cmdty:id></trn:currency>
  <trn:date-posted><ts:date>2024-03-01 00:00:00 +0000</ts:date></trn:date-posted>
  <trn:date-entered><ts:date>2024-03-01 12:00:00 +0000</ts:date></trn:date-entered>
  <trn:description>Paycheck</trn:description>
  <trn:splits>
    <trn:split>
      <split:id type="guid">s1guid</split:id>
      <split:reconciled-state>n</split:reconciled-state>
      <split:value>500000/100</split:value>
      <split:quantity>500000/100</split:quantity>
      <split:account type="guid">chkguid</split:account>
    </trn:split>
    <trn:split>
      <split:id type="guid">s2guid</split:id>
      <split:reconciled-state>n</split:reconciled-state>
      <split:value>-500000/100</split:value>
      <split:quantity>-500000/100</split:quantity>
      <split:account type="guid">salguid</split:account>
    </trn:split>
  </trn:splits>
</gnc:transaction>
</gnc:book>
</gnc-v2>`

func writeXMLFixture(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.gnucash")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestReadGnuCashXML(t *testing.T) {
	data, err := (Reader{}).ReadGnuCashXML(context.Background(), writeXMLFixture(t, []byte(sampleXML)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if data.Book.GUID != "bookguid" || data.Book.RootAccountGUID != "rootguid" {
		t.Errorf("book = %+v", data.Book)
	}
	// The template root is synthesised (the XML has none) and must be a distinct
	// ROOT account present in the account set.
	if data.Book.RootTemplateGUID == "" || data.Book.RootTemplateGUID == "rootguid" {
		t.Errorf("template root = %q", data.Book.RootTemplateGUID)
	}

	if len(data.Commodities) != 1 {
		t.Fatalf("commodities = %d, want 1", len(data.Commodities))
	}
	usd := data.Commodities[0]
	if usd.Mnemonic != "USD" || usd.Namespace != "CURRENCY" || usd.Fraction != 100 {
		t.Errorf("commodity = %+v (fraction should default to 100)", usd)
	}
	if usd.GUID == "" || usd.GUID == "USD" {
		t.Errorf("commodity GUID = %q, want a synthesised hex GUID", usd.GUID)
	}

	// 3 accounts from the file + 1 synthesised template root.
	if len(data.Accounts) != 4 {
		t.Fatalf("accounts = %d, want 4", len(data.Accounts))
	}
	chk := findAccount(t, data.Accounts, "chkguid")
	if chk.Type != domain.AccountBank || chk.Code != "1010" || chk.ParentGUID != "rootguid" ||
		chk.Description != "Main bank" || chk.CommodityGUID != usd.GUID {
		t.Errorf("checking = %+v", chk)
	}
	if !findAccount(t, data.Accounts, "salguid").Hidden {
		t.Error("salary account should be hidden (from slot)")
	}
	if !findAccount(t, data.Accounts, data.Book.RootTemplateGUID).Type.IsValid() {
		t.Error("synthesised template root missing from accounts")
	}

	if len(data.Transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(data.Transactions))
	}
	tx := data.Transactions[0]
	if tx.GUID != "txguid" || tx.Description != "Paycheck" || tx.CurrencyGUID != usd.GUID {
		t.Errorf("tx = %+v", tx)
	}
	if tx.PostDate.IsZero() || tx.PostDate.Year() != 2024 || tx.PostDate.Month() != 3 {
		t.Errorf("post date = %v", tx.PostDate)
	}
	if err := tx.ValidateBalanced(); err != nil {
		t.Errorf("imported transaction does not balance: %v", err)
	}
	if len(tx.Splits) != 2 {
		t.Fatalf("splits = %d, want 2", len(tx.Splits))
	}
	if tx.Splits[0].AccountGUID != "chkguid" || tx.Splits[0].Reconcile != domain.ReconcileNew {
		t.Errorf("split[0] = %+v", tx.Splits[0])
	}
	want := domain.MustFromNumDenom(5000, 1)
	if got := tx.Splits[0].Value; !got.Equal(want) {
		t.Errorf("split[0] value = %s, want %s", got, want)
	}
}

func TestReadGnuCashXMLGzipped(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(sampleXML)); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	data, err := (Reader{}).ReadGnuCashXML(context.Background(), writeXMLFixture(t, buf.Bytes()))
	if err != nil {
		t.Fatalf("read gzipped: %v", err)
	}
	if data.Book.GUID != "bookguid" || len(data.Transactions) != 1 {
		t.Errorf("gzipped read mismatch: book=%q txns=%d", data.Book.GUID, len(data.Transactions))
	}
}

func TestReadGnuCashXMLNoBook(t *testing.T) {
	const noBook = `<?xml version="1.0"?><gnc-v2 xmlns:gnc="x"></gnc-v2>`
	data, err := (Reader{}).ReadGnuCashXML(context.Background(), writeXMLFixture(t, []byte(noBook)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if data.Book.GUID != "" {
		t.Errorf("expected empty book, got %+v", data.Book)
	}
}
