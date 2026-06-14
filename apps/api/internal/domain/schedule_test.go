package domain

import (
	"testing"
	"time"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func TestNextDueDate(t *testing.T) {
	t.Run("never posted returns start date", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:   true,
			Period:    PeriodMonthly,
			Every:     1,
			StartDate: date(2024, 1, 1),
		}
		got := s.NextDueDate()
		if !got.Equal(date(2024, 1, 1)) {
			t.Errorf("got %v, want 2024-01-01", got)
		}
	})

	t.Run("monthly advances by one month after last post", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:        true,
			Period:         PeriodMonthly,
			Every:          1,
			StartDate:      date(2024, 1, 1),
			LastPostedDate: date(2024, 1, 1),
		}
		got := s.NextDueDate()
		if !got.Equal(date(2024, 2, 1)) {
			t.Errorf("got %v, want 2024-02-01", got)
		}
	})

	t.Run("monthly skips multiple missed periods", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:        true,
			Period:         PeriodMonthly,
			Every:          1,
			StartDate:      date(2024, 1, 1),
			LastPostedDate: date(2024, 3, 1),
		}
		got := s.NextDueDate()
		if !got.Equal(date(2024, 4, 1)) {
			t.Errorf("got %v, want 2024-04-01", got)
		}
	})

	t.Run("biweekly every=2", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:        true,
			Period:         PeriodWeekly,
			Every:          2,
			StartDate:      date(2024, 1, 1),
			LastPostedDate: date(2024, 1, 1),
		}
		got := s.NextDueDate()
		if !got.Equal(date(2024, 1, 15)) {
			t.Errorf("got %v, want 2024-01-15", got)
		}
	})

	t.Run("once already posted returns zero", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:        true,
			Period:         PeriodOnce,
			Every:          1,
			StartDate:      date(2024, 1, 1),
			LastPostedDate: date(2024, 1, 1),
		}
		got := s.NextDueDate()
		if !got.IsZero() {
			t.Errorf("got %v, want zero", got)
		}
	})

	t.Run("disabled returns zero", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:   false,
			Period:    PeriodMonthly,
			Every:     1,
			StartDate: date(2024, 1, 1),
		}
		if !s.NextDueDate().IsZero() {
			t.Error("expected zero for disabled schedule")
		}
	})

	t.Run("past end date returns zero", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:        true,
			Period:         PeriodMonthly,
			Every:          1,
			StartDate:      date(2024, 1, 1),
			EndDate:        date(2024, 2, 28),
			LastPostedDate: date(2024, 2, 1),
		}
		got := s.NextDueDate()
		if !got.IsZero() {
			t.Errorf("got %v, want zero (past end)", got)
		}
	})

	t.Run("yearly advances correctly", func(t *testing.T) {
		s := ScheduledTransaction{
			Enabled:        true,
			Period:         PeriodYearly,
			Every:          1,
			StartDate:      date(2024, 3, 15),
			LastPostedDate: date(2024, 3, 15),
		}
		got := s.NextDueDate()
		if !got.Equal(date(2025, 3, 15)) {
			t.Errorf("got %v, want 2025-03-15", got)
		}
	})
}

func TestIsDue(t *testing.T) {
	s := ScheduledTransaction{
		Enabled:   true,
		Period:    PeriodMonthly,
		Every:     1,
		StartDate: date(2024, 1, 1),
	}
	if !s.IsDue(date(2024, 1, 1)) {
		t.Error("expected due on start date")
	}
	if !s.IsDue(date(2024, 2, 1)) {
		t.Error("expected due after start date")
	}
	if s.IsDue(date(2023, 12, 31)) {
		t.Error("expected not due before start date")
	}
}

func TestValidateScheduleBalanced(t *testing.T) {
	usd := func(cents int64) GncNumeric { return MustFromNumDenom(cents, 100) }
	s := ScheduledTransaction{
		Splits: []ScheduledSplit{
			{AccountGUID: "a", Value: usd(5000)},
			{AccountGUID: "b", Value: usd(-5000)},
		},
	}
	if err := s.ValidateBalanced(); err != nil {
		t.Errorf("balanced: %v", err)
	}
	s.Splits[1].Value = usd(-4999)
	if err := s.ValidateBalanced(); err == nil {
		t.Error("expected error for unbalanced schedule")
	}
}
