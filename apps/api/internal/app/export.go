package app

import "context"

// GnuCashWriter writes a parsed book out as a GnuCash file. Implementations live
// in internal/infra/gnucash; defining the port here keeps the use-case
// independent of the on-disk format. SQLite is the higher-fidelity target; XML
// is the portable, human-readable alternative.
type GnuCashWriter interface {
	WriteGnuCashSQLite(ctx context.Context, path string, data GnuCashData) error
	WriteGnuCashXML(ctx context.Context, path string, data GnuCashData) error
}

// ExportRepository loads a whole book — its accounts (including roots), the
// commodities those accounts and transactions use, and every transaction with
// its splits — for export. It returns ErrBookNotFound for an unknown book.
type ExportRepository interface {
	LoadBook(ctx context.Context, bookGUID string) (GnuCashData, error)
}

// ExportService reads a book from the repository and writes it to a GnuCash
// file. It is the read counterpart of ImportService.
type ExportService struct {
	repo   ExportRepository
	writer GnuCashWriter
}

// NewExportService builds an ExportService from a repository and a writer.
func NewExportService(repo ExportRepository, writer GnuCashWriter) *ExportService {
	return &ExportService{repo: repo, writer: writer}
}

// ExportSQLite loads the book and writes it as a GnuCash SQLite database at path.
// It returns ErrBookNotFound if the book does not exist.
func (s *ExportService) ExportSQLite(ctx context.Context, bookGUID, path string) error {
	data, err := s.repo.LoadBook(ctx, bookGUID)
	if err != nil {
		return err
	}
	return s.writer.WriteGnuCashSQLite(ctx, path, data)
}

// ExportXML loads the book and writes it as a GnuCash XML document at path. It
// returns ErrBookNotFound if the book does not exist.
func (s *ExportService) ExportXML(ctx context.Context, bookGUID, path string) error {
	data, err := s.repo.LoadBook(ctx, bookGUID)
	if err != nil {
		return err
	}
	return s.writer.WriteGnuCashXML(ctx, path, data)
}
