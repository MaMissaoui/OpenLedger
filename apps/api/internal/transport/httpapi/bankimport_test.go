package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
	"github.com/openledger/openledger/apps/api/internal/infra/bankimport"
)

// bankFake satisfies app.BankImportRepository and app.TransactionRepository
// (statement lines post through PostingService).
type bankFake struct {
	accountCommodities map[string]app.AccountCommodityInfo
	importRefs         map[string]struct{}
}

func (f *bankFake) InsertTransaction(context.Context, domain.Transaction, app.AuditActor) error {
	return nil
}
func (f *bankFake) UpdateTransaction(context.Context, domain.Transaction, app.AuditActor) error {
	return nil
}
func (f *bankFake) DeleteTransaction(context.Context, string, app.AuditActor) error { return nil }
func (f *bankFake) TransactionAccountGUIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (f *bankFake) AccountCommodity(_ context.Context, accountGUID string) (app.AccountCommodityInfo, error) {
	if info, ok := f.accountCommodities[accountGUID]; ok {
		return info, nil
	}
	return app.AccountCommodityInfo{}, nil
}

func (f *bankFake) FindOrCreateImbalanceAccount(_ context.Context, _ string, currency domain.Commodity) (string, error) {
	return "imbalance-" + currency.GUID, nil
}

func (f *bankFake) ExistingImportRefs(_ context.Context, _ string) (map[string]struct{}, error) {
	if f.importRefs == nil {
		return map[string]struct{}{}, nil
	}
	return f.importRefs, nil
}

func bankServer(f *bankFake, authz *app.AuthzService) http.Handler {
	posting := app.NewPostingService(f)
	bank := app.NewBankImportService(posting, f, map[string]app.StatementReader{
		"ofx": bankimport.OFX{},
		"qif": bankimport.QIF{},
	})
	return authedServer(Services{BankImport: bank, Authz: authz})
}

const ofxStatement = `OFXHEADER:100
<OFX><BANKTRANLIST>
<STMTTRN><DTPOSTED>20240619<TRNAMT>-50.00<FITID>F1<NAME>SAFEWAY</STMTTRN>
<STMTTRN><DTPOSTED>20240620<TRNAMT>1000.00<FITID>F2<NAME>EMPLOYER</STMTTRN>
</BANKTRANLIST></OFX>`

func uploadStatement(h http.Handler, path, format, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if format != "" {
		_ = mw.WriteField("format", format)
	}
	part, _ := mw.CreateFormFile("file", "statement.ofx")
	_, _ = part.Write([]byte(content))
	_ = mw.Close()

	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, path, &body))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	h.ServeHTTP(rec, req)
	return rec
}

func bankImportRepo() *bankFake {
	return &bankFake{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"checking": {Commodity: domain.Commodity{GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"}},
		},
	}
}

func TestImportBankStatement(t *testing.T) {
	repo := bankImportRepo()
	rec := uploadStatement(bankServer(repo, nil), "/api/v1/accounts/checking/import-bank", "ofx", ofxStatement)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Imported, Skipped int
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Imported != 2 || got.Skipped != 0 {
		t.Errorf("result = %+v, want imported 2 skipped 0", got)
	}
}

func TestImportBankStatementSniffsFormat(t *testing.T) {
	// No "format" field: the handler sniffs OFX from the OFXHEADER marker.
	rec := uploadStatement(bankServer(bankImportRepo(), nil), "/api/v1/accounts/checking/import-bank", "", ofxStatement)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
}

func TestImportBankStatementUnrecognisedReturns400(t *testing.T) {
	rec := uploadStatement(bankServer(bankImportRepo(), nil), "/api/v1/accounts/checking/import-bank", "", "just some text")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestImportBankStatementForbidden(t *testing.T) {
	repo := bankImportRepo()
	rec := uploadStatement(bankServer(repo, app.NewAuthzService(&authStub{noMembership: true})), "/api/v1/accounts/checking/import-bank", "ofx", ofxStatement)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

const csvStatement = "Date,Amount,Description\n2024-06-19,-50.00,Coffee\n2024-06-20,1000.00,Payroll\n"

// uploadCSV posts a CSV statement with format=csv and a mapping JSON field.
func uploadCSV(h http.Handler, path, content, mapping string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("format", "csv")
	if mapping != "" {
		_ = mw.WriteField("mapping", mapping)
	}
	part, _ := mw.CreateFormFile("file", "statement.csv")
	_, _ = part.Write([]byte(content))
	_ = mw.Close()

	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, path, &body))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	h.ServeHTTP(rec, req)
	return rec
}

func TestPreviewBankCsv(t *testing.T) {
	rec := uploadStatement(bankServer(bankImportRepo(), nil),
		"/api/v1/accounts/checking/import-bank/preview", "", csvStatement)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Rows      [][]string
		TotalRows int
		Columns   int
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.TotalRows != 3 || got.Columns != 3 || got.Rows[0][0] != "Date" {
		t.Errorf("preview = %+v, want 3 rows / 3 cols with a Date header", got)
	}
}

func TestImportBankCsv(t *testing.T) {
	mapping := `{"hasHeader":true,"dateCol":0,"dateFormat":"2006-01-02","amountCol":1,"descCols":[2]}`
	rec := uploadCSV(bankServer(bankImportRepo(), nil), "/api/v1/accounts/checking/import-bank", csvStatement, mapping)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var got struct{ Imported, Skipped int }
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Imported != 2 {
		t.Errorf("imported = %d, want 2", got.Imported)
	}
}

func TestImportBankCsvMissingAmountMappingReturns400(t *testing.T) {
	mapping := `{"hasHeader":true,"dateCol":0}` // no amount/debit/credit columns
	rec := uploadCSV(bankServer(bankImportRepo(), nil), "/api/v1/accounts/checking/import-bank", csvStatement, mapping)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}
