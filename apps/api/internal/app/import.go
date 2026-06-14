package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ErrImportParse is returned when a GnuCash file cannot be read or is missing
// its book. Handlers map it to 400.
var ErrImportParse = errors.New("could not read GnuCash file")

// ErrImportConflict is returned when an import would collide with data already
// present (the book or its commodities already exist, e.g. a re-import of the
// same file). Handlers map it to 409.
var ErrImportConflict = errors.New("book already imported")

// GnuCashData is the parsed contents of one GnuCash file, expressed in domain
// types: a book together with all the commodities, accounts (including the root
// account[s]), and transactions it owns. It is the unit an import reads and
// persists.
type GnuCashData struct {
	Book         domain.Book
	Commodities  []domain.Commodity
	Accounts     []domain.Account
	Transactions []domain.Transaction
	Lots         []domain.Lot
}

// GnuCashReader reads a GnuCash file from disk into domain types. Both the
// SQLite and XML implementations live in internal/infra/gnucash; defining the
// port here keeps the use-case independent of the file format.
type GnuCashReader interface {
	ReadGnuCashSQLite(ctx context.Context, path string) (GnuCashData, error)
	ReadGnuCashXML(ctx context.Context, path string) (GnuCashData, error)
}

// ImportRepository persists a parsed GnuCash book atomically: every commodity,
// account, the book and its owner membership, and every transaction with its
// splits are written in one DB transaction so a partial import never lands.
type ImportRepository interface {
	ImportBook(ctx context.Context, data GnuCashData, ownerUserID string) error
}

// ImportResult summarises a completed import for the API response.
type ImportResult struct {
	BookGUID     string
	Commodities  int
	Accounts     int
	Transactions int
}

// ImportService reads a GnuCash file and persists it as a new book owned by the
// importing user. It re-checks the balance invariant on the way in rather than
// trusting the source file.
type ImportService struct {
	reader GnuCashReader
	repo   ImportRepository
}

// NewImportService builds an ImportService from a reader and a repository.
func NewImportService(reader GnuCashReader, repo ImportRepository) *ImportService {
	return &ImportService{reader: reader, repo: repo}
}

// ImportSQLite reads the GnuCash SQLite file at path and persists it as a new
// book owned by ownerUserID. It rejects the whole import (wrapping
// domain.ErrUnbalanced) if any transaction does not balance, so a malformed
// source file can never introduce an unbalanced transaction.
func (s *ImportService) ImportSQLite(ctx context.Context, path, ownerUserID string) (ImportResult, error) {
	data, err := s.reader.ReadGnuCashSQLite(ctx, path)
	return s.persist(ctx, data, err, ownerUserID)
}

// ImportXML reads the GnuCash XML file at path (optionally gzipped, GnuCash's
// default on-disk form) and persists it like ImportSQLite, applying the same
// balance re-validation.
func (s *ImportService) ImportXML(ctx context.Context, path, ownerUserID string) (ImportResult, error) {
	data, err := s.reader.ReadGnuCashXML(ctx, path)
	return s.persist(ctx, data, err, ownerUserID)
}

// persist validates and writes a freshly-read book: it maps a read error to
// ErrImportParse, rejects a book-less file, re-checks the balance invariant
// (wrapping domain.ErrUnbalanced) rather than trusting the source, then writes
// the book. It is the shared tail of every Import* method.
func (s *ImportService) persist(ctx context.Context, data GnuCashData, readErr error, ownerUserID string) (ImportResult, error) {
	if readErr != nil {
		return ImportResult{}, fmt.Errorf("%w: %w", ErrImportParse, readErr)
	}
	if data.Book.GUID == "" {
		return ImportResult{}, fmt.Errorf("%w: file contains no book", ErrImportParse)
	}

	var unbalanced []string
	for _, t := range data.Transactions {
		if err := t.ValidateBalanced(); err != nil {
			unbalanced = append(unbalanced, t.GUID)
		}
	}
	if len(unbalanced) > 0 {
		return ImportResult{}, fmt.Errorf("%w: %d transaction(s) in the file do not balance: %s",
			domain.ErrUnbalanced, len(unbalanced), summarise(unbalanced))
	}

	if err := s.repo.ImportBook(ctx, data, ownerUserID); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{
		BookGUID:     data.Book.GUID,
		Commodities:  len(data.Commodities),
		Accounts:     len(data.Accounts),
		Transactions: len(data.Transactions),
	}, nil
}

// summarise renders up to the first five GUIDs, with an ellipsis when more
// remain, for a compact error message.
func summarise(guids []string) string {
	const limit = 5
	if len(guids) <= limit {
		return strings.Join(guids, ", ")
	}
	return fmt.Sprintf("%s, … (%d more)", strings.Join(guids[:limit], ", "), len(guids)-limit)
}
