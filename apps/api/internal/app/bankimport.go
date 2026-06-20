package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// StatementTxn is one parsed line from a bank-statement file (OFX/QIF),
// expressed in domain money. Amount is signed (negative = money out of the
// account). FITID is the bank's stable id when the format carries one (OFX);
// QIF has none, so duplicate detection falls back to a content hash.
type StatementTxn struct {
	Date   time.Time
	Amount domain.GncNumeric
	Memo   string
	FITID  string
}

// StatementReader parses a bank-statement file into transactions. One
// implementation per format lives in internal/infra/bankimport.
type StatementReader interface {
	Read(r io.Reader) ([]StatementTxn, error)
}

// BankImportRepository resolves the account being imported into and backs
// duplicate detection.
type BankImportRepository interface {
	// AccountCommodity returns the commodity an account is denominated in, or
	// ErrAccountNotFound if the account is unknown.
	AccountCommodity(ctx context.Context, accountGUID string) (AccountCommodityInfo, error)
	// FindOrCreateImbalanceAccount returns the GUID of the Imbalance-MNEMONIC
	// account in the book that anchorAccountGUID belongs to, creating it when
	// absent. It is the offsetting account for uncategorised imported lines.
	FindOrCreateImbalanceAccount(ctx context.Context, anchorAccountGUID string, currency domain.Commodity) (string, error)
	// ExistingImportRefs returns the set of import refs (transaction num values)
	// already posted to the account, so re-imported lines can be skipped.
	ExistingImportRefs(ctx context.Context, accountGUID string) (map[string]struct{}, error)
}

// BankImportResult summarises a completed statement import.
type BankImportResult struct {
	AccountGUID string
	Imported    int
	Skipped     int
}

// BankImportService imports OFX/QIF bank statements into an existing account.
// Each statement line becomes a balanced two-split transaction posting the line
// to the chosen account against the book's Imbalance-CUR account (the user
// recategorises later). Lines already present — matched by import ref — are
// skipped, so re-importing an overlapping statement is safe.
type BankImportService struct {
	posting *PostingService
	repo    BankImportRepository
	readers map[string]StatementReader
}

// NewBankImportService wires the posting path, repository, and one reader per
// lower-cased format key (e.g. "ofx", "qif").
func NewBankImportService(posting *PostingService, repo BankImportRepository, readers map[string]StatementReader) *BankImportService {
	return &BankImportService{posting: posting, repo: repo, readers: readers}
}

// Import parses r as the named format and posts its transactions into
// accountGUID, offsetting each to the book's Imbalance account. Duplicates (by
// import ref) are skipped. It returns counts of imported and skipped lines, or
// ErrImportParse / ErrInvalidInput / ErrAccountNotFound.
func (s *BankImportService) Import(ctx context.Context, accountGUID, format string, r io.Reader, actor AuditActor) (BankImportResult, error) {
	reader, ok := s.readers[strings.ToLower(strings.TrimSpace(format))]
	if !ok {
		return BankImportResult{}, fmt.Errorf("%w: unsupported statement format %q", ErrInvalidInput, format)
	}

	info, err := s.repo.AccountCommodity(ctx, accountGUID)
	if err != nil {
		return BankImportResult{}, err
	}
	currency := info.Commodity
	if currency.Namespace != domain.NamespaceCurrency {
		return BankImportResult{}, fmt.Errorf("%w: statements can only be imported into a currency account", ErrInvalidInput)
	}

	txns, err := reader.Read(r)
	if err != nil {
		return BankImportResult{}, fmt.Errorf("%w: %v", ErrImportParse, err)
	}

	imbalanceGUID, err := s.repo.FindOrCreateImbalanceAccount(ctx, accountGUID, currency)
	if err != nil {
		return BankImportResult{}, err
	}
	seen, err := s.repo.ExistingImportRefs(ctx, accountGUID)
	if err != nil {
		return BankImportResult{}, err
	}

	result := BankImportResult{AccountGUID: accountGUID}
	for _, t := range txns {
		ref := importRef(t)
		if _, dup := seen[ref]; dup {
			result.Skipped++
			continue
		}
		tx := domain.Transaction{
			CurrencyGUID: currency.GUID,
			Num:          ref,
			PostDate:     t.Date,
			Description:  t.Memo,
			Splits: []domain.Split{
				{AccountGUID: accountGUID, Value: t.Amount, Quantity: t.Amount},
				{AccountGUID: imbalanceGUID, Value: t.Amount.Neg(), Quantity: t.Amount.Neg()},
			},
		}
		if _, err := s.posting.Post(ctx, tx, actor); err != nil {
			return BankImportResult{}, err
		}
		// Guard against duplicates within the same file too.
		seen[ref] = struct{}{}
		result.Imported++
	}
	return result, nil
}

// importRef is the duplicate-detection key for a statement line: the bank's
// FITID when present (OFX), else a stable short hash of date+amount+memo (QIF).
// It is stored in the transaction's Num field.
func importRef(t StatementTxn) string {
	if id := strings.TrimSpace(t.FITID); id != "" {
		return id
	}
	sum := sha1.Sum([]byte(t.Date.Format("2006-01-02") + "|" + t.Amount.String() + "|" + t.Memo))
	return "auto:" + hex.EncodeToString(sum[:6])
}
