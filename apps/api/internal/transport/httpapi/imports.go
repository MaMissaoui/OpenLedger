package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// maxImportUpload caps an uploaded GnuCash file at 64 MiB, which comfortably
// covers personal and small-business books while bounding memory/disk use.
const maxImportUpload = 64 << 20

// handleImportGnuCash imports an uploaded GnuCash SQLite file as a new book
// owned by the authenticated user. The file arrives as multipart/form-data
// under the "file" field; it is staged to a temp file (the SQLite driver reads
// from a path) and removed once the import completes.
func (s *Server) handleImportGnuCash(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportUpload)

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart upload with a 'file' field")
		return
	}
	defer func() { _ = file.Close() }()

	tmp, err := os.CreateTemp("", "openledger-gnucash-*.sqlite")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not stage upload")
		return
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := io.Copy(tmp, file); err != nil {
		_ = tmp.Close()
		writeError(w, http.StatusInternalServerError, "could not read upload")
		return
	}
	if err := tmp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not stage upload")
		return
	}

	userID := actorFromContext(r.Context()).UserID
	result, err := s.importer.ImportSQLite(r.Context(), tmp.Name(), userID)
	switch {
	case errors.Is(err, app.ErrImportParse):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, domain.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, app.ErrImportConflict):
		writeError(w, http.StatusConflict, "this book has already been imported")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not import file")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"bookGuid":     result.BookGUID,
		"commodities":  result.Commodities,
		"accounts":     result.Accounts,
		"transactions": result.Transactions,
	})
}
