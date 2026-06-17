package httpapi

import (
	"net/http"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
)

func (s *Server) handleBalanceSheet(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	asOf, ok := queryTime(w, r, "asOf", time.Now())
	if !ok {
		return
	}
	bs, err := s.report.BalanceSheet(r.Context(), bookGUID, asOf)
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid":                  bookGUID,
		"asOf":                      bs.AsOf,
		"assets":                    sectionDTO(bs.Assets),
		"liabilities":               sectionDTO(bs.Liabilities),
		"equity":                    sectionDTO(bs.Equity),
		"retainedEarnings":          numericAtScale(bs.RetainedEarnings, 100),
		"totalLiabilitiesAndEquity": numericAtScale(bs.TotalLiabilitiesAndEquity, 100),
	})
}

func (s *Server) handleIncomeStatement(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	// An absent "from" is an open lower bound (since the book began); an absent
	// "to" defaults to now.
	from, ok := queryTime(w, r, "from", time.Time{})
	if !ok {
		return
	}
	to, ok := queryTime(w, r, "to", time.Now())
	if !ok {
		return
	}
	is, err := s.report.IncomeStatement(r.Context(), bookGUID, from, to)
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid":  bookGUID,
		"from":      is.From,
		"to":        is.To,
		"income":    sectionDTO(is.Income),
		"expense":   sectionDTO(is.Expense),
		"netIncome": numericAtScale(is.NetIncome, 100),
	})
}

func (s *Server) handleCapitalGains(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	from, ok := queryTime(w, r, "from", time.Time{})
	if !ok {
		return
	}
	to, ok := queryTime(w, r, "to", time.Now())
	if !ok {
		return
	}
	cg, err := s.capitalGains.CapitalGains(r.Context(), bookGUID, from, to)
	if writeStructureError(w, err) {
		return
	}
	lines := make([]map[string]any, 0, len(cg.Lines))
	for _, l := range cg.Lines {
		lines = append(lines, map[string]any{
			"date":        l.Date,
			"description": l.Description,
			"account":     l.Account,
			"amount":      numericAtScale(l.Amount, l.Scale),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid": bookGUID,
		"from":     cg.From,
		"to":       cg.To,
		"lines":    lines,
		"total":    numericAtScale(cg.Total, 100),
	})
}

func (s *Server) handleCashFlow(w http.ResponseWriter, r *http.Request) {
	bookGUID := r.PathValue("id")
	if !s.authorizeBook(w, r, bookGUID, app.AccessRead) {
		return
	}
	from, ok := queryTime(w, r, "from", time.Time{})
	if !ok {
		return
	}
	to, ok := queryTime(w, r, "to", time.Now())
	if !ok {
		return
	}
	cf, err := s.report.CashFlowStatement(r.Context(), bookGUID, from, to)
	if writeStructureError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookGuid":      bookGUID,
		"from":          cf.From,
		"to":            cf.To,
		"operating":     cashSectionDTO(cf.Operating),
		"investing":     cashSectionDTO(cf.Investing),
		"financing":     cashSectionDTO(cf.Financing),
		"netChange":     numericAtScale(cf.NetChange, 100),
		"beginningCash": numericAtScale(cf.BeginningCash, 100),
		"endingCash":    numericAtScale(cf.EndingCash, 100),
	})
}

func cashSectionDTO(sec app.CashFlowSection) map[string]any {
	lines := make([]map[string]any, 0, len(sec.Lines))
	for _, l := range sec.Lines {
		lines = append(lines, map[string]any{
			"account": accountDTO(l.Account),
			"amount":  numericAtScale(l.Balance, l.Scale),
		})
	}
	return map[string]any{
		"lines": lines,
		"total": numericAtScale(sec.Total, 100),
	}
}

// queryTime parses an RFC 3339 timestamp from query parameter key, returning def
// when it is absent. On a malformed value it writes a 400 and returns ok=false.
func queryTime(w http.ResponseWriter, r *http.Request, key string, def time.Time) (time.Time, bool) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, key+" must be an RFC 3339 timestamp")
		return time.Time{}, false
	}
	return t, true
}

func sectionDTO(sec app.ReportSection) map[string]any {
	lines := make([]map[string]any, 0, len(sec.Lines))
	for _, l := range sec.Lines {
		lines = append(lines, map[string]any{
			"account": accountDTO(l.Account),
			"balance": numericAtScale(l.Balance, l.Scale),
		})
	}
	return map[string]any{
		"lines": lines,
		"total": numericAtScale(sec.Total, 100),
	}
}
