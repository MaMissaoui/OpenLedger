//go:build integration

// Package pg integration tests run against a real Postgres. They are excluded
// from the default `go test`; run them with:
//
//	make migrate DEV_DSN=... && go test -tags=integration ./internal/infra/pg/...
//
// The DSN comes from DATABASE_URL, defaulting to the local dev stack.
package pg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

func testDSN() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://openledger:openledger@localhost:5432/openledger?sslmode=disable"
}

func newGUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

func TestGetBillTermRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, testDSN())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("no Postgres reachable (%v); run `make dev && make migrate`", err)
	}

	repo := NewRepository(pool)

	// A book is required by the billterms.book_guid foreign key.
	bookGUID := newGUID(t)
	if _, err := pool.Exec(ctx,
		`INSERT INTO books (guid, root_account_guid, root_template_guid) VALUES ($1, $2, $3)`,
		bookGUID, newGUID(t), newGUID(t)); err != nil {
		t.Fatalf("insert book: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM books WHERE guid=$1`, bookGUID)
	})

	want := domain.BillTerm{
		GUID:         newGUID(t),
		BookGUID:     bookGUID,
		Name:         "2/10 Net 30",
		Description:  "2% if paid within 10 days, net 30",
		Type:         domain.BillTermDays,
		DueDays:      30,
		DiscountDays: 10,
		Discount:     domain.MustFromNumDenom(2, 100),
		Cutoff:       0,
	}
	if _, err := repo.CreateBillTerm(ctx, want); err != nil {
		t.Fatalf("CreateBillTerm: %v", err)
	}

	got, err := repo.GetBillTerm(ctx, want.GUID)
	if err != nil {
		t.Fatalf("GetBillTerm: %v", err)
	}
	if got.Name != want.Name || got.Type != want.Type || got.DueDays != want.DueDays ||
		got.DiscountDays != want.DiscountDays || got.Cutoff != want.Cutoff {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, want)
	}
	if got.Discount.Cmp(want.Discount) != 0 {
		t.Errorf("discount: got %v, want %v", got.Discount, want.Discount)
	}

	// The derived due date is the real point of the term.
	due := got.DueDate(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2024, 2, 14, 0, 0, 0, 0, time.UTC); !due.Equal(want) {
		t.Errorf("DueDate: got %v, want %v", due, want)
	}

	// A missing term resolves to the domain not-found sentinel.
	if _, err := repo.GetBillTerm(ctx, newGUID(t)); err != domain.ErrBillTermNotFound {
		t.Errorf("GetBillTerm(missing): got %v, want ErrBillTermNotFound", err)
	}
}
