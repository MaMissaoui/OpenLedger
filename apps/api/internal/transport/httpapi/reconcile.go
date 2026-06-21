package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// reconcileRequest is the body of a reconcile update: the single-character
// GnuCash reconcile flag to set ("n" unmarked, "c" cleared, "y" reconciled).
type reconcileRequest struct {
	State string `json:"state"`
}

// handleReconcileSplit sets a split's reconcile state. Reconciliation only
// annotates a split (it never changes amounts), so it requires write access to
// the split's book but does not go through the posting service.
func (s *Server) handleReconcileSplit(w http.ResponseWriter, r *http.Request) {
	splitGUID := r.PathValue("id")
	if !s.authorizeSplit(w, r, splitGUID, app.AccessWrite) {
		return
	}

	var req reconcileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len([]rune(req.State)) != 1 {
		writeError(w, http.StatusBadRequest, "state must be a single character")
		return
	}
	state := domain.ReconcileState([]rune(req.State)[0])

	switch err := s.Reconciler.SetReconcile(r.Context(), splitGUID, state); {
	case errors.Is(err, app.ErrInvalidReconcileState):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, app.ErrSplitNotFound):
		writeError(w, http.StatusNotFound, "split not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update reconcile state")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"splitGuid": splitGUID, "state": req.State})
}
