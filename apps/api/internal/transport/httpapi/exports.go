package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// handleExportGnuCash exports a book as a GnuCash file and streams it back as a
// download. The on-disk format is chosen by the optional ?format= query
// parameter: "sqlite" (the default, highest-fidelity target) or "xml" (the
// portable, human-readable form). The writer needs a path, so the file is built
// in a temp file which is removed once it has been sent.
func (s *Server) handleExportGnuCash(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "sqlite"
	}
	contentType, ok := exportContentTypes[format]
	if !ok {
		writeError(w, http.StatusBadRequest, `format must be "sqlite" or "xml"`)
		return
	}

	tmp, err := os.CreateTemp("", "openledger-export-*.gnucash")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not stage export")
		return
	}
	path := tmp.Name()
	// The SQLite driver opens the path itself, so close our handle now and let
	// the writer own the file; we only need the path.
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	var exportErr error
	switch format {
	case "xml":
		exportErr = s.Exporter.ExportXML(r.Context(), bookGUID, path)
	default:
		exportErr = s.Exporter.ExportSQLite(r.Context(), bookGUID, path)
	}
	switch {
	case errors.Is(exportErr, app.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book not found")
		return
	case exportErr != nil:
		writeError(w, http.StatusInternalServerError, "could not export book")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gnucash"`, bookGUID))
	http.ServeFile(w, r, path)
}

// exportContentTypes maps each supported export format to its MIME type, and
// doubles as the allow-list of valid ?format= values.
var exportContentTypes = map[string]string{
	"sqlite": "application/x-sqlite3",
	"xml":    "application/xml",
}
