package app

import (
	"context"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeReportRepo serves a fixed set of balances regardless of the date bounds,
// which it records so tests can assert how the service scoped the query.
type fakeReportRepo struct {
	root     string
	rows     []AccountWithBalance
	gotFrom  *time.Time
	gotTo    *time.Time
	rootErr  error
	balanErr error
}

func (f *fakeReportRepo) BookRootAccount(_ context.Context, _ string) (string, error) {
	return f.root, f.rootErr
}

func (f *fakeReportRepo) AccountBalances(_ context.Context, _ string, from, to *time.Time) ([]AccountWithBalance, error) {
	f.gotFrom, f.gotTo = from, to
	return f.rows, f.balanErr
}

func bal(typ domain.AccountType, rawNum int64) AccountWithBalance {
	return AccountWithBalance{
		Account:      domain.Account{GUID: string(typ), Type: typ},
		Balance:      domain.MustFromNumDenom(rawNum, 100),
		BalanceScale: 100,
	}
}

func TestBalanceSheetBalances(t *testing.T) {
	// A balanced book: +1500 checking (asset, debit), -1000 equity (credit),
	// -700 income (credit), +200 groceries (expense, debit). Raw values sum to
	// zero, so Assets must equal Liabilities + Equity + RetainedEarnings.
	repo := &fakeReportRepo{root: "root", rows: []AccountWithBalance{
		bal(domain.AccountBank, 150000),
		bal(domain.AccountEquity, -100000),
		bal(domain.AccountIncome, -70000),
		bal(domain.AccountExpense, 20000),
	}}
	asOf := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	bs, err := NewReportService(repo).BalanceSheet(context.Background(), "book-1", asOf)
	if err != nil {
		t.Fatalf("BalanceSheet: %v", err)
	}

	// Balance sheet is point-in-time: cumulative through asOf (no lower bound).
	if repo.gotFrom != nil || repo.gotTo == nil || !repo.gotTo.Equal(asOf) {
		t.Errorf("date bounds = [%v, %v], want [nil, %v]", repo.gotFrom, repo.gotTo, asOf)
	}
	if want := domain.MustFromNumDenom(150000, 100); !bs.Assets.Total.Equal(want) {
		t.Errorf("assets total = %s, want %s", bs.Assets.Total, want)
	}
	// Equity 1000 (credit → positive) + retained earnings (700 income − 200
	// expense = 500) = 1500, matching assets.
	if want := domain.MustFromNumDenom(50000, 100); !bs.RetainedEarnings.Equal(want) {
		t.Errorf("retained earnings = %s, want %s", bs.RetainedEarnings, want)
	}
	if !bs.TotalLiabilitiesAndEquity.Equal(bs.Assets.Total) {
		t.Errorf("balance sheet does not balance: assets %s vs L+E %s",
			bs.Assets.Total, bs.TotalLiabilitiesAndEquity)
	}
}

func TestBalanceSheetOmitsZeroLines(t *testing.T) {
	repo := &fakeReportRepo{root: "root", rows: []AccountWithBalance{
		bal(domain.AccountBank, 0),
		bal(domain.AccountCash, 5000),
	}}
	bs, err := NewReportService(repo).BalanceSheet(context.Background(), "b", time.Now())
	if err != nil {
		t.Fatalf("BalanceSheet: %v", err)
	}
	if len(bs.Assets.Lines) != 1 {
		t.Fatalf("asset lines = %d, want 1 (zero-balance account omitted)", len(bs.Assets.Lines))
	}
}

func TestIncomeStatement(t *testing.T) {
	repo := &fakeReportRepo{root: "root", rows: []AccountWithBalance{
		bal(domain.AccountIncome, -70000), // 700 earned
		bal(domain.AccountExpense, 20000), // 200 spent
		bal(domain.AccountBank, 150000),   // ignored by the income statement
	}}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	is, err := NewReportService(repo).IncomeStatement(context.Background(), "book-1", from, to)
	if err != nil {
		t.Fatalf("IncomeStatement: %v", err)
	}
	if repo.gotFrom == nil || !repo.gotFrom.Equal(from) || repo.gotTo == nil || !repo.gotTo.Equal(to) {
		t.Errorf("date bounds = [%v, %v], want [%v, %v]", repo.gotFrom, repo.gotTo, from, to)
	}
	if want := domain.MustFromNumDenom(70000, 100); !is.Income.Total.Equal(want) {
		t.Errorf("income total = %s, want %s", is.Income.Total, want)
	}
	if want := domain.MustFromNumDenom(50000, 100); !is.NetIncome.Equal(want) {
		t.Errorf("net income = %s, want %s", is.NetIncome, want)
	}
}

func TestCashFlowStatement(t *testing.T) {
	// The same balanced book, viewed since inception so ending cash equals the
	// period's net change and beginning cash is zero:
	//   bank    +1500 (cash)              owner invested 1000, earned 700, spent 200
	//   equity  -1000 → financing inflow +1000
	//   income   -700 → operating inflow  +700
	//   expense  +200 → operating outflow  -200
	repo := &fakeReportRepo{root: "root", rows: []AccountWithBalance{
		bal(domain.AccountBank, 150000),
		bal(domain.AccountEquity, -100000),
		bal(domain.AccountIncome, -70000),
		bal(domain.AccountExpense, 20000),
	}}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	cf, err := NewReportService(repo).CashFlowStatement(context.Background(), "book-1", from, to)
	if err != nil {
		t.Fatalf("CashFlowStatement: %v", err)
	}

	// Operating = +700 income − 200 expense = +500, from two lines.
	if want := domain.MustFromNumDenom(50000, 100); !cf.Operating.Total.Equal(want) {
		t.Errorf("operating total = %s, want %s", cf.Operating.Total, want)
	}
	if len(cf.Operating.Lines) != 2 {
		t.Errorf("operating lines = %d, want 2", len(cf.Operating.Lines))
	}
	// Financing = owner's +1000 contribution.
	if want := domain.MustFromNumDenom(100000, 100); !cf.Financing.Total.Equal(want) {
		t.Errorf("financing total = %s, want %s", cf.Financing.Total, want)
	}
	if !cf.Investing.Total.IsZero() || len(cf.Investing.Lines) != 0 {
		t.Errorf("investing = %s (%d lines), want zero", cf.Investing.Total, len(cf.Investing.Lines))
	}
	// Net change ties out to the cash accounts' own movement (+1500), and with no
	// prior history beginning cash is zero.
	if want := domain.MustFromNumDenom(150000, 100); !cf.NetChange.Equal(want) {
		t.Errorf("net change = %s, want %s", cf.NetChange, want)
	}
	if want := domain.MustFromNumDenom(150000, 100); !cf.EndingCash.Equal(want) {
		t.Errorf("ending cash = %s, want %s", cf.EndingCash, want)
	}
	if !cf.BeginningCash.IsZero() {
		t.Errorf("beginning cash = %s, want 0", cf.BeginningCash)
	}
}

func TestIncomeStatementOpenLowerBound(t *testing.T) {
	repo := &fakeReportRepo{root: "root"}
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if _, err := NewReportService(repo).IncomeStatement(context.Background(), "b", time.Time{}, to); err != nil {
		t.Fatalf("IncomeStatement: %v", err)
	}
	if repo.gotFrom != nil {
		t.Errorf("from bound = %v, want nil (zero time is open)", repo.gotFrom)
	}
}
