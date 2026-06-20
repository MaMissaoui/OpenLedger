// Package httpapi exposes the OpenLedger HTTP API over the app services.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// Server holds the dependencies the HTTP handlers need.
type Server struct {
	posting      *app.PostingService
	ledger       *app.LedgerService
	structure    *app.StructureService
	price        *app.PriceService
	report       *app.ReportService
	forecast     *app.ForecastService
	provision    *app.ProvisionService
	authz        *app.AuthzService
	importer     *app.ImportService
	exporter     *app.ExportService
	reconciler   *app.ReconcileService
	portfolio    *app.PortfolioService
	trade        *app.TradeService
	capitalGains *app.CapitalGainsService
	schedule     *app.ScheduleService
	budget       *app.BudgetService
	customer     *app.CustomerService
	vendor       *app.VendorService
	invoice      *app.InvoiceService
	billterm     *app.BillTermService
}

// NewServer builds a Server from its service dependencies.
func NewServer(posting *app.PostingService, ledger *app.LedgerService, structure *app.StructureService, price *app.PriceService, report *app.ReportService, forecast *app.ForecastService, provision *app.ProvisionService, authz *app.AuthzService, importer *app.ImportService, exporter *app.ExportService, reconciler *app.ReconcileService, portfolio *app.PortfolioService, trade *app.TradeService, capitalGains *app.CapitalGainsService, schedule *app.ScheduleService, budget *app.BudgetService, customer *app.CustomerService, vendor *app.VendorService, invoice *app.InvoiceService, billterm *app.BillTermService) *Server {
	return &Server{posting: posting, ledger: ledger, structure: structure, price: price, report: report, forecast: forecast, provision: provision, authz: authz, importer: importer, exporter: exporter, reconciler: reconciler, portfolio: portfolio, trade: trade, capitalGains: capitalGains, schedule: schedule, budget: budget, customer: customer, vendor: vendor, invoice: invoice, billterm: billterm}
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
	mux.HandleFunc("POST /api/v1/imports/gnucash", s.requireAuth(s.handleImportGnuCash))
	mux.HandleFunc("GET /api/v1/books/{id}/export/gnucash", s.requireAuth(s.handleExportGnuCash))
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
