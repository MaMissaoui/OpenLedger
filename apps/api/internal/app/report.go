package app

import (
	"context"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ReportRepository reads the account balances the financial reports aggregate.
type ReportRepository interface {
	// BookRootAccount returns a book's root_account_guid, or ErrBookNotFound.
	BookRootAccount(ctx context.Context, bookGUID string) (string, error)
	// AccountBalances returns every descendant of rootGUID (the root excluded)
	// with the raw signed sum of its splits' quantities, restricted to
	// transactions whose post_date falls within [from, to]. A nil bound is open
	// on that side. Balances are in each account's own commodity; there is no
	// cross-currency conversion (no price engine yet), so totals are exact only
	// for single-currency books.
	AccountBalances(ctx context.Context, rootGUID string, from, to *time.Time) ([]AccountWithBalance, error)
}

// ReportService produces the core financial statements (balance sheet and
// income statement) from a book's account balances.
type ReportService struct {
	repo ReportRepository
}

// NewReportService builds a ReportService backed by repo.
func NewReportService(repo ReportRepository) *ReportService {
	return &ReportService{repo: repo}
}

// ReportLine is one account's contribution to a report section, carrying its
// natural-sign balance (positive for the account's normal direction) rendered
// at the commodity fraction Scale.
type ReportLine struct {
	Account domain.Account
	Balance domain.GncNumeric
	Scale   int64
}

// ReportSection is a group of report lines with their total, both in natural
// sign. Accounts with a zero balance are omitted from Lines.
type ReportSection struct {
	Lines []ReportLine
	Total domain.GncNumeric
}

func (sec *ReportSection) add(row AccountWithBalance) {
	nat := row.Account.Type.NaturalBalance(row.Balance)
	if nat.IsZero() {
		return
	}
	sec.Lines = append(sec.Lines, ReportLine{Account: row.Account, Balance: nat, Scale: row.BalanceScale})
	sec.Total = sec.Total.Add(nat)
}

// BalanceSheet is a point-in-time statement of financial position: what the
// book owns (assets) against what it owes (liabilities) and owns outright
// (equity). RetainedEarnings folds the period's net income into equity so that
// Assets = Liabilities + Equity + RetainedEarnings for a balanced book.
type BalanceSheet struct {
	AsOf                      time.Time
	Assets                    ReportSection
	Liabilities               ReportSection
	Equity                    ReportSection
	RetainedEarnings          domain.GncNumeric
	TotalLiabilitiesAndEquity domain.GncNumeric
}

// IncomeStatement is a statement of performance over [From, To]: income earned
// against expenses incurred, with NetIncome = total income − total expense.
type IncomeStatement struct {
	From      time.Time
	To        time.Time
	Income    ReportSection
	Expense   ReportSection
	NetIncome domain.GncNumeric
}

// BalanceSheet computes the book's balance sheet as of asOf (inclusive).
func (s *ReportService) BalanceSheet(ctx context.Context, bookGUID string, asOf time.Time) (BalanceSheet, error) {
	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return BalanceSheet{}, err
	}
	rows, err := s.repo.AccountBalances(ctx, root, nil, &asOf)
	if err != nil {
		return BalanceSheet{}, err
	}

	bs := BalanceSheet{AsOf: asOf}
	income, expense := domain.Zero(), domain.Zero()
	for _, row := range rows {
		switch classify(row.Account.Type) {
		case sectionAsset:
			bs.Assets.add(row)
		case sectionLiability:
			bs.Liabilities.add(row)
		case sectionEquity:
			bs.Equity.add(row)
		case sectionIncome:
			income = income.Add(row.Account.Type.NaturalBalance(row.Balance))
		case sectionExpense:
			expense = expense.Add(row.Account.Type.NaturalBalance(row.Balance))
		}
	}
	bs.RetainedEarnings = income.Sub(expense)
	bs.TotalLiabilitiesAndEquity = bs.Liabilities.Total.Add(bs.Equity.Total).Add(bs.RetainedEarnings)
	return bs, nil
}

// IncomeStatement computes the book's income statement over [from, to]. A zero
// from or to is treated as an open bound.
func (s *ReportService) IncomeStatement(ctx context.Context, bookGUID string, from, to time.Time) (IncomeStatement, error) {
	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return IncomeStatement{}, err
	}
	rows, err := s.repo.AccountBalances(ctx, root, optionalTime(from), optionalTime(to))
	if err != nil {
		return IncomeStatement{}, err
	}

	is := IncomeStatement{From: from, To: to}
	for _, row := range rows {
		switch classify(row.Account.Type) {
		case sectionIncome:
			is.Income.add(row)
		case sectionExpense:
			is.Expense.add(row)
		}
	}
	is.NetIncome = is.Income.Total.Sub(is.Expense.Total)
	return is, nil
}

// CashFlowSection is a group of cash-flow lines with their total. Unlike a
// ReportSection these carry the *cash effect* of each account (inflow positive,
// outflow negative) rather than a natural balance.
type CashFlowSection struct {
	Lines []ReportLine
	Total domain.GncNumeric
}

func (sec *CashFlowSection) add(acct domain.Account, cash domain.GncNumeric, scale int64) {
	if cash.IsZero() {
		return
	}
	sec.Lines = append(sec.Lines, ReportLine{Account: acct, Balance: cash, Scale: scale})
	sec.Total = sec.Total.Add(cash)
}

// CashFlowStatement explains the movement of cash over [From, To], grouped the
// standard three ways. It rests on the double-entry identity that every
// transaction's split values sum to zero: therefore the change in cash equals
// the negative of the change in every non-cash account. Each non-cash account's
// period balance change Δ contributes −Δ to cash, classified by the account's
// type. NetChange = Operating + Investing + Financing, which for a
// single-currency book equals the cash accounts' own change over the period.
type CashFlowStatement struct {
	From          time.Time
	To            time.Time
	Operating     CashFlowSection
	Investing     CashFlowSection
	Financing     CashFlowSection
	NetChange     domain.GncNumeric
	BeginningCash domain.GncNumeric
	EndingCash    domain.GncNumeric
}

// CashFlowStatement computes the book's cash flow statement over [from, to]. A
// zero from or to is treated as an open bound.
func (s *ReportService) CashFlowStatement(ctx context.Context, bookGUID string, from, to time.Time) (CashFlowStatement, error) {
	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return CashFlowStatement{}, err
	}
	// Balance change of every account during the period, plus the cumulative
	// balance up to `to` (for the ending cash position).
	periodRows, err := s.repo.AccountBalances(ctx, root, optionalTime(from), optionalTime(to))
	if err != nil {
		return CashFlowStatement{}, err
	}
	asOfRows, err := s.repo.AccountBalances(ctx, root, nil, optionalTime(to))
	if err != nil {
		return CashFlowStatement{}, err
	}

	cf := CashFlowStatement{From: from, To: to}
	for _, row := range periodRows {
		cash := row.Balance.Neg() // −Δ: a non-cash account's change, as a cash effect
		switch cashFlowClass(row.Account.Type) {
		case cashOperating:
			cf.Operating.add(row.Account, cash, row.BalanceScale)
		case cashInvesting:
			cf.Investing.add(row.Account, cash, row.BalanceScale)
		case cashFinancing:
			cf.Financing.add(row.Account, cash, row.BalanceScale)
		}
	}
	cf.NetChange = cf.Operating.Total.Add(cf.Investing.Total).Add(cf.Financing.Total)

	ending := domain.Zero()
	for _, row := range asOfRows {
		if cashFlowClass(row.Account.Type) == cashAndEquivalents {
			ending = ending.Add(row.Balance)
		}
	}
	cf.EndingCash = ending
	cf.BeginningCash = ending.Sub(cf.NetChange)
	return cf, nil
}

// cashFlowSection identifies which part of the cash flow statement an account
// type contributes to.
type cashFlowSection int

const (
	cashExcluded       cashFlowSection = iota // ROOT, TRADING — netted out / no balance
	cashAndEquivalents                        // BANK, CASH — the cash being explained
	cashOperating
	cashInvesting
	cashFinancing
)

// cashFlowClass maps an account type to its cash flow classification: cash and
// equivalents are the cash being tracked; everything else is a source or use of
// cash grouped into operating, investing, or financing activities.
func cashFlowClass(t domain.AccountType) cashFlowSection {
	switch t {
	case domain.AccountBank, domain.AccountCash:
		return cashAndEquivalents
	case domain.AccountIncome, domain.AccountExpense,
		domain.AccountReceivable, domain.AccountPayable:
		return cashOperating
	case domain.AccountAsset, domain.AccountStock,
		domain.AccountMutual, domain.AccountCurrency:
		return cashInvesting
	case domain.AccountEquity, domain.AccountLiability, domain.AccountCredit:
		return cashFinancing
	default:
		return cashExcluded
	}
}

// optionalTime returns a pointer to t, or nil when t is the zero time (an open
// bound).
func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// reportSection identifies which statement section an account type belongs to.
type reportSection int

const (
	sectionNone reportSection = iota
	sectionAsset
	sectionLiability
	sectionEquity
	sectionIncome
	sectionExpense
)

// classify maps an account type to its report section. Trading and root
// accounts belong to no section and are excluded.
func classify(t domain.AccountType) reportSection {
	switch t {
	case domain.AccountAsset, domain.AccountBank, domain.AccountCash,
		domain.AccountStock, domain.AccountMutual, domain.AccountCurrency,
		domain.AccountReceivable:
		return sectionAsset
	case domain.AccountLiability, domain.AccountCredit, domain.AccountPayable:
		return sectionLiability
	case domain.AccountEquity:
		return sectionEquity
	case domain.AccountIncome:
		return sectionIncome
	case domain.AccountExpense:
		return sectionExpense
	default:
		return sectionNone
	}
}
