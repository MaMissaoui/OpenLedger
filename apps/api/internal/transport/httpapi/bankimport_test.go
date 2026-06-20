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
