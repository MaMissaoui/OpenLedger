package app

import (
	"context"
	"sort"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ForecastRepository reads the inputs a cash flow forecast projects forward: a
// book's current cash position and its scheduled (recurring) transactions.
type ForecastRepository interface {
	BookRootAccount(ctx context.Context, bookGUID string) (string, error)
	AccountBalances(ctx context.Context, rootGUID string, from, to *time.Time) ([]AccountWithBalance, error)
	ListScheduledTransactions(ctx context.Context, bookGUID string) ([]domain.ScheduledTransaction, error)
}

// ForecastService projects a book's cash balance forward from today by
// simulating its scheduled transactions over a horizon.
type ForecastService struct {
	repo ForecastRepository
}

// NewForecastService builds a ForecastService backed by repo.
func NewForecastService(repo ForecastRepository) *ForecastService {
	return &ForecastService{repo: repo}
}

// ForecastEvent is one projected cash movement: a single occurrence of a
// scheduled transaction, with its net effect on cash (inflow positive, outflow
// negative).
type ForecastEvent struct {
	Date   time.Time
	Name   string
	Amount domain.GncNumeric
}

// ForecastPoint is the projected cash position at the end of one month, with the
// inflows and outflows that moved it during that month.
type ForecastPoint struct {
	Date          time.Time
	ProjectedCash domain.GncNumeric
	Inflow        domain.GncNumeric
	Outflow       domain.GncNumeric
}

// CashFlowForecast is a forward projection of cash over [From, To]: a monthly
// balance curve plus the individual scheduled events that drive it. LowestCash
// flags the tightest point in the horizon — the practical liquidity risk.
type CashFlowForecast struct {
	From         time.Time
	To           time.Time
	StartingCash domain.GncNumeric
	EndingCash   domain.GncNumeric
	NetChange    domain.GncNumeric
	LowestCash   domain.GncNumeric
	LowestDate   time.Time
	Points       []ForecastPoint
	Events       []ForecastEvent
}

// Forecast projects cash from `from` over `months` whole months. The starting
// balance is the book's cash on hand as of `from`; each enabled scheduled
// transaction contributes its cash legs on every due date in the window.
func (s *ForecastService) Forecast(ctx context.Context, bookGUID string, from time.Time, months int) (CashFlowForecast, error) {
	if months <= 0 {
		months = 6
	}
	from = from.UTC().Truncate(24 * time.Hour)
	to := from.AddDate(0, months, 0)

	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return CashFlowForecast{}, err
	}
	// Cash on hand now, plus the set of cash-account GUIDs to recognise cash legs.
	asOfRows, err := s.repo.AccountBalances(ctx, root, nil, &from)
	if err != nil {
		return CashFlowForecast{}, err
	}
	starting := domain.Zero()
	cashAccounts := make(map[string]bool)
	for _, row := range asOfRows {
		if cashFlowClass(row.Account.Type) == cashAndEquivalents {
			cashAccounts[row.Account.GUID] = true
			starting = starting.Add(row.Balance)
		}
	}

	scheds, err := s.repo.ListScheduledTransactions(ctx, bookGUID)
	if err != nil {
		return CashFlowForecast{}, err
	}

	// Expand each schedule's occurrences in the window into cash events.
	var events []ForecastEvent
	for _, sched := range scheds {
		cashLeg := domain.Zero()
		hit := false
		for _, sp := range sched.Splits {
			if cashAccounts[sp.AccountGUID] {
				cashLeg = cashLeg.Add(sp.Value)
				hit = true
			}
		}
		if !hit || cashLeg.IsZero() {
			continue // a schedule that never touches cash doesn't move the forecast
		}
		for _, date := range sched.Occurrences(from, to) {
			events = append(events, ForecastEvent{Date: date, Name: sched.Name, Amount: cashLeg})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Date.Before(events[j].Date) })

	cf := CashFlowForecast{
		From:         from,
		To:           to,
		StartingCash: starting,
		Events:       events,
		LowestCash:   starting,
		LowestDate:   from,
	}

	// Walk month by month, applying that month's events to a running balance.
	running := starting
	ei := 0
	for m := 0; m < months; m++ {
		monthEnd := from.AddDate(0, m+1, 0)
		inflow, outflow := domain.Zero(), domain.Zero()
		for ei < len(events) && events[ei].Date.Before(monthEnd) {
			amt := events[ei].Amount
			if amt.Sign() >= 0 {
				inflow = inflow.Add(amt)
			} else {
				outflow = outflow.Add(amt)
			}
			running = running.Add(amt)
			ei++
		}
		if running.Cmp(cf.LowestCash) < 0 {
			cf.LowestCash = running
			cf.LowestDate = monthEnd
		}
		cf.Points = append(cf.Points, ForecastPoint{
			Date:          monthEnd,
			ProjectedCash: running,
			Inflow:        inflow,
			Outflow:       outflow,
		})
	}

	cf.EndingCash = running
	cf.NetChange = running.Sub(starting)
	return cf, nil
}
