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
	ID           string           `xml:"id"`
	Commodities  []xmlCommodity   `xml:"commodity"`
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

	// Synthesise the template root GnuCash keeps but the XML omits, so the book
	// round-trips back out to the SQLite backend with both roots present.
	templateRoot := domain.Account{GUID: newGUID(), Name: "Template Root", Type: domain.AccountRoot}
	accounts = append(accounts, templateRoot)

	transactions, err := xmlTransactions(doc.Book.Transactions, cmdtyGUID)
	if err != nil {
		return app.GnuCashData{}, err
	}

	return app.GnuCashData{
		Book: domain.Book{
			GUID:             doc.Book.ID,
			RootAccountGUID:  rootGUID,
			RootTemplateGUID: templateRoot.GUID,
		},
		Commodities:  commodities,
		Accounts:     accounts,
		Transactions: transactions,
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
