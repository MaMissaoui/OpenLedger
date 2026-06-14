package app

import (
	"context"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// CapitalGainsRepository reads the realized-gain postings the capital-gains
// report itemizes.
type CapitalGainsRepository interface {
	// BookRootAccount returns a book's root_account_guid, or ErrBookNotFound.
	BookRootAccount(ctx context.Context, bookGUID string) (string, error)
	// RealizedGainRows returns every split posted to a "Capital Gains" INCOME
	// account under rootGUID whose transaction falls within [from, to] (nil bound
	// is open), one row per split, oldest first.
	RealizedGainRows(ctx context.Context, rootGUID string, from, to *time.Time) ([]RealizedGainRow, error)
}

// RealizedGainRow is a single capital-gains posting as stored: the raw split
// value (credit-normal, since the gains account is INCOME) with the account it
// hit and its transaction's date/description.
type RealizedGainRow struct {
	Date        time.Time
	Description string
	Account     domain.Account
	Value       domain.GncNumeric
	Scale       int64
}

// RealizedGain is one line of the capital-gains report in natural sign: a
// positive Amount is a gain, negative a loss.
type RealizedGain struct {
	Date        time.Time
	Description string
	Account     string
	Amount      domain.GncNumeric
	Scale       int64
}

// CapitalGainsReport lists realized gains/losses over [From, To] with their
// total, all in natural sign.
type CapitalGainsReport struct {
	From  time.Time
	To    time.Time
	Lines []RealizedGain
	Total domain.GncNumeric
}

// CapitalGainsService produces the realized capital-gains report from the
// postings to the book's Capital Gains accounts.
type CapitalGainsService struct {
	repo CapitalGainsRepository
}

// NewCapitalGainsService builds a CapitalGainsService backed by repo.
func NewCapitalGainsService(repo CapitalGainsRepository) *CapitalGainsService {
	return &CapitalGainsService{repo: repo}
}

// CapitalGains computes the realized capital-gains report over [from, to]. A
// zero from or to is an open bound. It returns ErrBookNotFound for an unknown
// book.
func (s *CapitalGainsService) CapitalGains(ctx context.Context, bookGUID string, from, to time.Time) (CapitalGainsReport, error) {
	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return CapitalGainsReport{}, err
	}
	rows, err := s.repo.RealizedGainRows(ctx, root, optionalTime(from), optionalTime(to))
	if err != nil {
		return CapitalGainsReport{}, err
	}

	report := CapitalGainsReport{From: from, To: to, Total: domain.Zero()}
	for _, row := range rows {
		// Income is credit-normal: a gain is stored as a negative value, so the
		// natural-sign amount flips it to positive.
		amount := row.Account.Type.NaturalBalance(row.Value)
		report.Lines = append(report.Lines, RealizedGain{
			Date:        row.Date,
			Description: row.Description,
			Account:     row.Account.Name,
			Amount:      amount,
			Scale:       row.Scale,
		})
		report.Total = report.Total.Add(amount)
	}
	return report, nil
}
