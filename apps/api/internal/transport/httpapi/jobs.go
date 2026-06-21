package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type jobBodyDTO struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	Reference string `json:"reference"`
	Active    bool   `json:"active"`
	OwnerType string `json:"ownerType"`
	OwnerGUID string `json:"ownerGuid"`
}

func jobToResponse(j domain.Job) map[string]any {
	return map[string]any{
		"guid":      j.GUID,
		"bookGuid":  j.BookGUID,
		"name":      j.Name,
		"id":        j.ID,
		"reference": j.Reference,
		"active":    j.Active,
		"ownerType": j.OwnerType,
		"ownerGuid": j.OwnerGUID,
		"createdAt": j.CreatedAt,
	}
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	activeOnly := r.URL.Query().Get("active") == "true"
	list, err := s.job.ListJobs(r.Context(), bookGUID, userID, activeOnly)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	out := make([]map[string]any, len(list))
	for i, j := range list {
		out[i] = jobToResponse(j)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "jobs": out})
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto jobBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	j := domain.Job{
		BookGUID:  bookGUID,
		Name:      dto.Name,
		ID:        dto.ID,
		Reference: dto.Reference,
		Active:    dto.Active,
		OwnerType: dto.OwnerType,
		OwnerGUID: dto.OwnerGUID,
	}
	created, err := s.job.CreateJob(r.Context(), userID, j)
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "name, ownerType (customer|vendor) and ownerGuid are required")
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusCreated, jobToResponse(created))
	}
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	j, err := s.job.GetJob(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrJobNotFound):
		writeError(w, http.StatusNotFound, "job not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, jobToResponse(j))
	}
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto jobBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	j := domain.Job{
		GUID:      guid,
		Name:      dto.Name,
		ID:        dto.ID,
		Reference: dto.Reference,
		Active:    dto.Active,
	}
	updated, err := s.job.UpdateJob(r.Context(), userID, j)
	switch {
	case errors.Is(err, domain.ErrJobNotFound):
		writeError(w, http.StatusNotFound, "job not found")
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusOK, jobToResponse(updated))
	}
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	err := s.job.DeleteJob(r.Context(), guid, userID)
	switch {
	case errors.Is(err, domain.ErrJobNotFound):
		writeError(w, http.StatusNotFound, "job not found")
	case err != nil:
		writeAuthzError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
