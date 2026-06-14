package app

import (
	"context"
	"errors"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ErrScheduleNotFound is returned when a CRUD operation targets an unknown
// scheduled transaction. Handlers map it to 404.
var ErrScheduleNotFound = errors.New("scheduled transaction not found")

// ScheduleRepository persists scheduled transactions and their template splits.
type ScheduleRepository interface {
	CreateScheduledTransaction(ctx context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error)
	ListScheduledTransactions(ctx context.Context, bookGUID string) ([]domain.ScheduledTransaction, error)
	GetScheduledTransaction(ctx context.Context, guid string) (domain.ScheduledTransaction, error)
	UpdateScheduledTransaction(ctx context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error)
	DeleteScheduledTransaction(ctx context.Context, guid string) error
	// BookGUIDForSchedule returns the book_guid for the given schedule, or
	// ErrScheduleNotFound if it doesn't exist. Used by authorize paths.
	BookGUIDForSchedule(ctx context.Context, guid string) (string, error)
	// MarkSchedulePosted records the date on which a scheduled transaction was
	// last posted, so NextDueDate advances correctly next time.
	MarkSchedulePosted(ctx context.Context, guid string, date time.Time) error
}

// PostedSchedule records one posting made by PostDue.
type PostedSchedule struct {
	ScheduleGUID string
	Name         string
	PostDate     time.Time
	TxGUID       string
}

// ScheduleService manages scheduled transactions and posts due ones.
type ScheduleService struct {
	repo    ScheduleRepository
	posting *PostingService
	newGUID func() string
	now     func() time.Time
}

// NewScheduleService builds a ScheduleService.
func NewScheduleService(repo ScheduleRepository, posting *PostingService) *ScheduleService {
	return &ScheduleService{repo: repo, posting: posting, newGUID: NewGUID, now: time.Now}
}

// Create validates and persists a new scheduled transaction. It assigns GUIDs
// and rejects unbalanced or misconfigured templates.
func (s *ScheduleService) Create(ctx context.Context, sched domain.ScheduledTransaction, _ AuditActor) (domain.ScheduledTransaction, error) {
	if sched.Name == "" {
		return domain.ScheduledTransaction{}, ErrInvalidInput
	}
	if !sched.Period.IsValid() {
		return domain.ScheduledTransaction{}, ErrInvalidInput
	}
	if sched.Every <= 0 {
		sched.Every = 1
	}
	if sched.StartDate.IsZero() {
		return domain.ScheduledTransaction{}, ErrInvalidInput
	}
	if len(sched.Splits) < 2 {
		return domain.ScheduledTransaction{}, ErrInvalidInput
	}
	if err := sched.ValidateBalanced(); err != nil {
		return domain.ScheduledTransaction{}, domain.ErrUnbalanced
	}
	if sched.GUID == "" {
		sched.GUID = s.newGUID()
	}
	for i := range sched.Splits {
		if sched.Splits[i].GUID == "" {
			sched.Splits[i].GUID = s.newGUID()
		}
	}
	return s.repo.CreateScheduledTransaction(ctx, sched)
}

// List returns all scheduled transactions for a book.
func (s *ScheduleService) List(ctx context.Context, bookGUID string) ([]domain.ScheduledTransaction, error) {
	return s.repo.ListScheduledTransactions(ctx, bookGUID)
}

// Get returns a single scheduled transaction by GUID.
func (s *ScheduleService) Get(ctx context.Context, guid string) (domain.ScheduledTransaction, error) {
	return s.repo.GetScheduledTransaction(ctx, guid)
}

// Update replaces a scheduled transaction's fields and splits. The balance
// invariant is re-validated.
func (s *ScheduleService) Update(ctx context.Context, sched domain.ScheduledTransaction) (domain.ScheduledTransaction, error) {
	if sched.Name == "" || !sched.Period.IsValid() || sched.StartDate.IsZero() {
		return domain.ScheduledTransaction{}, ErrInvalidInput
	}
	if sched.Every <= 0 {
		sched.Every = 1
	}
	if len(sched.Splits) < 2 {
		return domain.ScheduledTransaction{}, ErrInvalidInput
	}
	if err := sched.ValidateBalanced(); err != nil {
		return domain.ScheduledTransaction{}, domain.ErrUnbalanced
	}
	for i := range sched.Splits {
		if sched.Splits[i].GUID == "" {
			sched.Splits[i].GUID = s.newGUID()
		}
	}
	return s.repo.UpdateScheduledTransaction(ctx, sched)
}

// Delete removes a scheduled transaction by GUID.
func (s *ScheduleService) Delete(ctx context.Context, guid string) error {
	return s.repo.DeleteScheduledTransaction(ctx, guid)
}

// BookGUIDForSchedule returns the book a schedule belongs to (for authz).
func (s *ScheduleService) BookGUIDForSchedule(ctx context.Context, guid string) (string, error) {
	return s.repo.BookGUIDForSchedule(ctx, guid)
}

// PostDue posts every enabled scheduled transaction in the book whose next due
// date is on or before asOf. It records the post date on each schedule so the
// due-date window advances. All due schedules are attempted; individual posting
// failures are collected and returned together.
func (s *ScheduleService) PostDue(ctx context.Context, bookGUID string, asOf time.Time, actor AuditActor) ([]PostedSchedule, error) {
	all, err := s.repo.ListScheduledTransactions(ctx, bookGUID)
	if err != nil {
		return nil, err
	}

	var (
		posted []PostedSchedule
		errs   []error
	)
	for _, sched := range all {
		if !sched.IsDue(asOf) {
			continue
		}
		postDate := sched.NextDueDate()
		splits := make([]domain.Split, len(sched.Splits))
		for i, sp := range sched.Splits {
			splits[i] = domain.Split{
				AccountGUID: sp.AccountGUID,
				Memo:        sp.Memo,
				Value:       sp.Value,
				Quantity:    sp.Value, // same-currency: quantity == value
			}
		}
		tx := domain.Transaction{
			CurrencyGUID: sched.CurrencyGUID,
			PostDate:     postDate,
			Description:  sched.Name,
			Splits:       splits,
		}
		result, err := s.posting.Post(ctx, tx, actor)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := s.repo.MarkSchedulePosted(ctx, sched.GUID, postDate); err != nil {
			errs = append(errs, err)
			continue
		}
		posted = append(posted, PostedSchedule{
			ScheduleGUID: sched.GUID,
			Name:         sched.Name,
			PostDate:     postDate,
			TxGUID:       result.GUID,
		})
	}
	if len(errs) > 0 {
		return posted, errs[0]
	}
	return posted, nil
}
