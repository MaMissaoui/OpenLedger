package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
	"github.com/openledger/openledger/apps/api/internal/infra/bankimport"
)

// maxStatementUpload caps a bank-statement upload at 16 MiB — far more than a
// realistic OFX/QIF statement while bounding memory use.
const maxStatementUpload = 16 << 20

// handleImportBankStatement imports an uploaded OFX/QIF statement into the
// account named in the path. Each line is posted against the book's Imbalance
// account; duplicates (by import ref) are skipped. The format comes from the
// "format" form field or is sniffed from the file's first bytes.
func (s *Server) handleImportBankStatement(w http.ResponseWriter, r *http.Request) {
	accountGUID := r.PathValue("id")
	if !s.authorizeAccount(w, r, accountGUID, app.AccessWrite) {
		return
	}
	if s.bankImport == nil {
		writeError(w, http.StatusServiceUnavailable, "bank-statement import is not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxStatementUpload)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart upload with a 'file' field")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read upload")
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.FormValue("format")))
	if format == "" {
		format = sniffStatementFormat(data)
	}

	actor := app.AuditActor{UserID: actorFromContext(r.Context()).UserID}

	// CSV needs a per-request column mapping (built by the wizard); other formats
	// use the service's static readers.
	var result app.BankImportResult
	if format == "csv" {
		mapping, ok := parseCSVMapping(w, r.FormValue("mapping"))
		if !ok {
			return
		}
		result, err = s.bankImport.ImportFrom(r.Context(), accountGUID,
			bankimport.CSV{Mapping: mapping}, bytes.NewReader(data), actor)
	} else {
		result, err = s.bankImport.Import(r.Context(), accountGUID, format, bytes.NewReader(data), actor)
	}
	switch {
	case errors.Is(err, app.ErrImportParse):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, app.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
		return
	case errors.Is(err, domain.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if writeStructureError(w, err) {
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"accountGuid": result.AccountGUID,
		"imported":    result.Imported,
		"skipped":     result.Skipped,
	})
}

// sniffStatementFormat guesses the statement format from the leading bytes: QIF
// begins with a "!Type:" header; OFX carries an "OFXHEADER" marker or an <OFX>
// root. Returns "" when neither matches, so the service reports an unsupported
// format.
func sniffStatementFormat(data []byte) string {
	head := strings.ToUpper(strings.TrimLeft(string(data[:min(len(data), 512)]), "\xef\xbb\xbf \t\r\n"))
	switch {
	case strings.HasPrefix(head, "!TYPE"):
		return "qif"
	case strings.Contains(head, "OFXHEADER") || strings.HasPrefix(head, "<OFX"):
		return "ofx"
	default:
		return ""
	}
}

// handlePreviewBankCSV parses an uploaded CSV and returns its first rows so the
// web wizard can build a column mapping. It does not persist anything.
func (s *Server) handlePreviewBankCSV(w http.ResponseWriter, r *http.Request) {
	accountGUID := r.PathValue("id")
	if !s.authorizeAccount(w, r, accountGUID, app.AccessWrite) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxStatementUpload)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart upload with a 'file' field")
		return
	}
	defer func() { _ = file.Close() }()

	preview, err := bankimport.PreviewCSV(file, 11)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not parse CSV: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":      preview.Rows,
		"totalRows": preview.TotalRows,
		"columns":   preview.Columns,
	})
}

// csvMappingDTO is the column mapping the wizard posts alongside a CSV upload.
// AmountCol and DebitCol/CreditCol are pointers so "unset" is distinguishable
// from column 0.
type csvMappingDTO struct {
	HasHeader  bool   `json:"hasHeader"`
	DateCol    int    `json:"dateCol"`
	DateFormat string `json:"dateFormat"`
	DescCols   []int  `json:"descCols"`
	AmountCol  *int   `json:"amountCol"`
	DebitCol   *int   `json:"debitCol"`
	CreditCol  *int   `json:"creditCol"`
	Invert     bool   `json:"invert"`
}

// parseCSVMapping decodes and validates the wizard's mapping JSON. On a problem
// it writes a 400 and returns ok=false.
func parseCSVMapping(w http.ResponseWriter, raw string) (bankimport.CSVMapping, bool) {
	var dto csvMappingDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or missing 'mapping' for CSV import")
		return bankimport.CSVMapping{}, false
	}
	col := func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	}
	mapping := bankimport.CSVMapping{
		HasHeader:  dto.HasHeader,
		DateCol:    dto.DateCol,
		DateFormat: dto.DateFormat,
		DescCols:   dto.DescCols,
		AmountCol:  col(dto.AmountCol),
		DebitCol:   col(dto.DebitCol),
		CreditCol:  col(dto.CreditCol),
		Invert:     dto.Invert,
	}
	if mapping.AmountCol < 0 && mapping.DebitCol < 0 && mapping.CreditCol < 0 {
		writeError(w, http.StatusBadRequest, "CSV mapping needs an amount column or debit/credit columns")
		return bankimport.CSVMapping{}, false
	}
	return mapping, true
}
