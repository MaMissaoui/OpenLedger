package app

import (
	"context"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeForecastRepo serves a fixed cash position and schedule list.
type fakeForecastRepo struct {
	root   string
	rows   []AccountWithBalance
	scheds []domain.ScheduledTransaction
}

func (f *fakeForecastRepo) BookRootAccount(_ context.Context, _ string) (string, error) {
	return f.root, nil
}

func (f *fakeForecastRepo) AccountBalances(_ context.Context, _ string, _, _ *time.Time) ([]AccountWithBalance, error) {
	return f.rows, nil
}

func (f *fakeForecastRepo) ListScheduledTransactions(_ context.Context, _ string) ([]domain.ScheduledTransaction, error) {
	return f.scheds, nil
}

func cashAcct(guid string) AccountWithBalance {
	return AccountWithBalance{
		Account:      domain.Account{GUID: guid, Type: domain.AccountBank},
		Balance:      domain.MustFromNumDenom(100000, 100), // 1000 on hand
		BalanceScale: 100,
	}
}

func TestForecastProjectsScheduledCash(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// A monthly +500 salary into the cash account, and a monthly -200 rent out of
	// it. Net +300/month against a 1000 opening balance.
	salary := domain.ScheduledTransaction{
		Name: "Salary", Enabled: true, Period: domain.PeriodMonthly, Every: 1,
		StartDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Splits: []domain.ScheduledSplit{
			{AccountGUID: "cash", Value: domain.MustFromNumDenom(50000, 100)},
			{AccountGUID: "income", Value: domain.MustFromNumDenom(-50000, 100)},
		},
	}
	rent := domain.ScheduledTransaction{
		Name: "Rent", Enabled: true, Period: domain.PeriodMonthly, Every: 1,
		StartDate: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
		Splits: []domain.ScheduledSplit{
			{AccountGUID: "cash", Value: domain.MustFromNumDenom(-20000, 100)},
			{AccountGUID: "expense", Value: domain.MustFromNumDenom(20000, 100)},
		},
	}
	repo := &fakeForecastRepo{
		root:   "root",
		rows:   []AccountWithBalance{cashAcct("cash")},
		scheds: []domain.ScheduledTransaction{salary, rent},
	}

	fc, err := NewForecastService(repo).Forecast(context.Background(), "book", start, 3)
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}

	if want := domain.MustFromNumDenom(100000, 100); !fc.StartingCash.Equal(want) {
		t.Errorf("starting cash = %s, want %s", fc.StartingCash, want)
	}
	// 3 months × (salary + rent) = 6 events.
	if len(fc.Events) != 6 {
		t.Fatalf("events = %d, want 6", len(fc.Events))
	}
	if len(fc.Points) != 3 {
		t.Fatalf("points = %d, want 3", len(fc.Points))
	}
	// Ending = 1000 + 3×(500−200) = 1900.
	if want := domain.MustFromNumDenom(190000, 100); !fc.EndingCash.Equal(want) {
		t.Errorf("ending cash = %s, want %s", fc.EndingCash, want)
	}
	if want := domain.MustFromNumDenom(90000, 100); !fc.NetChange.Equal(want) {
		t.Errorf("net change = %s, want %s", fc.NetChange, want)
	}
	// Each month's first point: 1000 + 300 = 1300, rising monotonically, so the
	// lowest point stays the opening 1000.
	if want := domain.MustFromNumDenom(130000, 100); !fc.Points[0].ProjectedCash.Equal(want) {
		t.Errorf("month 1 projected = %s, want %s", fc.Points[0].ProjectedCash, want)
	}
	if want := domain.MustFromNumDenom(100000, 100); !fc.LowestCash.Equal(want) {
		t.Errorf("lowest cash = %s, want %s", fc.LowestCash, want)
	}
}

func TestForecastIgnoresDisabledAndNonCash(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	disabled := domain.ScheduledTransaction{
		Name: "Old", Enabled: false, Period: domain.PeriodMonthly, Every: 1,
		StartDate: start,
		Splits: []domain.ScheduledSplit{
			{AccountGUID: "cash", Value: domain.MustFromNumDenom(99900, 100)},
			{AccountGUID: "income", Value: domain.MustFromNumDenom(-99900, 100)},
		},
	}
	// An accrual that never touches cash (expense ↔ payable) must not move it.
	accrual := domain.ScheduledTransaction{
		Name: "Accrue", Enabled: true, Period: domain.PeriodMonthly, Every: 1,
		StartDate: start,
		Splits: []domain.ScheduledSplit{
			{AccountGUID: "expense", Value: domain.MustFromNumDenom(10000, 100)},
			{AccountGUID: "payable", Value: domain.MustFromNumDenom(-10000, 100)},
		},
	}
	repo := &fakeForecastRepo{
		root:   "root",
		rows:   []AccountWithBalance{cashAcct("cash")},
		scheds: []domain.ScheduledTransaction{disabled, accrual},
	}
	fc, err := NewForecastService(repo).Forecast(context.Background(), "book", start, 6)
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}
	if len(fc.Events) != 0 {
		t.Errorf("events = %d, want 0 (disabled + non-cash schedules)", len(fc.Events))
	}
	if !fc.NetChange.IsZero() {
		t.Errorf("net change = %s, want 0", fc.NetChange)
	}
}
