package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type invoiceBodyDTO struct {
	ID           string `json:"id"`
	Type         string `json:"type"` // "invoice" or "bill"
	OwnerGUID    string `json:"ownerGuid"`
	DateOpened   string `json:"dateOpened"`
	Notes        string `json:"notes"`
	Active       bool   `json:"active"`
	CurrencyGUID string `json:"currencyGuid"`
	TermsGUID    string `json:"termsGuid"`
}

type entryBodyDTO struct {
	InvoiceGUID string      `json:"invoiceGuid"`
	Date        string      `json:"date"`
	Description string      `json:"description"`
	Action      string      `json:"action"`
	Notes       string      `json:"notes"`
	Quantity    *numericDTO `json:"quantity"`
	AccountGUID string      `json:"accountGuid"`
	Price       *numericDTO `json:"price"`
	Taxable     bool        `json:"taxable"`
}

type postInvoiceDTO struct {
	PostDate    string `json:"postDate"`
	DueDate     string `json:"dueDate"`
	PostAccGUID string `json:"postAccGuid"`
}

// ── Response helpers ──────────────────────────────────────────────────────────

func invoiceToResponse(inv domain.Invoice) map[string]any {
	r := map[string]any{
		"guid":         inv.GUID,
		"bookGuid":     inv.BookGUID,
		"id":           inv.ID,
		"type":         string(inv.Type),
		"ownerGuid":    inv.OwnerGUID,
		"dateOpened":   inv.DateOpened.Format("2006-01-02"),
		"notes":        inv.Notes,
		"active":       inv.Active,
		"currencyGuid": inv.CurrencyGUID,
		"postTxnGuid":  inv.PostTxnGUID,
		"postAccGuid":  inv.PostAccGUID,
		"termsGuid":    inv.TermsGUID,
		"paidTxnGuid":  inv.PaidTxnGUID,
		"createdAt":    inv.CreatedAt.Format(time.RFC3339),
	}
	if inv.DatePosted != nil {
		r["datePosted"] = inv.DatePosted.Format("2006-01-02")
	} else {
		r["datePosted"] = nil
	}
	if inv.DateDue != nil {
		r["dateDue"] = inv.DateDue.Format("2006-01-02")
	} else {
		r["dateDue"] = nil
	}
	if inv.PaidAt != nil {
		r["paidAt"] = inv.PaidAt.Format("2006-01-02")
	} else {
		r["paidAt"] = nil
	}
	if inv.Entries != nil {
		entries := make([]map[string]any, len(inv.Entries))
		for i, e := range inv.Entries {
			entries[i] = entryToResponse(e)
		}
		r["entries"] = entries
	}
	return r
}

func entryToResponse(e domain.InvoiceEntry) map[string]any {
	return map[string]any{
		"guid":        e.GUID,
		"invoiceGuid": e.InvoiceGUID,
		"date":        e.Date.Format("2006-01-02"),
		"description": e.Description,
		"action":      e.Action,
		"notes":       e.Notes,
		"quantity":    numericAtScale(e.Quantity, 1000),
		"accountGuid": e.AccountGUID,
		"price":       numericAtScale(e.Price, 100),
		"lineTotal":   numericAtScale(e.LineTotal(), 100),
		"taxable":     e.Taxable,
		"createdAt":   e.CreatedAt.Format(time.RFC3339),
	}
}

// ── Invoice handlers ──────────────────────────────────────────────────────────

func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	bookGUID := r.PathValue("id")
	invType := r.URL.Query().Get("type")
	if invType == "" {
		invType = "invoice"
	}
	list, err := s.invoice.ListInvoices(r.Context(), userID, bookGUID, domain.InvoiceType(invType))
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	rows := make([]map[string]any, len(list))
	for i, inv := range list {
		rows[i] = invoiceToResponse(inv)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookGuid": bookGUID, "type": invType, "invoices": rows})
}

func (s *Server) handleCreateInvoice(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	bookGUID := r.PathValue("id")
	var body invoiceBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	dateOpened := time.Now().UTC().Truncate(24 * time.Hour)
	if body.DateOpened != "" {
		if t, err := time.Parse("2006-01-02", body.DateOpened); err == nil {
			dateOpened = t
		}
	}
	inv := domain.Invoice{
		BookGUID:     bookGUID,
		ID:           body.ID,
		Type:         domain.InvoiceType(body.Type),
		OwnerGUID:    body.OwnerGUID,
		DateOpened:   dateOpened,
		Notes:        body.Notes,
		Active:       body.Active,
		CurrencyGUID: body.CurrencyGUID,
		TermsGUID:    body.TermsGUID,
	}
	created, err := s.invoice.CreateInvoice(r.Context(), userID, inv)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, invoiceToResponse(created))
}

func (s *Server) handleGetInvoice(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	inv, err := s.invoice.GetInvoice(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoiceToResponse(inv))
}

func (s *Server) handleUpdateInvoice(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	guid := r.PathValue("id")
	var body invoiceBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	dateOpened := time.Now().UTC().Truncate(24 * time.Hour)
	if body.DateOpened != "" {
		if t, err := time.Parse("2006-01-02", body.DateOpened); err == nil {
			dateOpened = t
		}
	}
	inv := domain.Invoice{
		GUID:         guid,
		ID:           body.ID,
		OwnerGUID:    body.OwnerGUID,
		DateOpened:   dateOpened,
		Notes:        body.Notes,
		Active:       body.Active,
		CurrencyGUID: body.CurrencyGUID,
		TermsGUID:    body.TermsGUID,
	}
	updated, err := s.invoice.UpdateInvoice(r.Context(), userID, inv)
	if err != nil {
		if errors.Is(err, domain.ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "invoice not found")
			return
		}
		if errors.Is(err, domain.ErrInvoiceAlreadyPosted) {
			writeError(w, http.StatusUnprocessableEntity, "invoice is already posted")
			return
		}
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoiceToResponse(updated))
}

func (s *Server) handleDeleteInvoice(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	if err := s.invoice.DeleteInvoice(r.Context(), userID, r.PathValue("id")); err != nil {
		if errors.Is(err, domain.ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "invoice not found")
			return
		}
		if errors.Is(err, domain.ErrInvoiceAlreadyPosted) {
			writeError(w, http.StatusUnprocessableEntity, "cannot delete a posted invoice")
			return
		}
		writeAuthzError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePostInvoice(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	guid := r.PathValue("id")
	var body postInvoiceDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.PostAccGUID == "" {
		writeError(w, http.StatusBadRequest, "postAccGuid is required")
		return
	}
	postDate := time.Now().UTC().Truncate(24 * time.Hour)
	if body.PostDate != "" {
		if t, err := time.Parse("2006-01-02", body.PostDate); err == nil {
			postDate = t
		}
	}
	var dueDate *time.Time
	if body.DueDate != "" {
		if t, err := time.Parse("2006-01-02", body.DueDate); err == nil {
			dueDate = &t
		}
	}
	req := app.PostRequest{PostDate: postDate, DueDate: dueDate, PostAccGUID: body.PostAccGUID}
	inv, err := s.invoice.PostInvoice(r.Context(), userID, guid, req)
	if err != nil {
		if errors.Is(err, domain.ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "invoice not found")
			return
		}
		if errors.Is(err, domain.ErrInvoiceAlreadyPosted) {
			writeError(w, http.StatusUnprocessableEntity, "invoice is already posted")
			return
		}
		if errors.Is(err, domain.ErrInvoiceNoEntries) {
			writeError(w, http.StatusUnprocessableEntity, "invoice has no entries")
			return
		}
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoiceToResponse(inv))
}

// ── Entry handlers ────────────────────────────────────────────────────────────

func (s *Server) handleListEntries(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	invoiceGUID := r.PathValue("id")
	// GetInvoice validates auth and loads entries.
	inv, err := s.invoice.GetInvoice(r.Context(), userID, invoiceGUID)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	rows := make([]map[string]any, len(inv.Entries))
	for i, e := range inv.Entries {
		rows[i] = entryToResponse(e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoiceGuid": invoiceGUID, "entries": rows})
}

func (s *Server) handleAddEntry(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	invoiceGUID := r.PathValue("id")
	var body entryBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	qty := domain.MustFromNumDenom(1, 1)
	if body.Quantity != nil {
		qty = domain.MustFromNumDenom(body.Quantity.Num, body.Quantity.Denom)
	}
	price := domain.Zero()
	if body.Price != nil {
		price = domain.MustFromNumDenom(body.Price.Num, body.Price.Denom)
	}
	entryDate := time.Now().UTC().Truncate(24 * time.Hour)
	if body.Date != "" {
		if t, err := time.Parse("2006-01-02", body.Date); err == nil {
			entryDate = t
		}
	}
	entry := domain.InvoiceEntry{
		InvoiceGUID: invoiceGUID,
		Date:        entryDate,
		Description: body.Description,
		Action:      body.Action,
		Notes:       body.Notes,
		Quantity:    qty,
		AccountGUID: body.AccountGUID,
		Price:       price,
		Taxable:     body.Taxable,
	}
	created, err := s.invoice.AddEntry(r.Context(), userID, entry)
	if err != nil {
		if errors.Is(err, domain.ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "invoice not found")
			return
		}
		if errors.Is(err, domain.ErrInvoiceAlreadyPosted) {
			writeError(w, http.StatusUnprocessableEntity, "invoice is already posted")
			return
		}
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entryToResponse(created))
}

func (s *Server) handleUpdateEntry(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	guid := r.PathValue("id")
	var body entryBodyDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	qty := domain.MustFromNumDenom(1, 1)
	if body.Quantity != nil {
		qty = domain.MustFromNumDenom(body.Quantity.Num, body.Quantity.Denom)
	}
	price := domain.Zero()
	if body.Price != nil {
		price = domain.MustFromNumDenom(body.Price.Num, body.Price.Denom)
	}
	entryDate := time.Now().UTC().Truncate(24 * time.Hour)
	if body.Date != "" {
		if t, err := time.Parse("2006-01-02", body.Date); err == nil {
			entryDate = t
		}
	}
	entry := domain.InvoiceEntry{
		GUID:        guid,
		Date:        entryDate,
		Description: body.Description,
		Action:      body.Action,
		Notes:       body.Notes,
		Quantity:    qty,
		AccountGUID: body.AccountGUID,
		Price:       price,
		Taxable:     body.Taxable,
	}
	updated, err := s.invoice.UpdateEntry(r.Context(), userID, entry)
	if err != nil {
		if errors.Is(err, domain.ErrEntryNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		if errors.Is(err, domain.ErrInvoiceAlreadyPosted) {
			writeError(w, http.StatusUnprocessableEntity, "invoice is already posted")
			return
		}
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entryToResponse(updated))
}

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	if err := s.invoice.DeleteEntry(r.Context(), userID, r.PathValue("id")); err != nil {
		if errors.Is(err, domain.ErrEntryNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		if errors.Is(err, domain.ErrInvoiceAlreadyPosted) {
			writeError(w, http.StatusUnprocessableEntity, "invoice is already posted")
			return
		}
		writeAuthzError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Payment handler ───────────────────────────────────────────────────────────

func (s *Server) handlePayInvoice(w http.ResponseWriter, r *http.Request) {
	userID := actorFromContext(r.Context()).UserID
	guid := r.PathValue("id")
	var body struct {
		PaymentDate    string `json:"paymentDate"`
		PaymentAccGUID string `json:"paymentAccGuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.PaymentAccGUID == "" {
		writeError(w, http.StatusBadRequest, "paymentAccGuid is required")
		return
	}
	payDate := time.Now().UTC().Truncate(24 * time.Hour)
	if body.PaymentDate != "" {
		if t, err := time.Parse("2006-01-02", body.PaymentDate); err == nil {
			payDate = t
		}
	}
	req := app.PayRequest{PaymentDate: payDate, PaymentAccGUID: body.PaymentAccGUID}
	inv, err := s.invoice.PayInvoice(r.Context(), userID, guid, req)
	if err != nil {
		if errors.Is(err, domain.ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "invoice not found")
			return
		}
		if errors.Is(err, domain.ErrInvoiceNotPosted) {
			writeError(w, http.StatusUnprocessableEntity, "invoice has not been posted")
			return
		}
		if errors.Is(err, domain.ErrInvoiceAlreadyPaid) {
			writeError(w, http.StatusUnprocessableEntity, "invoice is already paid")
			return
		}
		writeAuthzError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoiceToResponse(inv))
}

// ── Aging report handlers ─────────────────────────────────────────────────────

func (s *Server) handleARAgingReport(w http.ResponseWriter, r *http.Request) {
	s.handleAgingReport(w, r, domain.InvoiceTypeCustomer)
}

func (s *Server) handleAPAgingReport(w http.ResponseWriter, r *http.Request) {
	s.handleAgingReport(w, r, domain.InvoiceTypeBill)
}

func (s *Server) handleAgingReport(w http.ResponseWriter, r *http.Request, invType domain.InvoiceType) {
	userID := actorFromContext(r.Context()).UserID
	bookGUID := r.PathValue("id")
	report, err := s.invoice.AgingReport(r.Context(), userID, bookGUID, invType)
	if err != nil {
		writeAuthzError(w, err)
		return
	}
	buckets := make([]map[string]any, len(report.Buckets))
	for i, b := range report.Buckets {
		rows := make([]map[string]any, len(b.Rows))
		for j, row := range b.Rows {
			rows[j] = map[string]any{
				"invoice":     invoiceToResponse(row.Invoice),
				"total":       numericAtScale(row.Total, 100),
				"daysOverdue": row.DaysOverdue,
			}
		}
		buckets[i] = map[string]any{
			"label": b.Label,
			"rows":  rows,
			"total": numericAtScale(b.Total, 100),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid": report.BookGUID,
		"asOf":     report.AsOf,
		"buckets":  buckets,
		"total":    numericAtScale(report.Total, 100),
	})
}
