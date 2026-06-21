package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// memberBodyDTO is the request body for adding a member (by email) or changing
// a member's role.
type memberBodyDTO struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func memberToResponse(m app.Member) map[string]any {
	return map[string]any{
		"userId":   m.UserID,
		"email":    m.Email,
		"ldapUser": m.LDAPUser,
		"role":     string(m.Role),
	}
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	list, err := s.Membership.ListMembers(r.Context(), userID, bookGUID)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	out := make([]map[string]any, len(list))
	for i, m := range list {
		out[i] = memberToResponse(m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "members": out})
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	userID := actorFromContext(r.Context()).UserID
	var dto memberBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	member, err := s.Membership.AddMember(r.Context(), userID, bookGUID, dto.Email, app.Role(dto.Role))
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "email and a valid role are required")
	case errors.Is(err, app.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "no user with that email has signed in yet")
	case err != nil:
		writeAuthzError(w, err)
	default:
		writeJSON(w, http.StatusCreated, memberToResponse(member))
	}
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	targetUserID := r.PathValue("userId")
	userID := actorFromContext(r.Context()).UserID
	var dto memberBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	err := s.Membership.UpdateMemberRole(r.Context(), userID, bookGUID, targetUserID, app.Role(dto.Role))
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "a valid role is required")
	case errors.Is(err, app.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "that user is not a member of this book")
	case errors.Is(err, app.ErrLastOwner):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	targetUserID := r.PathValue("userId")
	userID := actorFromContext(r.Context()).UserID
	err := s.Membership.RemoveMember(r.Context(), userID, bookGUID, targetUserID)
	switch {
	case errors.Is(err, app.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "that user is not a member of this book")
	case errors.Is(err, app.ErrLastOwner):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeAuthzError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
