// Command seed-skr04 populates a fresh database with a realistic German
// consulting-firm demo book using the SKR04 (Standardkontenrahmen 04) chart of
// accounts, account codes, and EUR as the base currency.
//
//	make seed-skr04   # or: cd apps/api && go run ./cmd/seed-skr04
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
		log.Fatalf("seed-skr04: %v", err)
	}
}

func run() error {
	ctx := context.Background()

	dsn := envOr("DATABASE_URL", "postgres://openledger:openledger@localhost:5432/openledger?sslmode=disable")
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
	posting := app.NewPostingService(repo).WithTrading(app.NewTradingService(repo))
	provision := app.NewProvisionService(repo)

	// 1. EUR commodity.
	// CreateCommodity uses ON CONFLICT DO NOTHING internally, so if EUR already
	// exists (e.g. a previous seed run) the returned GUID won't match the DB
	// row. Re-fetch by mnemonic to always get the canonical GUID.
	if _, err := structure.CreateCommodity(ctx, domain.Commodity{
		Mnemonic: "EUR", Fullname: "Euro", Fraction: 100,
	}); err != nil {
		return fmt.Errorf("create EUR commodity: %w", err)
	}
	all, err := structure.ListCommodities(ctx)
	if err != nil {
		return fmt.Errorf("list commodities: %w", err)
	}
	var eur domain.Commodity
	for _, c := range all {
		if c.Mnemonic == "EUR" {
			eur = c
			break
		}
	}
	if eur.GUID == "" {
		return fmt.Errorf("EUR commodity not found after creation")
	}

	// 2. Provision the seed owner so the book is reachable via the API.
	ownerUID := envOr("SEED_OWNER_LDAP_UID", "admin")
	ownerEmail := envOr("SEED_OWNER_EMAIL", ownerUID+"@openledger.local")
	ownerID, err := provision.ProvisionUser(ctx, ownerUID, ownerEmail)
	if err != nil {
		return fmt.Errorf("provision seed owner %q: %w", ownerUID, err)
	}

	// 3. Book.
	book, err := structure.CreateBook(ctx, ownerID, "Demo Company", "")
	if err != nil {
		return fmt.Errorf("create book: %w", err)
	}

	// 4. SKR04 chart of accounts.
	//
	// Hierarchy mirrors the SKR04 class numbering:
	//   Klasse 0 – Anlagevermögen   (fixed assets)
	//   Klasse 1 – Umlaufvermögen   (current assets)
	//   Klasse 2 – Eigenkapital     (equity)
	//   Klasse 3 – Verbindlichkeiten (liabilities)
	//   Klasse 4 – Betriebliche Aufwendungen (operating expenses)
	//   Klasse 8 – Erlöse           (revenue)
	mk := func(name, code string, typ domain.AccountType, parent string, placeholder bool) (domain.Account, error) {
		a, e := structure.CreateAccount(ctx, book.GUID, domain.Account{
			Name: name, Code: code, Type: typ, ParentGUID: parent,
			CommodityGUID: eur.GUID, Placeholder: placeholder,
		})
		if e != nil {
			return domain.Account{}, fmt.Errorf("create account %q: %w", name, e)
		}
		return a, nil
	}

	// Placeholder group headers.
	anlage, err := mk("Anlagevermögen", "0", domain.AccountAsset, "", true)
	if err != nil {
		return err
	}
	umlauf, err := mk("Umlaufvermögen", "1", domain.AccountAsset, "", true)
	if err != nil {
		return err
	}
	eigenkapGrp, err := mk("Eigenkapital", "2", domain.AccountEquity, "", true)
	if err != nil {
		return err
	}
	verbGrp, err := mk("Verbindlichkeiten", "3", domain.AccountLiability, "", true)
	if err != nil {
		return err
	}
	aufwGrp, err := mk("Betriebliche Aufwendungen", "4", domain.AccountExpense, "", true)
	if err != nil {
		return err
	}
	erloesGrp, err := mk("Erlöse", "8", domain.AccountIncome, "", true)
	if err != nil {
		return err
	}

	// Klasse 0 – fixed assets.
	bga, err := mk("Betriebs- und Geschäftsausstattung", "0420", domain.AccountAsset, anlage.GUID, false)
	if err != nil {
		return err
	}

	// Klasse 1 – current assets.
	kasse, err := mk("Kasse", "1000", domain.AccountCash, umlauf.GUID, false)
	if err != nil {
		return err
	}
	girokonto, err := mk("Girokonto", "1200", domain.AccountBank, umlauf.GUID, false)
	if err != nil {
		return err
	}
	if _, err = mk("Forderungen aus Lieferungen und Leistungen", "1400", domain.AccountReceivable, umlauf.GUID, false); err != nil {
		return err
	}

	// Klasse 2 – equity.
	ekonto, err := mk("Eigenkapital", "2000", domain.AccountEquity, eigenkapGrp.GUID, false)
	if err != nil {
		return err
	}

	// Klasse 3 – liabilities.
	if _, err = mk("Verbindlichkeiten aus Lieferungen und Leistungen", "3300", domain.AccountPayable, verbGrp.GUID, false); err != nil {
		return err
	}

	// Klasse 4 – operating expenses.
	loehne, err := mk("Löhne und Gehälter", "4010", domain.AccountExpense, aufwGrp.GUID, false)
	if err != nil {
		return err
	}
	miete, err := mk("Miete", "4120", domain.AccountExpense, aufwGrp.GUID, false)
	if err != nil {
		return err
	}
	werbe, err := mk("Werbe- und Reisekosten", "4200", domain.AccountExpense, aufwGrp.GUID, false)
	if err != nil {
		return err
	}
	buero, err := mk("Bürobedarf", "4650", domain.AccountExpense, aufwGrp.GUID, false)
	if err != nil {
		return err
	}
	buchfuehrung, err := mk("Buchführungskosten", "4945", domain.AccountExpense, aufwGrp.GUID, false)
	if err != nil {
		return err
	}

	// Klasse 8 – revenue.
	umsatz, err := mk("Umsatzerlöse 19 % USt", "8000", domain.AccountIncome, erloesGrp.GUID, false)
	if err != nil {
		return err
	}

	// 5. Balanced transactions.
	//
	// All dates are in the current calendar year so reports (YTD income
	// statement, cash-flow) show meaningful data on first load.
	year := time.Now().Year()
	on := func(month, day int) time.Time {
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	}
	// cent converts a euro-cent integer to GncNumeric (e.g. 5000000 = 50 000,00 EUR).
	cent := func(n int64) domain.GncNumeric { return domain.MustFromNumDenom(n, 100) }
	sp := func(acct string, cents int64) domain.Split {
		a := cent(cents)
		return domain.Split{AccountGUID: acct, Value: a, Quantity: a}
	}
	post := func(desc string, d time.Time, splits ...domain.Split) error {
		_, e := posting.Post(ctx, domain.Transaction{
			CurrencyGUID: eur.GUID,
			PostDate:     d,
			Description:  desc,
			Splits:       splits,
		}, app.AuditActor{BookGUID: book.GUID})
		if e != nil {
			return fmt.Errorf("post %q: %w", desc, e)
		}
		return nil
	}

	// Jan 1 – Eigenkapitaleinlage (founder's capital contribution).
	if err = post("Eigenkapitaleinlage",
		on(1, 1),
		sp(girokonto.GUID, 5_000_000), // +50 000,00
		sp(ekonto.GUID, -5_000_000),   // −50 000,00
	); err != nil {
		return err
	}

	// Jan 5 – PC + Peripherie (office equipment).
	if err = post("Büroausstattung – PC und Peripherie",
		on(1, 5),
		sp(bga.GUID, 238_000),        // +2 380,00
		sp(girokonto.GUID, -238_000), // −2 380,00
	); err != nil {
		return err
	}

	// Jan 31 – Miete Januar.
	if err = post("Miete Januar",
		on(1, 31),
		sp(miete.GUID, 120_000), // +1 200,00
		sp(girokonto.GUID, -120_000),
	); err != nil {
		return err
	}

	// Feb 3 – Beratungshonorar Müller GmbH.
	if err = post("Beratungshonorar Müller GmbH",
		on(2, 3),
		sp(girokonto.GUID, 595_000), // +5 950,00
		sp(umsatz.GUID, -595_000),
	); err != nil {
		return err
	}

	// Feb 28 – Gehaltszahlung + Miete.
	if err = post("Gehaltszahlung Februar",
		on(2, 28),
		sp(loehne.GUID, 350_000), // +3 500,00
		sp(girokonto.GUID, -350_000),
	); err != nil {
		return err
	}
	if err = post("Miete Februar",
		on(2, 28),
		sp(miete.GUID, 120_000),
		sp(girokonto.GUID, -120_000),
	); err != nil {
		return err
	}

	// Mar 1 – Werbeausgaben (online ads).
	if err = post("Online-Werbung März",
		on(3, 1),
		sp(werbe.GUID, 45_000), // +450,00
		sp(girokonto.GUID, -45_000),
	); err != nil {
		return err
	}

	// Mar 15 – Beratungshonorar Meyer AG.
	if err = post("Beratungshonorar Meyer AG",
		on(3, 15),
		sp(girokonto.GUID, 476_000), // +4 760,00
		sp(umsatz.GUID, -476_000),
	); err != nil {
		return err
	}

	// Mar 20 – Bürobedarf (toner, paper).
	if err = post("Bürobedarf – Toner und Papier",
		on(3, 20),
		sp(buero.GUID, 8_900), // +89,00
		sp(girokonto.GUID, -8_900),
	); err != nil {
		return err
	}

	// Mar 28 – Gehaltszahlung März.
	if err = post("Gehaltszahlung März",
		on(3, 28),
		sp(loehne.GUID, 350_000),
		sp(girokonto.GUID, -350_000),
	); err != nil {
		return err
	}

	// Mar 31 – Miete + Buchführungskosten (Steuerberater).
	if err = post("Miete März",
		on(3, 31),
		sp(miete.GUID, 120_000),
		sp(girokonto.GUID, -120_000),
	); err != nil {
		return err
	}
	if err = post("Buchführungskosten Q1 – Steuerberater",
		on(3, 31),
		sp(buchfuehrung.GUID, 18_000), // +180,00
		sp(girokonto.GUID, -18_000),
	); err != nil {
		return err
	}

	// Apr 1 – Kasseneinlage (cash float from bank).
	if err = post("Kasseneinlage",
		on(4, 1),
		sp(kasse.GUID, 50_000), // +500,00
		sp(girokonto.GUID, -50_000),
	); err != nil {
		return err
	}

	// Apr 10 – Beratungshonorar Becker KG (largest contract yet).
	if err = post("Beratungshonorar Becker KG",
		on(4, 10),
		sp(girokonto.GUID, 833_000), // +8 330,00
		sp(umsatz.GUID, -833_000),
	); err != nil {
		return err
	}

	// Apr 28 – Gehaltszahlung April.
	if err = post("Gehaltszahlung April",
		on(4, 28),
		sp(loehne.GUID, 350_000),
		sp(girokonto.GUID, -350_000),
	); err != nil {
		return err
	}

	// Apr 30 – Miete April.
	if err = post("Miete April",
		on(4, 30),
		sp(miete.GUID, 120_000),
		sp(girokonto.GUID, -120_000),
	); err != nil {
		return err
	}

	// Final bank balance: 50 000 − 2 380 − 3×1 200 − 5 950 − 4 760 − 8 330
	//   ... + 3×3 500 + 450 + 89 + 180 + 500 = see output below.
	fmt.Printf("Seeded SKR04 demo book %s\n", book.GUID)
	fmt.Printf("  Owner (lldap uid): %s\n", ownerUID)
	fmt.Printf("  Currency:          EUR\n")
	fmt.Printf("  Girokonto (1200):  balance ≈ 50 141,00 EUR\n")
	fmt.Printf("  Revenue YTD:       19 040,00 EUR (3 invoices)\n")
	fmt.Printf("  Expenses YTD:      operating + payroll + equipment\n")
	fmt.Printf("Log in via Authelia as %q, then browse to /api/v1/books\n", ownerUID)
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
