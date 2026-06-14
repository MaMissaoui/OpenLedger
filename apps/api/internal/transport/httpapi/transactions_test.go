package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeRepo implements all repository ports in memory so the HTTP layer can be
// tested without a database.
type fakeRepo struct {
	inserted     *domain.Transaction
	updated      *domain.Transaction
	deletedGUID  string
	txAccounts   []string            // returned by TransactionAccountGUIDs
	txNotFound   bool                // make TransactionAccountGUIDs / GetTransaction return ErrTransactionNotFound
	gotTx        *domain.Transaction // returned by GetTransaction
	exists       bool
	registerRows []app.RegisterEntry
	registerTot  int64

	// Structure side.
	commodities  []domain.Commodity
	prices       []domain.Price
	books        []domain.Book
	accounts     []domain.Account
	bookRoot     string // root returned by BookRootAccount
	bookNotFound bool   // make BookRootAccount return ErrBookNotFound
	listAccounts []app.AccountWithBalance
	reportRows   []app.AccountWithBalance // returned by AccountBalances

	// Portfolio side. holdings is returned by SecurityHoldings; latestPrices maps
	// a commodity GUID to its most recent quote (absent → no quote).
	holdings     []app.HoldingBalance
	latestPrices map[string]domain.Price

	// Trade side. accountCommodities maps an account GUID to its commodity info;
	// openLots drives FIFO matching; createdLots/closedLots capture lot lifecycle;
	// capGainsAccount is the find-or-created gains account; realizedGainRows backs
	// the capital-gains report.
	accountCommodities map[string]app.AccountCommodityInfo
	openLots           []domain.OpenLot
	createdLots        []string
	closedLots         []string
	capGainsAccount    string
	realizedGainRows   []app.RealizedGainRow

	// Import side. readerData/readerErr drive the fake GnuCashReader;
	// importedData captures what ImportBook persisted, and importErr forces a
	// repository failure.
	readerData   app.GnuCashData
	readerErr    error
	importedData *app.GnuCashData
	importErr    error

	// Export side. loadBookData/loadBookErr drive the fake ExportRepository.
	loadBookData app.GnuCashData
	loadBookErr  error

	// Reconcile side. splitUnknown forces a 404; the reconciled* fields capture
	// what SetSplitReconcile was asked to write.
	splitUnknown    bool
	reconciledSplit string
	reconciledState domain.ReconcileState
	reconciledDate  *time.Time

	// Provision side.
	provisionedUserID string // returned by FindOrCreateLDAPUser (default "user-1")

	// Authz side. The zero value grants owner access so most tests don't set it
	// up; set role to test a specific permission level, or noMembership for 403.
	noMembership    bool     // UserBookRole reports no membership row
	role            app.Role // membership role (defaults to owner when empty)
	accountUnknown  bool     // BookGUIDForAccount returns ErrAccountNotFound
	accountBookGUID string   // book returned by BookGUIDForAccount (default "book-1")
}

func (f *fakeRepo) FindOrCreateLDAPUser(_ context.Context, _, _ string) (string, error) {
	if f.provisionedUserID != "" {
		return f.provisionedUserID, nil
	}
	return "user-1", nil
}

func (f *fakeRepo) UserBookRole(_ context.Context, _, _ string) (app.Role, bool, error) {
	if f.noMembership {
		return "", false, nil
	}
	if f.role != "" {
		return f.role, true, nil
	}
	return app.RoleOwner, true, nil
}

func (f *fakeRepo) BookGUIDForAccount(_ context.Context, _ string) (string, error) {
	if f.accountUnknown {
		return "", app.ErrAccountNotFound
	}
	if f.accountBookGUID != "" {
		return f.accountBookGUID, nil
	}
	return "book-1", nil
}

func (f *fakeRepo) AccountGUIDForSplit(_ context.Context, _ string) (string, error) {
	if f.splitUnknown {
		return "", app.ErrSplitNotFound
	}
	return "checking", nil
}

func (f *fakeRepo) SetSplitReconcile(_ context.Context, splitGUID string, state domain.ReconcileState, date *time.Time) error {
	f.reconciledSplit = splitGUID
	f.reconciledState = state
	f.reconciledDate = date
	return nil
}

func (f *fakeRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	cp := tx
	f.inserted = &cp
	return nil
}

func (f *fakeRepo) UpdateTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	cp := tx
	f.updated = &cp
	return nil
}

func (f *fakeRepo) DeleteTransaction(_ context.Context, guid string, _ app.AuditActor) error {
	f.deletedGUID = guid
	return nil
}

func (f *fakeRepo) TransactionAccountGUIDs(_ context.Context, _ string) ([]string, error) {
	if f.txNotFound {
		return nil, app.ErrTransactionNotFound
	}
	if f.txAccounts != nil {
		return f.txAccounts, nil
	}
	return []string{"checking", "groceries"}, nil
}

func (f *fakeRepo) AccountExists(_ context.Context, _ string) (bool, error) {
	return f.exists, nil
}

func (f *fakeRepo) ListAccountRegister(_ context.Context, _ string, _, _ int) ([]app.RegisterEntry, int64, error) {
	return f.registerRows, f.registerTot, nil
}

func (f *fakeRepo) GetTransaction(_ context.Context, guid string) (domain.Transaction, error) {
	if f.txNotFound || f.gotTx == nil {
		return domain.Transaction{}, app.ErrTransactionNotFound
	}
	tx := *f.gotTx
	tx.GUID = guid
	return tx, nil
}

func (f *fakeRepo) InsertCommodity(_ context.Context, c domain.Commodity) error {
	f.commodities = append(f.commodities, c)
	return nil
}

func (f *fakeRepo) ListCommodities(_ context.Context) ([]domain.Commodity, error) {
	return f.commodities, nil
}

func (f *fakeRepo) InsertPrice(_ context.Context, p domain.Price) error {
	f.prices = append(f.prices, p)
	return nil
}

func (f *fakeRepo) ListPricesByCommodity(_ context.Context, _ string) ([]domain.Price, error) {
	return f.prices, nil
}

func (f *fakeRepo) InsertBook(_ context.Context, b domain.Book, _, _ domain.Account, _ string) error {
	f.books = append(f.books, b)
	return nil
}

func (f *fakeRepo) ListBooksForUser(_ context.Context, _ string) ([]domain.Book, error) {
	return f.books, nil
}

func (f *fakeRepo) InsertAccount(_ context.Context, a domain.Account) error {
	f.accounts = append(f.accounts, a)
	return nil
}

func (f *fakeRepo) BookRootAccount(_ context.Context, _ string) (string, error) {
	if f.bookNotFound {
		return "", app.ErrBookNotFound
	}
	return f.bookRoot, nil
}

func (f *fakeRepo) SecurityHoldings(_ context.Context, _ string) ([]app.HoldingBalance, error) {
	return f.holdings, nil
}

func (f *fakeRepo) AccountCommodity(_ context.Context, accountGUID string) (app.AccountCommodityInfo, error) {
	if info, ok := f.accountCommodities[accountGUID]; ok {
		return info, nil
	}
	if f.accountUnknown {
		return app.AccountCommodityInfo{}, app.ErrAccountNotFound
	}
	return app.AccountCommodityInfo{}, nil
}

func (f *fakeRepo) CreateLot(_ context.Context, lotGUID, _ string) error {
	f.createdLots = append(f.createdLots, lotGUID)
	return nil
}

func (f *fakeRepo) OpenLotsForAccount(_ context.Context, _ string) ([]domain.OpenLot, error) {
	return f.openLots, nil
}

func (f *fakeRepo) SetLotClosed(_ context.Context, lotGUID string) error {
	f.closedLots = append(f.closedLots, lotGUID)
	return nil
}

func (f *fakeRepo) FindOrCreateCapitalGainsAccount(_ context.Context, _ string, _ domain.Commodity) (string, error) {
	if f.capGainsAccount == "" {
		f.capGainsAccount = "capgains"
	}
	return f.capGainsAccount, nil
}

func (f *fakeRepo) RealizedGainRows(_ context.Context, _ string, _, _ *time.Time) ([]app.RealizedGainRow, error) {
	return f.realizedGainRows, nil
}

func (f *fakeRepo) LatestPrice(_ context.Context, commodityGUID string) (domain.Price, bool, error) {
	p, ok := f.latestPrices[commodityGUID]
	return p, ok, nil
}

func (f *fakeRepo) ListAccountsUnderRoot(_ context.Context, _ string) ([]app.AccountWithBalance, error) {
	return f.listAccounts, nil
}

func (f *fakeRepo) AccountBalances(_ context.Context, _ string, _, _ *time.Time) ([]app.AccountWithBalance, error) {
	return f.reportRows, nil
}

// ReadGnuCashSQLite is the fake GnuCashReader: it ignores the path and returns
// the canned reader data/error the test configured.
func (f *fakeRepo) ReadGnuCashSQLite(_ context.Context, _ string) (app.GnuCashData, error) {
	return f.readerData, f.readerErr
}

func (f *fakeRepo) ReadGnuCashXML(_ context.Context, _ string) (app.GnuCashData, error) {
	return f.readerData, f.readerErr
}

// LoadBook is the fake ExportRepository: it returns the canned book data/error
// the test configured.
func (f *fakeRepo) LoadBook(_ context.Context, _ string) (app.GnuCashData, error) {
	return f.loadBookData, f.loadBookErr
}

// fakeWriter is a stub GnuCashWriter that records what it was asked to write and
// creates a placeholder file at the path so the handler can stream it back. The
// file content carries the format token so handler dispatch can be asserted.
type fakeWriter struct {
	wrote  *app.GnuCashData
	format string
}

func (fw *fakeWriter) WriteGnuCashSQLite(_ context.Context, path string, data app.GnuCashData) error {
	return fw.record("sqlite", path, data)
}

func (fw *fakeWriter) WriteGnuCashXML(_ context.Context, path string, data app.GnuCashData) error {
	return fw.record("xml", path, data)
}

func (fw *fakeWriter) record(format, path string, data app.GnuCashData) error {
	cp := data
	fw.wrote = &cp
	fw.format = format
	return os.WriteFile(path, []byte("gnucash-export:"+format), 0o600)
}

func (f *fakeRepo) ImportBook(_ context.Context, data app.GnuCashData, _ string) error {
	if f.importErr != nil {
		return f.importErr
	}
	cp := data
	f.importedData = &cp
	return nil
}

func newTestServer(repo *fakeRepo) http.Handler {
	posting := app.NewPostingService(repo)
	return NewServer(
		posting,
		app.NewLedgerService(repo),
		app.NewStructureService(repo),
		app.NewPriceService(repo),
		app.NewReportService(repo),
		app.NewProvisionService(repo),
		app.NewAuthzService(repo),
		app.NewImportService(repo, repo),
		app.NewExportService(repo, &fakeWriter{}),
		app.NewReconcileService(repo),
		app.NewPortfolioService(repo),
		app.NewTradeService(repo, posting),
		app.NewCapitalGainsService(repo),
	).Routes()
}

// withAuth sets the Authelia-forwarded identity headers so requests reach the
// protected /api/v1 handlers. In production Traefik adds these after Authelia
// verifies the session; in tests we set them directly.
func withAuth(req *http.Request) *http.Request {
	req.Header.Set("Remote-User", "test-user")
	req.Header.Set("Remote-Email", "test@example.com")
	return req
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/transactions", strings.NewReader(body)))
	h.ServeHTTP(rec, req)
	return rec
}

func TestPostBalancedTransaction(t *testing.T) {
	repo := &fakeRepo{}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","description":"groceries","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-5000,"denom":100},"quantity":{"num":-5000,"denom":100}}
		]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted == nil {
		t.Fatal("transaction was not persisted")
	}
	if repo.inserted.GUID == "" {
		t.Error("expected a generated transaction GUID")
	}
	for _, s := range repo.inserted.Splits {
		if s.GUID == "" {
			t.Error("expected a generated split GUID")
		}
	}
}

func TestPostUnbalancedTransactionReturns422(t *testing.T) {
	repo := &fakeRepo{}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-4900,"denom":100},"quantity":{"num":-4900,"denom":100}}
		]}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted != nil {
		t.Error("unbalanced transaction must not be persisted")
	}
}

func TestPostInvalidJSONReturns400(t *testing.T) {
	rec := post(newTestServer(&fakeRepo{}), `{ not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPostSingleSplitReturns400(t *testing.T) {
	rec := post(newTestServer(&fakeRepo{}), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}}
		]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (request shape); body = %s", rec.Code, rec.Body.String())
	}
}
