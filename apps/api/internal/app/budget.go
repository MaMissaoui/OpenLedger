package app

import (
	"context"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// BudgetRepository persists budgets and their per-account period amounts.
type BudgetRepository interface {
	CreateBudget(ctx context.Context, b domain.Budget) (domain.Budget, error)
	ListBudgets(ctx context.Context, bookGUID string) ([]domain.Budget, error)
	GetBudget(ctx context.Context, guid string) (domain.Budget, error)
	UpdateBudget(ctx context.Context, b domain.Budget) (domain.Budget, error)
	DeleteBudget(ctx context.Context, guid string) error
	BookGUIDForBudget(ctx context.Context, guid string) (string, error)
}

// BudgetVarianceLine is one account's budgeted vs actual spending for a period.
type BudgetVarianceLine struct {
	Account  domain.Account
	Budgeted domain.GncNumeric
	Actual   domain.GncNumeric
	Variance domain.GncNumeric // Actual − Budgeted
}

// BudgetReport is a budget-vs-actual summary for one period within a budget.
type BudgetReport struct {
	Budget        domain.Budget
	PeriodNum     int
	PeriodStart   time.Time
	PeriodEnd     time.Time
	PeriodLabel   string
	Lines         []BudgetVarianceLine
	TotalBudgeted domain.GncNumeric
	TotalActual   domain.GncNumeric
	TotalVariance domain.GncNumeric
}

// BudgetReportRepository combines the existing balance and root-account queries
// needed by the budget report, without duplicating them from ReportRepository.
type BudgetReportRepository interface {
	BudgetRepository
	BookRootAccount(ctx context.Context, bookGUID string) (string, error)
	AccountBalances(ctx context.Context, rootGUID string, from, to *time.Time) ([]AccountWithBalance, error)
}

// BudgetService manages budgets and produces variance reports.
type BudgetService struct {
	repo    BudgetReportRepository
	newGUID func() string
}

// NewBudgetService builds a BudgetService.
func NewBudgetService(repo BudgetReportRepository) *BudgetService {
	return &BudgetService{repo: repo, newGUID: NewGUID}
}

// Create validates and persists a new budget.
func (s *BudgetService) Create(ctx context.Context, b domain.Budget) (domain.Budget, error) {
	if b.Name == "" || !b.PeriodType.IsValid() || b.StartDate.IsZero() || b.NumPeriods <= 0 {
		return domain.Budget{}, ErrInvalidInput
	}
	if b.GUID == "" {
		b.GUID = s.newGUID()
	}
	return s.repo.CreateBudget(ctx, b)
}

// List returns all budgets for a book (amounts not loaded — use Get for detail).
func (s *BudgetService) List(ctx context.Context, bookGUID string) ([]domain.Budget, error) {
	return s.repo.ListBudgets(ctx, bookGUID)
}

// Get returns a budget with its amounts.
func (s *BudgetService) Get(ctx context.Context, guid string) (domain.Budget, error) {
	return s.repo.GetBudget(ctx, guid)
}

// Update replaces a budget's metadata and all its amounts.
func (s *BudgetService) Update(ctx context.Context, b domain.Budget) (domain.Budget, error) {
	if b.Name == "" || !b.PeriodType.IsValid() || b.StartDate.IsZero() || b.NumPeriods <= 0 {
		return domain.Budget{}, ErrInvalidInput
	}
	return s.repo.UpdateBudget(ctx, b)
}

// Delete removes a budget and all its amounts.
func (s *BudgetService) Delete(ctx context.Context, guid string) error {
	return s.repo.DeleteBudget(ctx, guid)
}

// BookGUIDForBudget returns the book a budget belongs to (for authz).
func (s *BudgetService) BookGUIDForBudget(ctx context.Context, guid string) (string, error) {
	return s.repo.BookGUIDForBudget(ctx, guid)
}

// Report returns a budget-vs-actual variance report for the period containing
// asOf (defaults to period 0 when asOf is before the budget start, or the last
// period when asOf is after it).
func (s *BudgetService) Report(ctx context.Context, budgetGUID string, asOf time.Time) (BudgetReport, error) {
	b, err := s.repo.GetBudget(ctx, budgetGUID)
	if err != nil {
		return BudgetReport{}, err
	}

	periodIdx := b.PeriodIndex(asOf)
	if periodIdx < 0 {
		if asOf.Before(b.StartDate) {
			periodIdx = 0
		} else {
			periodIdx = b.NumPeriods - 1
		}
	}

	periodStart := b.PeriodStart(periodIdx)
	periodEnd := b.PeriodEnd(periodIdx)

	rootGUID, err := s.repo.BookRootAccount(ctx, b.BookGUID)
	if err != nil {
		return BudgetReport{}, err
	}

	actuals, err := s.repo.AccountBalances(ctx, rootGUID, &periodStart, &periodEnd)
	if err != nil {
		return BudgetReport{}, err
	}

	actualByGUID := make(map[string]domain.GncNumeric, len(actuals))
	accountByGUID := make(map[string]domain.Account, len(actuals))
	for _, row := range actuals {
		nat := row.Account.Type.NaturalBalance(row.Balance)
		actualByGUID[row.Account.GUID] = nat
		accountByGUID[row.Account.GUID] = row.Account
	}

	// Collect amounts for this period.
	amtsByAcct := make(map[string]domain.GncNumeric)
	for _, amt := range b.Amounts {
		if amt.PeriodNum == periodIdx {
			amtsByAcct[amt.AccountGUID] = amt.Value
		}
	}

	var (
		lines         []BudgetVarianceLine
		totalBudgeted domain.GncNumeric
		totalActual   domain.GncNumeric
	)
	for acctGUID, budgeted := range amtsByAcct {
		actual := actualByGUID[acctGUID]
		account, ok := accountByGUID[acctGUID]
		if !ok {
			account = domain.Account{GUID: acctGUID, Name: acctGUID}
		}
		lines = append(lines, BudgetVarianceLine{
			Account:  account,
			Budgeted: budgeted,
			Actual:   actual,
			Variance: actual.Sub(budgeted),
		})
		totalBudgeted = totalBudgeted.Add(budgeted)
		totalActual = totalActual.Add(actual)
	}

	return BudgetReport{
		Budget:        b,
		PeriodNum:     periodIdx,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		PeriodLabel:   b.PeriodLabel(periodIdx),
		Lines:         lines,
		TotalBudgeted: totalBudgeted,
		TotalActual:   totalActual,
		TotalVariance: totalActual.Sub(totalBudgeted),
	}, nil
}
