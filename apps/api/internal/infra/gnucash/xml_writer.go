package gnucash

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// gncXMLWriteTime is the timestamp format GnuCash's XML backend writes for
// <ts:date>; it is the first (canonical) layout ReadGnuCashXML accepts. Times
// are emitted as UTC.
const gncXMLWriteTime = "2006-01-02 15:04:05 -0700"

// xmlHeader is the document preamble: the XML declaration plus the <gnc-v2>
// root with the namespace declarations GnuCash emits. The unqualified-local-name
// reader only needs the prefixes to be syntactically present; the URIs match
// GnuCash so a real GnuCash install also accepts the file.
const xmlHeader = `<?xml version="1.0" encoding="utf-8" ?>
<gnc-v2
     xmlns:gnc="http://www.gnucash.org/XML/gnc"
     xmlns:act="http://www.gnucash.org/XML/act"
     xmlns:book="http://www.gnucash.org/XML/book"
     xmlns:cd="http://www.gnucash.org/XML/cd"
     xmlns:cmdty="http://www.gnucash.org/XML/cmdty"
     xmlns:slot="http://www.gnucash.org/XML/slot"
     xmlns:split="http://www.gnucash.org/XML/split"
     xmlns:trn="http://www.gnucash.org/XML/trn"
     xmlns:ts="http://www.gnucash.org/XML/ts">
`

// xmlWriter is an error-tracking wrapper over a buffered writer (the Effective Go
// "errWriter" pattern): once a write fails, subsequent writes are no-ops and the
// first error is reported by err(). It keeps the per-element write code free of
// repetitive error checks while still surfacing I/O failures.
type xmlWriter struct {
	w   *bufio.Writer
	err error
}

func (x *xmlWriter) printf(format string, a ...any) {
	if x.err != nil {
		return
	}
	_, x.err = fmt.Fprintf(x.w, format, a...)
}

// WriteGnuCashXML writes data out as an (uncompressed) GnuCash XML file at path.
// The format mirrors GnuCash's XML backend closely enough that the file
// re-imports through ReadGnuCashXML and opens in GnuCash itself: commodities are
// referenced by namespace+mnemonic (their synthesised GUIDs are not emitted, as
// the XML format has no commodity-GUID slot), and money is written as GnuCash
// "num/denom" rationals at each commodity's fraction.
//
// The synthesised template root is not written: GnuCash keeps template accounts
// in its <gnc:template-transactions> section, not the main account tree, so the
// exported book carries a single ROOT account like a native file. SQLite remains
// the higher-fidelity export (it preserves commodity GUIDs and the template
// root); XML is the portable, human-readable alternative.
func (Writer) WriteGnuCashXML(_ context.Context, path string, data app.GnuCashData) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create xml: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	fraction := make(map[string]int64, len(data.Commodities))
	cmdtyRef := make(map[string]xmlRef, len(data.Commodities))
	for _, c := range data.Commodities {
		fraction[c.GUID] = c.Fraction
		cmdtyRef[c.GUID] = xmlRef{space: c.Namespace, id: c.Mnemonic}
	}
	acctCommodity := make(map[string]string, len(data.Accounts))
	for _, a := range data.Accounts {
		acctCommodity[a.GUID] = a.CommodityGUID
	}

	// Emit every account except the synthesised template root (GnuCash keeps it
	// in its template-transactions section, which we don't write).
	accounts := make([]domain.Account, 0, len(data.Accounts))
	for _, a := range data.Accounts {
		if a.GUID == data.Book.RootTemplateGUID {
			continue
		}
		accounts = append(accounts, a)
	}

	x := &xmlWriter{w: bufio.NewWriter(f)}
	x.printf("%s", xmlHeader)
	x.writeCountData("book", 1)
	x.printf("<gnc:book version=\"2.0.0\">\n")
	x.printf("<book:id type=\"guid\">%s</book:id>\n", xmlEscape(data.Book.GUID))
	x.writeCountData("commodity", len(data.Commodities))
	x.writeCountData("account", len(accounts))
	x.writeCountData("transaction", len(data.Transactions))

	for _, c := range data.Commodities {
		x.writeCommodity(c)
	}
	for _, a := range accounts {
		x.writeAccount(a, cmdtyRef[a.CommodityGUID], fraction[a.CommodityGUID])
	}
	for _, t := range data.Transactions {
		if err := x.writeTransaction(t, cmdtyRef, fraction, acctCommodity); err != nil {
			return err
		}
	}
	x.printf("</gnc:book>\n</gnc-v2>\n")

	if x.err != nil {
		return fmt.Errorf("write xml: %w", x.err)
	}
	if err := x.w.Flush(); err != nil {
		return fmt.Errorf("flush xml: %w", err)
	}
	return nil
}

// xmlRef is a commodity reference written inline on accounts and transactions:
// the namespace+mnemonic pair that points at a commodity definition.
type xmlRef struct {
	space string
	id    string
}

func (x *xmlWriter) writeCountData(typ string, n int) {
	x.printf("<gnc:count-data cd:type=\"%s\">%d</gnc:count-data>\n", typ, n)
}

func (x *xmlWriter) writeCommodity(c domain.Commodity) {
	x.printf("<gnc:commodity version=\"2.0.0\">\n")
	x.printf("  <cmdty:space>%s</cmdty:space>\n", xmlEscape(c.Namespace))
	x.printf("  <cmdty:id>%s</cmdty:id>\n", xmlEscape(c.Mnemonic))
	if c.Fullname != "" {
		x.printf("  <cmdty:name>%s</cmdty:name>\n", xmlEscape(c.Fullname))
	}
	x.printf("  <cmdty:fraction>%d</cmdty:fraction>\n", c.Fraction)
	x.printf("</gnc:commodity>\n")
}

func (x *xmlWriter) writeAccount(a domain.Account, ref xmlRef, scu int64) {
	x.printf("<gnc:account version=\"2.0.0\">\n")
	x.printf("  <act:name>%s</act:name>\n", xmlEscape(a.Name))
	x.printf("  <act:id type=\"guid\">%s</act:id>\n", xmlEscape(a.GUID))
	x.printf("  <act:type>%s</act:type>\n", xmlEscape(string(a.Type)))
	if a.CommodityGUID != "" {
		x.printf("  <act:commodity>\n")
		x.printf("    <cmdty:space>%s</cmdty:space>\n", xmlEscape(ref.space))
		x.printf("    <cmdty:id>%s</cmdty:id>\n", xmlEscape(ref.id))
		x.printf("  </act:commodity>\n")
		if scu > 0 {
			x.printf("  <act:commodity-scu>%d</act:commodity-scu>\n", scu)
		}
	}
	if a.Code != "" {
		x.printf("  <act:code>%s</act:code>\n", xmlEscape(a.Code))
	}
	if a.Description != "" {
		x.printf("  <act:description>%s</act:description>\n", xmlEscape(a.Description))
	}
	x.writeAccountSlots(a)
	if a.ParentGUID != "" {
		x.printf("  <act:parent type=\"guid\">%s</act:parent>\n", xmlEscape(a.ParentGUID))
	}
	x.printf("</gnc:account>\n")
}

// writeAccountSlots emits the placeholder/hidden flags as KVP slots, matching
// how GnuCash stores them, so they survive a round trip.
func (x *xmlWriter) writeAccountSlots(a domain.Account) {
	if !a.Placeholder && !a.Hidden {
		return
	}
	x.printf("  <act:slots>\n")
	if a.Placeholder {
		x.writeBoolSlot("placeholder")
	}
	if a.Hidden {
		x.writeBoolSlot("hidden")
	}
	x.printf("  </act:slots>\n")
}

func (x *xmlWriter) writeBoolSlot(key string) {
	x.printf("    <slot>\n")
	x.printf("      <slot:key>%s</slot:key>\n", xmlEscape(key))
	x.printf("      <slot:value type=\"string\">true</slot:value>\n")
	x.printf("    </slot>\n")
}

func (x *xmlWriter) writeTransaction(t domain.Transaction, cmdtyRef map[string]xmlRef, fraction map[string]int64, acctCommodity map[string]string) error {
	currencyFraction := fraction[t.CurrencyGUID]
	if currencyFraction == 0 {
		return fmt.Errorf("transaction %s: unknown currency %s", t.GUID, t.CurrencyGUID)
	}
	ref := cmdtyRef[t.CurrencyGUID]

	x.printf("<gnc:transaction version=\"2.0.0\">\n")
	x.printf("  <trn:id type=\"guid\">%s</trn:id>\n", xmlEscape(t.GUID))
	x.printf("  <trn:currency>\n")
	x.printf("    <cmdty:space>%s</cmdty:space>\n", xmlEscape(ref.space))
	x.printf("    <cmdty:id>%s</cmdty:id>\n", xmlEscape(ref.id))
	x.printf("  </trn:currency>\n")
	if t.Num != "" {
		x.printf("  <trn:num>%s</trn:num>\n", xmlEscape(t.Num))
	}
	x.printf("  <trn:date-posted><ts:date>%s</ts:date></trn:date-posted>\n", gncXMLTime(t.PostDate))
	x.printf("  <trn:date-entered><ts:date>%s</ts:date></trn:date-entered>\n", gncXMLTime(t.EnterDate))
	x.printf("  <trn:description>%s</trn:description>\n", xmlEscape(t.Description))
	x.printf("  <trn:splits>\n")
	for _, s := range t.Splits {
		if err := x.writeSplit(t.GUID, s, currencyFraction, fraction, acctCommodity); err != nil {
			return err
		}
	}
	x.printf("  </trn:splits>\n")
	x.printf("</gnc:transaction>\n")
	return nil
}

func (x *xmlWriter) writeSplit(txGUID string, s domain.Split, currencyFraction int64, fraction map[string]int64, acctCommodity map[string]string) error {
	value, err := gncRational(s.Value, currencyFraction)
	if err != nil {
		return fmt.Errorf("transaction %s split %s value: %w", txGUID, s.GUID, err)
	}
	accFraction := fraction[acctCommodity[s.AccountGUID]]
	if accFraction == 0 {
		return fmt.Errorf("split %s: unknown commodity for account %s", s.GUID, s.AccountGUID)
	}
	quantity, err := gncRational(s.Quantity, accFraction)
	if err != nil {
		return fmt.Errorf("transaction %s split %s quantity: %w", txGUID, s.GUID, err)
	}
	reconcile := s.Reconcile
	if reconcile == 0 {
		reconcile = domain.ReconcileNew
	}

	x.printf("    <trn:split>\n")
	x.printf("      <split:id type=\"guid\">%s</split:id>\n", xmlEscape(s.GUID))
	if s.Memo != "" {
		x.printf("      <split:memo>%s</split:memo>\n", xmlEscape(s.Memo))
	}
	if s.Action != "" {
		x.printf("      <split:action>%s</split:action>\n", xmlEscape(s.Action))
	}
	x.printf("      <split:reconciled-state>%s</split:reconciled-state>\n", string(reconcile))
	x.printf("      <split:value>%s</split:value>\n", value)
	x.printf("      <split:quantity>%s</split:quantity>\n", quantity)
	x.printf("      <split:account type=\"guid\">%s</split:account>\n", xmlEscape(s.AccountGUID))
	x.printf("    </trn:split>\n")
	return nil
}

// gncRational renders amount as a GnuCash "num/denom" string at the given
// fraction (denominator). An amount that is not exact at that fraction is an
// error rather than being rounded.
func gncRational(amount domain.GncNumeric, denom int64) (string, error) {
	num, err := amount.AtDenom(denom)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d/%d", num, denom), nil
}

// gncXMLTime renders a timestamp in GnuCash's XML <ts:date> format (UTC), or the
// empty string for the zero time.
func gncXMLTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(gncXMLWriteTime)
}

// xmlEscape escapes a string for inclusion in XML character data or a
// double-quoted attribute value.
func xmlEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		// EscapeText only fails if the underlying writer fails; a strings.Builder
		// never does.
		return s
	}
	return b.String()
}
