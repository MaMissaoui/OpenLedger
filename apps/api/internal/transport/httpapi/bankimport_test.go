package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

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

func bankImportRepo() *fakeRepo {
	return &fakeRepo{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"checking": {Commodity: domain.Commodity{GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"}},
		},
	}
}

func TestImportBankStatement(t *testing.T) {
	repo := bankImportRepo()
	rec := uploadStatement(newTestServer(repo), "/api/v1/accounts/checking/import-bank", "ofx", ofxStatement)
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
	rec := uploadStatement(newTestServer(bankImportRepo()), "/api/v1/accounts/checking/import-bank", "", ofxStatement)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
}

func TestImportBankStatementUnrecognisedReturns400(t *testing.T) {
	rec := uploadStatement(newTestServer(bankImportRepo()), "/api/v1/accounts/checking/import-bank", "", "just some text")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestImportBankStatementForbidden(t *testing.T) {
	repo := bankImportRepo()
	repo.noMembership = true
	rec := uploadStatement(newTestServer(repo), "/api/v1/accounts/checking/import-bank", "ofx", ofxStatement)
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
	rec := uploadStatement(newTestServer(bankImportRepo()),
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
	rec := uploadCSV(newTestServer(bankImportRepo()), "/api/v1/accounts/checking/import-bank", csvStatement, mapping)
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
	rec := uploadCSV(newTestServer(bankImportRepo()), "/api/v1/accounts/checking/import-bank", csvStatement, mapping)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}
