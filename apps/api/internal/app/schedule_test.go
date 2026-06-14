package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeScheduleRepo holds schedules in memory.
type fakeScheduleRepo struct {
	schedules   map[string]domain.ScheduledTransaction
	postedDates map[string]time.Time
	errCreate   error
	errList     error
	errGet      error
	errUpdate   error
	errDelete   error
	errMarkPost error

	// embed transaction writing
	fakeTransactionRepo
}

func newFakeScheduleRepo() *fakeScheduleRepo {
	return &fakeScheduleRepo{
		schedules:   make(map[string]domain.ScheduledTransaction),
		postedDates: make(map[string]time.Time),
	}
}

func (f *fakeScheduleRepo) CreateScheduledTransaction(_ context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error) {
	if f.errCreate != nil {
		return domain.ScheduledTransaction{}, f.errCreate
	}
	f.schedules[s.GUID] = s
	return s, nil
}

func (f *fakeScheduleRepo) ListScheduledTransactions(_ context.Context, bookGUID string) ([]domain.ScheduledTransaction, error) {
	if f.errList != nil {
		return nil, f.errList
	}
	var out []domain.ScheduledTransaction
	for _, s := range f.schedules {
		if s.BookGUID == bookGUID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeScheduleRepo) GetScheduledTransaction(_ context.Context, guid string) (domain.ScheduledTransaction, error) {
	if f.errGet != nil {
		return domain.ScheduledTransaction{}, f.errGet
	}
	s, ok := f.schedules[guid]
	if !ok {
		return domain.ScheduledTransaction{}, ErrScheduleNotFound
	}
	return s, nil
}

func (f *fakeScheduleRepo) UpdateScheduledTransaction(_ context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error) {
	if f.errUpdate != nil {
		return domain.ScheduledTransaction{}, f.errUpdate
	}
	if _, ok := f.schedules[s.GUID]; !ok {
		return domain.ScheduledTransaction{}, ErrScheduleNotFound
	}
	f.schedules[s.GUID] = s
	return s, nil
}

func (f *fakeScheduleRepo) DeleteScheduledTransaction(_ context.Context, guid string) error {
	if f.errDelete != nil {
		return f.errDelete
	}
	delete(f.schedules, guid)
	return nil
}

func (f *fakeScheduleRepo) BookGUIDForSchedule(_ context.Context, guid string) (string, error) {
	s, ok := f.schedules[guid]
	if !ok {
		return "", ErrScheduleNotFound
	}
	return s.BookGUID, nil
}

func (f *fakeScheduleRepo) MarkSchedulePosted(_ context.Context, guid string, date time.Time) error {
	if f.errMarkPost != nil {
		return f.errMarkPost
	}
	s, ok := f.schedules[guid]
	if !ok {
		return ErrScheduleNotFound
	}
	s.LastPostedDate = date
	f.schedules[guid] = s
	f.postedDates[guid] = date
	return nil
}

// fakeTransactionRepo satisfies TransactionRepository for the embedded posting service.
type fakeTransactionRepo struct {
	inserted []domain.Transaction
}

func (f *fakeTransactionRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ AuditActor) error {
	f.inserted = append(f.inserted, tx)
	return nil
}
func (f *fakeTransactionRepo) UpdateTransaction(_ context.Context, _ domain.Transaction, _ AuditActor) error {
	return nil
}
func (f *fakeTransactionRepo) DeleteTransaction(_ context.Context, _ string, _ AuditActor) error {
	return nil
}
func (f *fakeTransactionRepo) TransactionAccountGUIDs(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func cents(n int64) domain.GncNumeric { return domain.MustFromNumDenom(n, 100) }

func newScheduleService(repo *fakeScheduleRepo) *ScheduleService {
	posting := NewPostingService(&repo.fakeTransactionRepo)
	svc := NewScheduleService(repo, posting)
	svc.newGUID = func() string { return "test-guid" }
	svc.now = func() time.Time { return time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC) }
	return svc
}

func validSchedule(bookGUID string) domain.ScheduledTransaction {
	return domain.ScheduledTransaction{
		BookGUID:     bookGUID,
		Name:         "Monthly Rent",
		Enabled:      true,
		CurrencyGUID: "usd",
		Period:       domain.PeriodMonthly,
		Every:        1,
		StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Splits: []domain.ScheduledSplit{
			{AccountGUID: "rent", Value: cents(200000)},
			{AccountGUID: "checking", Value: cents(-200000)},
		},
	}
}

func TestScheduleCreate(t *testing.T) {
	repo := newFakeScheduleRepo()
	svc := newScheduleService(repo)
	ctx := context.Background()

	sched, err := svc.Create(ctx, validSchedule("book-1"), AuditActor{BookGUID: "book-1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sched.GUID == "" {
		t.Error("expected GUID to be assigned")
	}
	for _, sp := range sched.Splits {
		if sp.GUID == "" {
			t.Error("expected split GUID to be assigned")
		}
	}
}

func TestScheduleCreateValidation(t *testing.T) {
	repo := newFakeScheduleRepo()
	svc := newScheduleService(repo)
	ctx := context.Background()

	t.Run("empty name rejected", func(t *testing.T) {
		s := validSchedule("book-1")
		s.Name = ""
		if _, err := svc.Create(ctx, s, AuditActor{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
	t.Run("invalid period rejected", func(t *testing.T) {
		s := validSchedule("book-1")
		s.Period = "fortnightly"
		if _, err := svc.Create(ctx, s, AuditActor{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
	t.Run("zero start date rejected", func(t *testing.T) {
		s := validSchedule("book-1")
		s.StartDate = time.Time{}
		if _, err := svc.Create(ctx, s, AuditActor{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
	t.Run("fewer than 2 splits rejected", func(t *testing.T) {
		s := validSchedule("book-1")
		s.Splits = s.Splits[:1]
		if _, err := svc.Create(ctx, s, AuditActor{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
	t.Run("unbalanced splits rejected", func(t *testing.T) {
		s := validSchedule("book-1")
		s.Splits[0].Value = cents(100000)
		if _, err := svc.Create(ctx, s, AuditActor{}); err == nil {
			t.Error("expected error for unbalanced schedule")
		}
	})
}

func TestPostDue(t *testing.T) {
	repo := newFakeScheduleRepo()
	svc := newScheduleService(repo)
	ctx := context.Background()

	// Seed the repo directly so there's exactly one schedule to match.
	s := validSchedule("book-1")
	s.GUID = "sched-1"
	repo.schedules["sched-1"] = s

	asOf := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	posted, err := svc.PostDue(ctx, "book-1", asOf, AuditActor{BookGUID: "book-1"})
	if err != nil {
		t.Fatalf("post due: %v", err)
	}
	if len(posted) != 1 {
		t.Fatalf("posted = %d, want 1", len(posted))
	}
	if posted[0].Name != "Monthly Rent" {
		t.Errorf("name = %q, want Monthly Rent", posted[0].Name)
	}
	// Posting service should have been called.
	if len(repo.inserted) != 1 {
		t.Errorf("inserted = %d, want 1", len(repo.inserted))
	}
	// LastPostedDate should be advanced.
	if repo.postedDates["sched-1"].IsZero() {
		t.Error("expected MarkSchedulePosted to be called")
	}
}

func TestPostDueNotDue(t *testing.T) {
	repo := newFakeScheduleRepo()
	svc := newScheduleService(repo)
	ctx := context.Background()

	sched := validSchedule("book-1")
	sched.GUID = "sched-1"
	sched.StartDate = time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	repo.schedules["sched-1"] = sched

	// asOf is before the start date → nothing due
	asOf := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	posted, err := svc.PostDue(ctx, "book-1", asOf, AuditActor{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posted) != 0 {
		t.Errorf("expected 0 postings, got %d", len(posted))
	}
}
