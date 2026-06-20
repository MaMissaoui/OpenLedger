package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
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

	actor := actorFromContext(r.Context())
	result, err := s.bankImport.Import(r.Context(), accountGUID, format, bytes.NewReader(data),
		app.AuditActor{UserID: actor.UserID})
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
