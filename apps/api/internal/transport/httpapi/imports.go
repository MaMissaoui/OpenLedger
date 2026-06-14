package httpapi

import (
	"bytes"
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

	format, err := sniffGnuCashFormat(tmp.Name())
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is not a recognised GnuCash SQLite or XML document")
		return
	}

	userID := actorFromContext(r.Context()).UserID
	var result app.ImportResult
	switch format {
	case formatSQLite:
		result, err = s.importer.ImportSQLite(r.Context(), tmp.Name(), userID)
	case formatXML:
		result, err = s.importer.ImportXML(r.Context(), tmp.Name(), userID)
	}
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

// GnuCash file formats the import endpoint accepts.
const (
	formatSQLite = "sqlite"
	formatXML    = "xml"
)

// sniffGnuCashFormat inspects the leading bytes of an uploaded file to decide
// whether it is the SQLite backend (magic "SQLite format 3"), or the XML format
// — either gzipped (GnuCash's default, magic 0x1f 0x8b) or plain ('<'). It
// errors when the bytes match neither, so an unrelated upload is rejected before
// any parsing.
func sniffGnuCashFormat(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, 16)
	n, err := io.ReadFull(f, magic)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	magic = magic[:n]

	switch {
	case bytes.HasPrefix(magic, []byte("SQLite format 3")):
		return formatSQLite, nil
	case len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		return formatXML, nil // gzipped — the XML reader gunzips transparently
	case bytes.HasPrefix(bytes.TrimLeft(magic, "\xef\xbb\xbf \t\r\n"), []byte("<")):
		return formatXML, nil
	default:
		return "", errors.New("unrecognised file format")
	}
}
