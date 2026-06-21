package gnucash

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// gncXMLTimeLayouts are the timestamp formats GnuCash's XML backend writes for
// <ts:date> (and the date-only <gdate> fallback). The first match wins; all are
// normalised to UTC.
var gncXMLTimeLayouts = []string{
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// XML document shape. GnuCash's XML uses namespaced element names (gnc:, act:,
// trn:, …); Go's encoding/xml matches on the local name when the struct tag
// carries no namespace, so the unqualified local names below bind regardless of
// prefix. Only direct children of <gnc:book> are decoded, which deliberately
// excludes the nested <gnc:template-transactions> accounts/transactions.
type xmlFile struct {
	Book xmlBook `xml:"book"`
}

type xmlBook struct {
	ID                   string             `xml:"id"`
	Commodities          []xmlCommodity     `xml:"commodity"`
	Accounts             []xmlAccount       `xml:"account"`
	Transactions         []xmlTransaction   `xml:"transaction"`
	ScheduledXactions    []xmlSchedXaction  `xml:"schedxaction"`
	TemplateTransactions xmlTemplateSection `xml:"template-transactions"`
}

// xmlSchedXaction is a <gnc:schedxaction> element from a GnuCash XML file.
type xmlSchedXaction struct {
	ID        string          `xml:"id"`
	Name      string          `xml:"name"`
	Enabled   string          `xml:"enabled"`
	Start     string          `xml:"start>gdate"`
	End       string          `xml:"end>gdate"`
	Last      string          `xml:"last>gdate"`
	TemplAcct string          `xml:"templ-acct"`
	Schedule  []xmlRecurrence `xml:"schedule>recurrence"`
}

// xmlRecurrence is a <gnc:recurrence> inside a scheduled transaction's
// <sx:schedule> element.
type xmlRecurrence struct {
	Mult       string `xml:"mult"`
	PeriodType string `xml:"period_type"`
	Start      string `xml:"start>gdate"`
}

// xmlTemplateSection is the <gnc:template-transactions> block, which holds
// template accounts and template transactions for scheduled transactions.
type xmlTemplateSection struct {
	Accounts     []xmlAccount     `xml:"account"`
	Transactions []xmlTransaction `xml:"transaction"`
}

type xmlCommodity struct {
	Space    string `xml:"space"`
	ID       string `xml:"id"`
	Name     string `xml:"name"`
	Fraction string `xml:"fraction"`
}

// xmlCmdtyRef is an inline commodity reference (an account's denomination or a
// transaction's currency): a namespace + mnemonic that points at a commodity.
type xmlCmdtyRef struct {
	Space string `xml:"space"`
	ID    string `xml:"id"`
}

type xmlAccount struct {
	Name        string      `xml:"name"`
	ID          string      `xml:"id"`
	Type        string      `xml:"type"`
	Commodity   xmlCmdtyRef `xml:"commodity"`
	Code        string      `xml:"code"`
	Description string      `xml:"description"`
	Parent      string      `xml:"parent"`
	Slots       []xmlSlot   `xml:"slots>slot"`
}

type xmlSlot struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}

type xmlTransaction struct {
	ID          string      `xml:"id"`
	Currency    xmlCmdtyRef `xml:"currency"`
	Num         string      `xml:"num"`
	DatePosted  string      `xml:"date-posted>date"`
	DateEntered string      `xml:"date-entered>date"`
	Description string      `xml:"description"`
	Splits      []xmlSplit  `xml:"splits>split"`
}

type xmlSplit struct {
	ID         string `xml:"id"`
	Memo       string `xml:"memo"`
	Action     string `xml:"action"`
	Reconciled string `xml:"reconciled-state"`
	Value      string `xml:"value"`
	Quantity   string `xml:"quantity"`
	Account    string `xml:"account"`
}

// ReadGnuCashXML opens a GnuCash XML file at path (optionally gzipped, which is
// GnuCash's default on-disk form) and reads its book together with every
// commodity, account, and transaction it contains, into domain types.
//
// Unlike the SQLite backend, the XML format identifies commodities by
// namespace+mnemonic rather than a GUID, and does not store the book's root
// account GUIDs. So this reader synthesises a GUID per commodity, derives the
// root account from the ROOT-typed account, and synthesises a template root
// (GnuCash always has one; the XML only emits it when scheduled transactions
// exist). Currencies that omit a fraction default to 100 (cents) — the SQLite
// backend remains the exact, highest-fidelity import path.
func (Reader) ReadGnuCashXML(_ context.Context, path string) (app.GnuCashData, error) {
	f, err := os.Open(path)
	if err != nil {
		return app.GnuCashData{}, fmt.Errorf("open xml: %w", err)
	}
	defer func() { _ = f.Close() }()

	// GnuCash gzips by default; sniff the magic bytes and wrap transparently.
	magic := make([]byte, 2)
	if _, err := f.Read(magic); err != nil {
		return app.GnuCashData{}, fmt.Errorf("read xml: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return app.GnuCashData{}, fmt.Errorf("seek xml: %w", err)
	}

	var dec *xml.Decoder
	if magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return app.GnuCashData{}, fmt.Errorf("gunzip xml: %w", err)
		}
		defer func() { _ = gz.Close() }()
		dec = xml.NewDecoder(gz)
	} else {
		dec = xml.NewDecoder(f)
	}

	var doc xmlFile
	if err := dec.Decode(&doc); err != nil {
		return app.GnuCashData{}, fmt.Errorf("parse xml: %w", err)
	}
	if doc.Book.ID == "" {
		// No book — the caller treats a zero GnuCashData as a parse error.
		return app.GnuCashData{}, nil
	}

	commodities, cmdtyGUID, err := xmlCommodities(doc.Book.Commodities)
	if err != nil {
		return app.GnuCashData{}, err
	}
	accounts, rootGUID := xmlAccounts(doc.Book.Accounts, cmdtyGUID)

	// The template root exists in the XML only when there are scheduled
	// transactions. If present, parse it from the template-transactions section;
	// otherwise synthesise one so the book round-trips to SQLite with both roots.
	templateAccounts, templateRootGUID := xmlAccounts(doc.Book.TemplateTransactions.Accounts, cmdtyGUID)
	var templateRoot domain.Account
	if templateRootGUID == "" {
		templateRoot = domain.Account{GUID: newGUID(), Name: "Template Root", Type: domain.AccountRoot}
		templateRootGUID = templateRoot.GUID
		accounts = append(accounts, templateRoot)
	}
	// Template accounts (non-root) reference the synthesised/parsed template root.
	for _, ta := range templateAccounts {
		if ta.Type != domain.AccountRoot {
			accounts = append(accounts, ta)
		}
	}

	transactions, err := xmlTransactions(doc.Book.Transactions, cmdtyGUID)
	if err != nil {
		return app.GnuCashData{}, err
	}

	// Template transactions are used only to resolve scheduled splits; they are
	// NOT added to the real transaction list.
	templateTxns, err := xmlTransactions(doc.Book.TemplateTransactions.Transactions, cmdtyGUID)
	if err != nil {
		return app.GnuCashData{}, err
	}

	// Build a lookup of template transactions by GUID for split resolution.
	tmplTxByGUID := make(map[string]*domain.Transaction, len(templateTxns))
	for i := range templateTxns {
		tmplTxByGUID[templateTxns[i].GUID] = &templateTxns[i]
	}

	// Build a lookup: template account GUID → account (to detect marker splits).
	tmplAcctByGUID := make(map[string]struct{}, len(templateAccounts))
	for _, ta := range templateAccounts {
		tmplAcctByGUID[ta.GUID] = struct{}{}
	}

	scheds, err := xmlScheduledTransactions(doc.Book.ScheduledXactions, tmplTxByGUID, tmplAcctByGUID, doc.Book.ID)
	if err != nil {
		return app.GnuCashData{}, err
	}

	return app.GnuCashData{
		Book: domain.Book{
			GUID:             doc.Book.ID,
			RootAccountGUID:  rootGUID,
			RootTemplateGUID: templateRootGUID,
		},
		Commodities:           commodities,
		Accounts:              accounts,
		Transactions:          transactions,
		ScheduledTransactions: scheds,
	}, nil
}

// xmlCommodities turns the book's commodity definitions into domain commodities
// with synthesised GUIDs, and returns a map from "namespace:mnemonic" to that
// GUID so account/transaction references can be resolved. The "template"
// pseudo-commodity GnuCash uses for scheduled-transaction templates is skipped.
func xmlCommodities(in []xmlCommodity) ([]domain.Commodity, map[string]string, error) {
	guidByRef := make(map[string]string, len(in))
	var out []domain.Commodity
	for _, c := range in {
		if c.Space == "" || c.Space == "template" {
			continue
		}
		ref := c.Space + ":" + c.ID
		if _, seen := guidByRef[ref]; seen {
			continue
		}
		fraction := int64(100)
		if c.Fraction != "" {
			n, err := strconv.ParseInt(strings.TrimSpace(c.Fraction), 10, 64)
			if err != nil || n <= 0 {
				return nil, nil, fmt.Errorf("commodity %s: invalid fraction %q", ref, c.Fraction)
			}
			fraction = n
		}
		guid := newGUID()
		guidByRef[ref] = guid
		out = append(out, domain.Commodity{
			GUID:      guid,
			Namespace: c.Space,
			Mnemonic:  c.ID,
			Fullname:  c.Name,
			Fraction:  fraction,
		})
	}
	return out, guidByRef, nil
}

// xmlAccounts maps the parsed accounts into domain accounts and returns the GUID
// of the ROOT account (the book's real root). Account GUIDs are preserved from
// the file; commodity references are resolved through guidByRef.
func xmlAccounts(in []xmlAccount, guidByRef map[string]string) ([]domain.Account, string) {
	var (
		out      []domain.Account
		rootGUID string
	)
	for _, a := range in {
		acct := domain.Account{
			GUID:          a.ID,
			Name:          a.Name,
			Type:          domain.AccountType(a.Type),
			CommodityGUID: guidByRef[a.Commodity.Space+":"+a.Commodity.ID],
			ParentGUID:    a.Parent,
			Code:          a.Code,
			Description:   a.Description,
		}
		for _, s := range a.Slots {
			switch s.Key {
			case "placeholder":
				acct.Placeholder = s.Value == "true"
			case "hidden":
				acct.Hidden = s.Value == "true"
			}
		}
		if acct.Type == domain.AccountRoot && rootGUID == "" {
			rootGUID = acct.GUID
		}
		out = append(out, acct)
	}
	return out, rootGUID
}

// xmlTransactions maps the parsed transactions and their splits into domain
// types, resolving the transaction currency through guidByRef and parsing each
// money amount from its GnuCash "num/denom" rational string.
func xmlTransactions(in []xmlTransaction, guidByRef map[string]string) ([]domain.Transaction, error) {
	var out []domain.Transaction
	for _, t := range in {
		posted, err := parseGncXMLTime(t.DatePosted)
		if err != nil {
			return nil, fmt.Errorf("transaction %s post date: %w", t.ID, err)
		}
		entered, err := parseGncXMLTime(t.DateEntered)
		if err != nil {
			return nil, fmt.Errorf("transaction %s enter date: %w", t.ID, err)
		}
		tx := domain.Transaction{
			GUID:         t.ID,
			CurrencyGUID: guidByRef[t.Currency.Space+":"+t.Currency.ID],
			Num:          t.Num,
			PostDate:     posted,
			EnterDate:    entered,
			Description:  t.Description,
		}
		for _, s := range t.Splits {
			value, err := parseGncRational(s.Value)
			if err != nil {
				return nil, fmt.Errorf("split %s value: %w", s.ID, err)
			}
			quantity, err := parseGncRational(s.Quantity)
			if err != nil {
				return nil, fmt.Errorf("split %s quantity: %w", s.ID, err)
			}
			split := domain.Split{
				GUID:        s.ID,
				AccountGUID: s.Account,
				Memo:        s.Memo,
				Action:      s.Action,
				Value:       value,
				Quantity:    quantity,
			}
			if r := strings.TrimSpace(s.Reconciled); r != "" {
				split.Reconcile = domain.ReconcileState([]rune(r)[0])
			}
			tx.Splits = append(tx.Splits, split)
		}
		out = append(out, tx)
	}
	return out, nil
}

// parseGncRational parses a GnuCash money amount, written as a "num/denom"
// rational (e.g. "500000/100") or a bare integer.
func parseGncRational(s string) (domain.GncNumeric, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return domain.Zero(), nil
	}
	num, denom := s, "1"
	if n, d, ok := strings.Cut(s, "/"); ok {
		num, denom = n, d
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return domain.GncNumeric{}, fmt.Errorf("invalid amount %q", s)
	}
	d, err := strconv.ParseInt(denom, 10, 64)
	if err != nil {
		return domain.GncNumeric{}, fmt.Errorf("invalid amount %q", s)
	}
	return domain.FromNumDenom(n, d)
}

// parseGncXMLTime parses a GnuCash XML timestamp, returning the zero time for an
// empty value. Times are normalised to UTC.
func parseGncXMLTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range gncXMLTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", s)
}

// parseGncXMLDate parses a bare GnuCash <gdate> value ("YYYY-MM-DD"), returning
// the zero time for an absent or empty value.
func parseGncXMLDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t
}

// xmlScheduledTransactions converts parsed <gnc:schedxaction> elements into
// domain.ScheduledTransaction values. Template splits are resolved by looking
// up each schedxaction's templ-acct in the pre-parsed template transactions.
func xmlScheduledTransactions(
	in []xmlSchedXaction,
	tmplTxByGUID map[string]*domain.Transaction,
	tmplAcctByGUID map[string]struct{},
	bookID string,
) ([]domain.ScheduledTransaction, error) {
	if len(in) == 0 {
		return nil, nil
	}

	// Build a reverse index: template account GUID → template transaction GUIDs.
	// (A template account appears as the account_guid on the marker split of each
	// template transaction; the real schedule splits are the other splits.)
	markerToTx := make(map[string][]string) // templateActGUID → []txGUID
	for _, tx := range tmplTxByGUID {
		for _, sp := range tx.Splits {
			if _, isTemplAcct := tmplAcctByGUID[sp.AccountGUID]; isTemplAcct {
				markerToTx[sp.AccountGUID] = append(markerToTx[sp.AccountGUID], tx.GUID)
				break
			}
		}
	}

	var out []domain.ScheduledTransaction
	for _, sx := range in {
		s := domain.ScheduledTransaction{
			GUID:           sx.ID,
			BookGUID:       bookID,
			Name:           sx.Name,
			Enabled:        strings.EqualFold(strings.TrimSpace(sx.Enabled), "y"),
			StartDate:      parseGncXMLDate(sx.Start),
			EndDate:        parseGncXMLDate(sx.End),
			LastPostedDate: parseGncXMLDate(sx.Last),
			Every:          1,
			Period:         domain.PeriodMonthly,
		}

		// Parse the first recurrence rule.
		if len(sx.Schedule) > 0 {
			r := sx.Schedule[0]
			if n, err := strconv.Atoi(strings.TrimSpace(r.Mult)); err == nil && n > 0 {
				s.Every = n
			}
			s.Period = mapGncPeriod(strings.TrimSpace(r.PeriodType))
		}

		// Resolve template splits via templ-acct.
		for _, txGUID := range markerToTx[sx.TemplAcct] {
			tx, ok := tmplTxByGUID[txGUID]
			if !ok {
				continue
			}
			// Use the transaction's currency for the schedule.
			if s.CurrencyGUID == "" {
				s.CurrencyGUID = tx.CurrencyGUID
			}
			// Collect non-marker splits as the scheduled split templates.
			for _, sp := range tx.Splits {
				if _, isTemplAcct := tmplAcctByGUID[sp.AccountGUID]; isTemplAcct {
					continue // skip marker split
				}
				s.Splits = append(s.Splits, domain.ScheduledSplit{
					GUID:        sp.GUID,
					AccountGUID: sp.AccountGUID,
					Memo:        sp.Memo,
					Value:       sp.Value,
				})
			}
		}

		out = append(out, s)
	}
	return out, nil
}

// newGUID returns a fresh 32-character hex GUID in GnuCash's identifier format.
// The XML backend does not store commodity GUIDs, so the importer mints them.
func newGUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is catastrophic and not something an import can
		// meaningfully recover from.
		panic(fmt.Sprintf("gnucash: generate guid: %v", err))
	}
	return hex.EncodeToString(b[:])
}
