// Command seed populates a fresh database with one demo book: a small chart of
// accounts and a couple of balanced transactions. It exercises the same
// services the HTTP API uses (structure + posting), so a successful run is also
// a smoke test of the create -> post -> register vertical slice.
//
//	make seed   # or: cd apps/api && go run ./cmd/seed
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
	"github.com/openledger/openledger/apps/api/internal/infra/pg"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func run() error {
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://openledger:openledger@localhost:5432/openledger?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db (is it up and migrated?): %w", err)
	}

	repo := pg.NewRepository(pool)
	structure := app.NewStructureService(repo)
	posting := app.NewPostingService(repo)
	provision := app.NewProvisionService(repo)

	// 1. Currency.
	usd, err := structure.CreateCommodity(ctx, domain.Commodity{
		Mnemonic: "USD", Fullname: "US Dollar", Fraction: 100,
	})
	if err != nil {
		return fmt.Errorf("create commodity: %w", err)
	}

	// 2. JIT-provision the seed owner. Auth is handled by Authelia + lldap;
	// this just ensures a users row exists so membership can be recorded.
	// SEED_OWNER_LDAP_UID must match an actual lldap user (default: admin).
	ownerUID := envOr("SEED_OWNER_LDAP_UID", "admin")
	ownerEmail := envOr("SEED_OWNER_EMAIL", ownerUID+"@openledger.local")
	ownerID, err := provision.ProvisionUser(ctx, ownerUID, ownerEmail)
	if err != nil {
		return fmt.Errorf("provision seed owner %q: %w", ownerUID, err)
	}

	// 3. Book (and its root account), owned by the LDAP user so it is reachable
	// through the membership-scoped API.
	book, err := structure.CreateBook(ctx, ownerID)
	if err != nil {
		return fmt.Errorf("create book: %w", err)
	}

	// 4. Chart of accounts: placeholder groups, then the leaves we post to.
	mk := func(name, code string, typ domain.AccountType, parent string, placeholder bool) (domain.Account, error) {
		return structure.CreateAccount(ctx, book.GUID, domain.Account{
			Name: name, Code: code, Type: typ, ParentGUID: parent,
			CommodityGUID: usd.GUID, Placeholder: placeholder,
		})
	}

	assets, err := mk("Assets", "1000", domain.AccountAsset, "", true)
	if err != nil {
		return err
	}
	expenses, err := mk("Expenses", "5000", domain.AccountExpense, "", true)
	if err != nil {
		return err
	}
	equity, err := mk("Equity", "3000", domain.AccountEquity, "", true)
	if err != nil {
		return err
	}
	checking, err := mk("Checking", "1010", domain.AccountBank, assets.GUID, false)
	if err != nil {
		return err
	}
	groceries, err := mk("Groceries", "5010", domain.AccountExpense, expenses.GUID, false)
	if err != nil {
		return err
	}
	opening, err := mk("Opening Balances", "3010", domain.AccountEquity, equity.GUID, false)
	if err != nil {
		return err
	}

	// 5. Balanced transactions (same currency, so value == quantity).
	usdAmount := func(cents int64) domain.GncNumeric { return domain.MustFromNumDenom(cents, 100) }
	split := func(acct string, cents int64) domain.Split {
		return domain.Split{AccountGUID: acct, Value: usdAmount(cents), Quantity: usdAmount(cents)}
	}

	openingTx := domain.Transaction{
		CurrencyGUID: usd.GUID,
		PostDate:     time.Now().AddDate(0, 0, -7).UTC(),
		Description:  "Opening balance",
		Splits:       []domain.Split{split(checking.GUID, 100000), split(opening.GUID, -100000)},
	}
	if _, err := posting.Post(ctx, openingTx, app.AuditActor{BookGUID: book.GUID}); err != nil {
		return fmt.Errorf("post opening balance: %w", err)
	}

	groceriesTx := domain.Transaction{
		CurrencyGUID: usd.GUID,
		PostDate:     time.Now().UTC(),
		Description:  "Weekly groceries",
		Splits:       []domain.Split{split(groceries.GUID, 5000), split(checking.GUID, -5000)},
	}
	if _, err := posting.Post(ctx, groceriesTx, app.AuditActor{BookGUID: book.GUID}); err != nil {
		return fmt.Errorf("post groceries: %w", err)
	}

	fmt.Printf("Seeded demo book %s\n", book.GUID)
	fmt.Printf("  Owner (lldap uid): %s\n", ownerUID)
	fmt.Printf("  Checking account:  %s (balance $950.00)\n", checking.GUID)
	fmt.Printf("  Groceries account: %s (balance $50.00)\n", groceries.GUID)
	fmt.Printf("Log in via Authelia as %q, then browse to /api/v1/books\n", ownerUID)
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
