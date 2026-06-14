package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

type fakeBudgetRepo struct {
	budgets  map[string]domain.Budget
	actuals  []AccountWithBalance
	bookRoot string
}

func newFakeBudgetRepo() *fakeBudgetRepo {
	return &fakeBudgetRepo{
		budgets:  make(map[string]domain.Budget),
		bookRoot: "root-1",
	}
}

func (f *fakeBudgetRepo) CreateBudget(_ context.Context, b domain.Budget) (domain.Budget, error) {
	f.budgets[b.GUID] = b
	return b, nil
}

func (f *fakeBudgetRepo) ListBudgets(_ context.Context, bookGUID string) ([]domain.Budget, error) {
	var out []domain.Budget
	for _, b := range f.budgets {
		if b.BookGUID == bookGUID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBudgetRepo) GetBudget(_ context.Context, guid string) (domain.Budget, error) {
	b, ok := f.budgets[guid]
	if !ok {
		return domain.Budget{}, domain.ErrBudgetNotFound
	}
	return b, nil
}

func (f *fakeBudgetRepo) UpdateBudget(_ context.Context, b domain.Budget) (domain.Budget, error) {
	if _, ok := f.budgets[b.GUID]; !ok {
		return domain.Budget{}, domain.ErrBudgetNotFound
	}
	f.budgets[b.GUID] = b
	return b, nil
}

func (f *fakeBudgetRepo) DeleteBudget(_ context.Context, guid string) error {
	delete(f.budgets, guid)
	return nil
}

func (f *fakeBudgetRepo) BookGUIDForBudget(_ context.Context, guid string) (string, error) {
	b, ok := f.budgets[guid]
	if !ok {
		return "", domain.ErrBudgetNotFound
	}
	return b.BookGUID, nil
}

func (f *fakeBudgetRepo) BookRootAccount(_ context.Context, _ string) (string, error) {
	return f.bookRoot, nil
}

func (f *fakeBudgetRepo) AccountBalances(_ context.Context, _ string, _, _ *time.Time) ([]AccountWithBalance, error) {
	return f.actuals, nil
}

func newBudgetService(repo *fakeBudgetRepo) *BudgetService {
	svc := NewBudgetService(repo)
	svc.newGUID = func() string { return "bgt-1" }
	return svc
}

func validBudget(bookGUID string) domain.Budget {
	return domain.Budget{
		BookGUID:   bookGUID,
		Name:       "2024 Budget",
		PeriodType: domain.BudgetMonthly,
		NumPeriods: 12,
		StartDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestBudgetCreate(t *testing.T) {
	repo := newFakeBudgetRepo()
	svc := newBudgetService(repo)
	ctx := context.Background()

	b, err := svc.Create(ctx, validBudget("book-1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.GUID == "" {
		t.Error("expected GUID to be assigned")
	}
}

func TestBudgetCreateValidation(t *testing.T) {
	repo := newFakeBudgetRepo()
	svc := newBudgetService(repo)
	ctx := context.Background()

	t.Run("empty name", func(t *testing.T) {
		b := validBudget("book-1")
		b.Name = ""
		if _, err := svc.Create(ctx, b); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
	t.Run("invalid period type", func(t *testing.T) {
		b := validBudget("book-1")
		b.PeriodType = "fortnightly"
		if _, err := svc.Create(ctx, b); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
	t.Run("zero start date", func(t *testing.T) {
		b := validBudget("book-1")
		b.StartDate = time.Time{}
		if _, err := svc.Create(ctx, b); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
	t.Run("zero num_periods", func(t *testing.T) {
		b := validBudget("book-1")
		b.NumPeriods = 0
		if _, err := svc.Create(ctx, b); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
}

func TestBudgetReport(t *testing.T) {
	repo := newFakeBudgetRepo()
	svc := newBudgetService(repo)
	ctx := context.Background()

	// Create a budget with $500 allocated to a groceries account in period 0.
	b := validBudget("book-1")
	b.GUID = "bgt-1"
	b.Amounts = []domain.BudgetAmount{
		{AccountGUID: "groc", PeriodNum: 0, Value: domain.MustFromNumDenom(50000, 100)},
	}
	repo.budgets["bgt-1"] = b

	// Fake actual: $450 spent on groceries.
	repo.actuals = []AccountWithBalance{{
		Account: domain.Account{GUID: "groc", Name: "Groceries", Type: domain.AccountExpense},
		Balance: domain.MustFromNumDenom(45000, 100),
	}}

	asOf := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	report, err := svc.Report(ctx, "bgt-1", asOf)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report.PeriodNum != 0 {
		t.Errorf("period = %d, want 0", report.PeriodNum)
	}
	if len(report.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(report.Lines))
	}
	line := report.Lines[0]
	if !line.Budgeted.Equal(domain.MustFromNumDenom(50000, 100)) {
		t.Errorf("budgeted = %s, want $500", line.Budgeted)
	}
	if !line.Actual.Equal(domain.MustFromNumDenom(45000, 100)) {
		t.Errorf("actual = %s, want $450", line.Actual)
	}
	// variance = actual - budgeted = -$50 (under budget)
	wantVariance := domain.MustFromNumDenom(-5000, 100)
	if !line.Variance.Equal(wantVariance) {
		t.Errorf("variance = %s, want -$50", line.Variance)
	}
}
