package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// sqliteMagic is the SQLite file header, so an upload sniffs as the SQLite
// backend. The fake reader ignores the rest of the bytes and returns canned
// data; the prefix only has to satisfy the handler's format detection.
var sqliteMagic = []byte("SQLite format 3\x00")

// uploadGnuCash posts a multipart form with a "file" part to the import
// endpoint. The leading bytes drive format detection; the fake reader returns
// canned data, so this exercises the handler's upload plumbing and status
// mapping.
func uploadGnuCash(h http.Handler, content []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "book.gnucash")
	_, _ = part.Write(content)
	_ = mw.Close()

	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/imports/gnucash", &body))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	h.ServeHTTP(rec, req)
	return rec
}

func importableData() app.GnuCashData {
	return app.GnuCashData{
		Book:        domain.Book{GUID: "book1", RootAccountGUID: "root1", RootTemplateGUID: "troot1"},
		Commodities: []domain.Commodity{{GUID: "usd", Namespace: "CURRENCY", Mnemonic: "USD", Fraction: 100}},
		Accounts: []domain.Account{
			{GUID: "root1", Type: domain.AccountRoot},
			{GUID: "chk", Type: domain.AccountBank, CommodityGUID: "usd", ParentGUID: "root1"},
			{GUID: "sal", Type: domain.AccountIncome, CommodityGUID: "usd", ParentGUID: "root1"},
		},
		Transactions: []domain.Transaction{{
			GUID: "tx1", CurrencyGUID: "usd",
			Splits: []domain.Split{
				{GUID: "s1", AccountGUID: "chk", Value: domain.MustFromNumDenom(5000, 1), Quantity: domain.MustFromNumDenom(5000, 1)},
				{GUID: "s2", AccountGUID: "sal", Value: domain.MustFromNumDenom(-5000, 1), Quantity: domain.MustFromNumDenom(-5000, 1)},
			},
		}},
	}
}

func TestImportGnuCash(t *testing.T) {
	repo := &fakeRepo{readerData: importableData()}
	rec := uploadGnuCash(newTestServer(repo), sqliteMagic)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.importedData == nil {
		t.Fatal("import was not persisted")
	}
	if repo.importedData.Book.GUID != "book1" {
		t.Errorf("persisted book = %+v", repo.importedData.Book)
	}
}

func TestImportGnuCashXML(t *testing.T) {
	repo := &fakeRepo{readerData: importableData()}
	// Content starting with '<' sniffs as XML, dispatching to ImportXML.
	rec := uploadGnuCash(newTestServer(repo), []byte("<?xml version=\"1.0\"?><gnc-v2/>"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.importedData == nil || repo.importedData.Book.GUID != "book1" {
		t.Errorf("XML import was not persisted: %+v", repo.importedData)
	}
}

func TestImportGnuCashUnrecognisedFormatReturns400(t *testing.T) {
	repo := &fakeRepo{readerData: importableData()}
	rec := uploadGnuCash(newTestServer(repo), []byte("just some bytes"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	if repo.importedData != nil {
		t.Error("unrecognised upload must not be persisted")
	}
}

func TestImportGnuCashUnbalancedReturns422(t *testing.T) {
	data := importableData()
	data.Transactions[0].Splits[1].Value = domain.MustFromNumDenom(-4000, 1) // breaks balance
	repo := &fakeRepo{readerData: data}

	rec := uploadGnuCash(newTestServer(repo), sqliteMagic)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	if repo.importedData != nil {
		t.Error("unbalanced import must not be persisted")
	}
}

func TestImportGnuCashConflictReturns409(t *testing.T) {
	repo := &fakeRepo{readerData: importableData(), importErr: app.ErrImportConflict}
	rec := uploadGnuCash(newTestServer(repo), sqliteMagic)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
}

func TestImportGnuCashMissingFileReturns400(t *testing.T) {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/imports/gnucash", nil))
	newTestServer(&fakeRepo{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}
