// Package httpapi exposes the OpenLedger HTTP API over the app services.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// Services is the set of app use-case services the HTTP handlers depend on.
// It is constructed once (in main.go, or a test helper) and embedded in Server,
// so adding a service is a one-line field add rather than a positional-argument
// ripple across every construction site. Handlers reach a service through the
// promoted field, e.g. s.Posting.
type Services struct {
	Posting      *app.PostingService
	Ledger       *app.LedgerService
	Structure    *app.StructureService
	Price        *app.PriceService
	Quote        *app.QuoteService
	BankImport   *app.BankImportService
	Report       *app.ReportService
	Forecast     *app.ForecastService
	Provision    *app.ProvisionService
	Authz        *app.AuthzService
	Membership   *app.MembershipService
	Importer     *app.ImportService
	Exporter     *app.ExportService
	Reconciler   *app.ReconcileService
	Portfolio    *app.PortfolioService
	Trade        *app.TradeService
	CapitalGains *app.CapitalGainsService
	Schedule     *app.ScheduleService
	Budget       *app.BudgetService
	Customer     *app.CustomerService
	Vendor       *app.VendorService
	Employee     *app.EmployeeService
	Job          *app.JobService
	Invoice      *app.InvoiceService
	BillTerm     *app.BillTermService
	TaxTable     *app.TaxTableService
}

// Server holds the dependencies the HTTP handlers need. Build one directly:
// &Server{Services: Services{...}} — the named-field literal is the wiring
// contract, so field order and unset (nil) services no longer matter.
type Server struct {
	Services
}

// Routes returns the configured HTTP handler. /healthz is public; every
// /api/v1/* route requires a verified Authelia session (Remote-User header).
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)

	mux.HandleFunc("GET /api/v1/commodities", s.requireAuth(s.handleListCommodities))
	mux.HandleFunc("POST /api/v1/commodities", s.requireAuth(s.handleCreateCommodity))
	mux.HandleFunc("GET /api/v1/books", s.requireAuth(s.handleListBooks))
	mux.HandleFunc("POST /api/v1/books", s.requireAuth(s.handleCreateBook))
	mux.HandleFunc("GET /api/v1/books/{id}/accounts", s.requireAuth(s.handleListAccounts))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/balance-sheet", s.requireAuth(s.handleBalanceSheet))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/income-statement", s.requireAuth(s.handleIncomeStatement))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/cash-flow", s.requireAuth(s.handleCashFlow))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/cash-flow-forecast", s.requireAuth(s.handleCashFlowForecast))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/portfolio", s.requireAuth(s.handlePortfolio))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/capital-gains", s.requireAuth(s.handleCapitalGains))
	mux.HandleFunc("GET /api/v1/books/{id}/scheduled-transactions", s.requireAuth(s.handleListScheduledTransactions))
	mux.HandleFunc("POST /api/v1/books/{id}/scheduled-transactions", s.requireAuth(s.handleCreateScheduledTransaction))
	mux.HandleFunc("POST /api/v1/books/{id}/scheduled-transactions/post-due", s.requireAuth(s.handlePostDueSchedules))
	mux.HandleFunc("GET /api/v1/books/{id}/budgets", s.requireAuth(s.handleListBudgets))
	mux.HandleFunc("POST /api/v1/books/{id}/budgets", s.requireAuth(s.handleCreateBudget))
	mux.HandleFunc("GET /api/v1/budgets/{id}", s.requireAuth(s.handleGetBudget))
	mux.HandleFunc("PATCH /api/v1/budgets/{id}", s.requireAuth(s.handleUpdateBudget))
	mux.HandleFunc("DELETE /api/v1/budgets/{id}", s.requireAuth(s.handleDeleteBudget))
	mux.HandleFunc("GET /api/v1/budgets/{id}/report", s.requireAuth(s.handleBudgetReport))
	mux.HandleFunc("GET /api/v1/books/{id}/bill-terms", s.requireAuth(s.handleListBillTerms))
	mux.HandleFunc("POST /api/v1/books/{id}/bill-terms", s.requireAuth(s.handleCreateBillTerm))
	mux.HandleFunc("GET /api/v1/bill-terms/{id}", s.requireAuth(s.handleGetBillTerm))
	mux.HandleFunc("PATCH /api/v1/bill-terms/{id}", s.requireAuth(s.handleUpdateBillTerm))
	mux.HandleFunc("DELETE /api/v1/bill-terms/{id}", s.requireAuth(s.handleDeleteBillTerm))
	mux.HandleFunc("GET /api/v1/books/{id}/tax-tables", s.requireAuth(s.handleListTaxTables))
	mux.HandleFunc("POST /api/v1/books/{id}/tax-tables", s.requireAuth(s.handleCreateTaxTable))
	mux.HandleFunc("GET /api/v1/tax-tables/{id}", s.requireAuth(s.handleGetTaxTable))
	mux.HandleFunc("PATCH /api/v1/tax-tables/{id}", s.requireAuth(s.handleUpdateTaxTable))
	mux.HandleFunc("DELETE /api/v1/tax-tables/{id}", s.requireAuth(s.handleDeleteTaxTable))
	mux.HandleFunc("GET /api/v1/scheduled-transactions/{id}", s.requireAuth(s.handleGetScheduledTransaction))
	mux.HandleFunc("PATCH /api/v1/scheduled-transactions/{id}", s.requireAuth(s.handleUpdateScheduledTransaction))
	mux.HandleFunc("DELETE /api/v1/scheduled-transactions/{id}", s.requireAuth(s.handleDeleteScheduledTransaction))
	mux.HandleFunc("POST /api/v1/securities/buy", s.requireAuth(s.handleBuySecurity))
	mux.HandleFunc("POST /api/v1/securities/sell", s.requireAuth(s.handleSellSecurity))
	mux.HandleFunc("POST /api/v1/accounts", s.requireAuth(s.handleCreateAccount))
	mux.HandleFunc("POST /api/v1/transactions", s.requireAuth(s.handlePostTransaction))
	mux.HandleFunc("GET /api/v1/transactions/{id}", s.requireAuth(s.handleGetTransaction))
	mux.HandleFunc("PATCH /api/v1/transactions/{id}", s.requireAuth(s.handleUpdateTransaction))
	mux.HandleFunc("DELETE /api/v1/transactions/{id}", s.requireAuth(s.handleDeleteTransaction))
	mux.HandleFunc("GET /api/v1/accounts/{id}/register", s.requireAuth(s.handleAccountRegister))
	mux.HandleFunc("PATCH /api/v1/splits/{id}/reconcile", s.requireAuth(s.handleReconcileSplit))
	mux.HandleFunc("GET /api/v1/prices", s.requireAuth(s.handleListPrices))
	mux.HandleFunc("POST /api/v1/prices", s.requireAuth(s.handleCreatePrice))
	mux.HandleFunc("POST /api/v1/prices/fetch", s.requireAuth(s.handleFetchPrice))
	mux.HandleFunc("POST /api/v1/imports/gnucash", s.requireAuth(s.handleImportGnuCash))
	mux.HandleFunc("POST /api/v1/accounts/{id}/import-bank/preview", s.requireAuth(s.handlePreviewBankCSV))
	mux.HandleFunc("POST /api/v1/accounts/{id}/import-bank", s.requireAuth(s.handleImportBankStatement))
	mux.HandleFunc("GET /api/v1/books/{id}/export/gnucash", s.requireAuth(s.handleExportGnuCash))
	// Settings: book members (admin-managed RBAC)
	mux.HandleFunc("GET /api/v1/books/{id}/members", s.requireAuth(s.handleListMembers))
	mux.HandleFunc("POST /api/v1/books/{id}/members", s.requireAuth(s.handleAddMember))
	mux.HandleFunc("PATCH /api/v1/books/{id}/members/{userId}", s.requireAuth(s.handleUpdateMember))
	mux.HandleFunc("DELETE /api/v1/books/{id}/members/{userId}", s.requireAuth(s.handleRemoveMember))
	// Business: customers
	mux.HandleFunc("GET /api/v1/books/{id}/customers", s.requireAuth(s.handleListCustomers))
	mux.HandleFunc("POST /api/v1/books/{id}/customers", s.requireAuth(s.handleCreateCustomer))
	mux.HandleFunc("GET /api/v1/customers/{id}", s.requireAuth(s.handleGetCustomer))
	mux.HandleFunc("PATCH /api/v1/customers/{id}", s.requireAuth(s.handleUpdateCustomer))
	mux.HandleFunc("DELETE /api/v1/customers/{id}", s.requireAuth(s.handleDeleteCustomer))
	// Business: vendors
	mux.HandleFunc("GET /api/v1/books/{id}/vendors", s.requireAuth(s.handleListVendors))
	mux.HandleFunc("POST /api/v1/books/{id}/vendors", s.requireAuth(s.handleCreateVendor))
	mux.HandleFunc("GET /api/v1/vendors/{id}", s.requireAuth(s.handleGetVendor))
	mux.HandleFunc("PATCH /api/v1/vendors/{id}", s.requireAuth(s.handleUpdateVendor))
	mux.HandleFunc("DELETE /api/v1/vendors/{id}", s.requireAuth(s.handleDeleteVendor))
	// Business: employees
	mux.HandleFunc("GET /api/v1/books/{id}/employees", s.requireAuth(s.handleListEmployees))
	mux.HandleFunc("POST /api/v1/books/{id}/employees", s.requireAuth(s.handleCreateEmployee))
	mux.HandleFunc("GET /api/v1/employees/{id}", s.requireAuth(s.handleGetEmployee))
	mux.HandleFunc("PATCH /api/v1/employees/{id}", s.requireAuth(s.handleUpdateEmployee))
	mux.HandleFunc("DELETE /api/v1/employees/{id}", s.requireAuth(s.handleDeleteEmployee))
	// Business: jobs
	mux.HandleFunc("GET /api/v1/books/{id}/jobs", s.requireAuth(s.handleListJobs))
	mux.HandleFunc("POST /api/v1/books/{id}/jobs", s.requireAuth(s.handleCreateJob))
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.requireAuth(s.handleGetJob))
	mux.HandleFunc("PATCH /api/v1/jobs/{id}", s.requireAuth(s.handleUpdateJob))
	mux.HandleFunc("DELETE /api/v1/jobs/{id}", s.requireAuth(s.handleDeleteJob))
	// Business: invoices and bills
	mux.HandleFunc("GET /api/v1/books/{id}/invoices", s.requireAuth(s.handleListInvoices))
	mux.HandleFunc("POST /api/v1/books/{id}/invoices", s.requireAuth(s.handleCreateInvoice))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/ar-aging", s.requireAuth(s.handleARAgingReport))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/ap-aging", s.requireAuth(s.handleAPAgingReport))
	mux.HandleFunc("GET /api/v1/invoices/{id}", s.requireAuth(s.handleGetInvoice))
	mux.HandleFunc("PATCH /api/v1/invoices/{id}", s.requireAuth(s.handleUpdateInvoice))
	mux.HandleFunc("DELETE /api/v1/invoices/{id}", s.requireAuth(s.handleDeleteInvoice))
	mux.HandleFunc("POST /api/v1/invoices/{id}/post", s.requireAuth(s.handlePostInvoice))
	mux.HandleFunc("POST /api/v1/invoices/{id}/pay", s.requireAuth(s.handlePayInvoice))
	mux.HandleFunc("GET /api/v1/invoices/{id}/entries", s.requireAuth(s.handleListEntries))
	mux.HandleFunc("POST /api/v1/invoices/{id}/entries", s.requireAuth(s.handleAddEntry))
	mux.HandleFunc("PATCH /api/v1/entries/{id}", s.requireAuth(s.handleUpdateEntry))
	mux.HandleFunc("DELETE /api/v1/entries/{id}", s.requireAuth(s.handleDeleteEntry))
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
