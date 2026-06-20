// Command server is the OpenLedger HTTP API entrypoint. It wires a pgx
// connection pool into the app services and serves the HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/infra/gnucash"
	"github.com/openledger/openledger/apps/api/internal/infra/pg"
	"github.com/openledger/openledger/apps/api/internal/transport/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dsn := envOr("DATABASE_URL", "postgres://openledger:openledger@localhost:5432/openledger?sslmode=disable")
	// pgxpool.New is lazy: it validates the DSN but does not dial until first
	// use, so the server (and /healthz) start even if the DB is briefly down.
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		logger.Error("invalid database config", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := pg.NewRepository(pool)
	posting := app.NewPostingService(repo).WithTrading(app.NewTradingService(repo))
	ledger := app.NewLedgerService(repo)
	structure := app.NewStructureService(repo)
	price := app.NewPriceService(repo)
	report := app.NewReportService(repo)
	forecast := app.NewForecastService(repo)
	provision := app.NewProvisionService(repo)
	authz := app.NewAuthzService(repo)
	importer := app.NewImportService(gnucash.NewReader(), repo)
	exporter := app.NewExportService(repo, gnucash.NewWriter())
	reconciler := app.NewReconcileService(repo)
	portfolio := app.NewPortfolioService(repo)
	trade := app.NewTradeService(repo, posting)
	capitalGains := app.NewCapitalGainsService(repo)
	schedule := app.NewScheduleService(repo, posting)
	budget := app.NewBudgetService(repo)
	customer := app.NewCustomerService(repo, authz)
	vendor := app.NewVendorService(repo, authz)
	invoice := app.NewInvoiceService(repo, posting, authz)
	billterm := app.NewBillTermService(repo)

	server := httpapi.NewServer(posting, ledger, structure, price, report, forecast, provision, authz, importer, exporter, reconciler, portfolio, trade, capitalGains, schedule, budget, customer, vendor, invoice, billterm)

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
	logger.Info("server stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
