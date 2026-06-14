package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// handleExportGnuCash exports a book as a GnuCash SQLite file and streams it
// back as a download. The writer needs a path, so the database is built in a
// temp file which is removed once it has been sent.
func (s *Server) handleExportGnuCash(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
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

	switch err := s.exporter.ExportSQLite(r.Context(), bookGUID, path); {
	case errors.Is(err, app.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not export book")
		return
	}

	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gnucash"`, bookGUID))
	http.ServeFile(w, r, path)
}
